package config

import (
	"bytes"
	"crynux_bridge/utils"
	"encoding/json"
	"fmt"
	"strings"
)

// HeartbeatRawJSONPresent reports whether raw holds a non-empty, non-null JSON value.
func HeartbeatRawJSONPresent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	return true
}

func heartbeatRawJSONPresent(raw json.RawMessage) bool {
	return HeartbeatRawJSONPresent(raw)
}

func heartbeatJSONArrayLen(raw json.RawMessage) (int, error) {
	if !heartbeatRawJSONPresent(raw) {
		return 0, nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return 0, fmt.Errorf("must be a JSON array")
	}
	return len(arr), nil
}

const HeartbeatRandPlaceholder = "{{hb_rand}}"

func validateHeartbeatTasksConfig(appConfig *AppConfig) error {
	if appConfig.Task.HeartbeatTasks.MaxQueuedDuration < 0 {
		return fmt.Errorf("task.heartbeat_tasks.max_queued_duration must not be negative")
	}
	for i, task := range appConfig.Task.HeartbeatTasks.Tasks {
		if task.Ratio > 0 && task.MaxPendingTasks == 0 {
			return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: max_pending_tasks must be > 0 when ratio > 0", i)
		}
		taskType := strings.ToLower(task.Type)
		switch taskType {
		case "llm":
			if task.MaxNewTokens == 0 {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: max_new_tokens must be > 0 for llm tasks", i)
			}
			if task.Steps != 0 {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: steps is not supported for llm tasks", i)
			}
			if strings.TrimSpace(task.Dtype) != "" {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: dtype is not supported for llm tasks; LLM heartbeat dtype is always bfloat16", i)
			}
			if err := validateHeartbeatQuantizationConfig(task.QuantizationConfig); err != nil {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: %w", i, err)
			}
			if err := validateHeartbeatToolChoice(task.ToolChoice, task.Tools); err != nil {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: %w", i, err)
			}
			if err := validateHeartbeatResponseFormat(task.ResponseFormat); err != nil {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: %w", i, err)
			}
		case "sd":
			if task.Steps == 0 {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: steps must be > 0 for sd tasks", i)
			}
			if task.MaxNewTokens != 0 {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: max_new_tokens is not supported for sd tasks", i)
			}
			toolsLen, err := heartbeatJSONArrayLen(task.Tools)
			if err != nil {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: tools %w", i, err)
			}
			if toolsLen > 0 {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: tools is not supported for sd tasks", i)
			}
			if heartbeatRawJSONPresent(task.ToolChoice) {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: tool_choice is not supported for sd tasks", i)
			}
			if heartbeatRawJSONPresent(task.ResponseFormat) {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: response_format is not supported for sd tasks", i)
			}
			if err := validateHeartbeatQuantizationConfig(task.QuantizationConfig); err != nil {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: %w", i, err)
			}
		default:
			return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: unsupported heartbeat task type %q", i, task.Type)
		}

		promptHasToolCalls := false
		for j := range task.Prompts {
			prompt := &appConfig.Task.HeartbeatTasks.Tasks[i].Prompts[j]
			if err := validateAndNormalizeHeartbeatPrompt(taskType, prompt); err != nil {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d].prompts[%d]: %w", i, j, err)
			}
			if taskType == "llm" && task.Ratio > 0 && !heartbeatPromptContainsRandPlaceholder(prompt) {
				return fmt.Errorf(
					"task.heartbeat_tasks.tasks[%d].prompts[%d]: llm prompts must contain %s when ratio > 0",
					i, j, HeartbeatRandPlaceholder,
				)
			}
			if heartbeatMessagesHaveAssistantToolCalls(prompt.Messages) {
				promptHasToolCalls = true
			}
		}
		toolsLen, err := heartbeatJSONArrayLen(task.Tools)
		if err != nil {
			return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: tools %w", i, err)
		}
		if promptHasToolCalls && toolsLen == 0 {
			return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: tools must be non-empty when prompts include assistant tool_calls", i)
		}
	}
	return nil
}

func heartbeatPromptContainsRandPlaceholder(prompt *HeartbeatPromptConfig) bool {
	if strings.Contains(prompt.Text, HeartbeatRandPlaceholder) {
		return true
	}
	for _, block := range prompt.Content {
		if block.Type == "text" && strings.Contains(block.Text, HeartbeatRandPlaceholder) {
			return true
		}
	}
	for _, message := range prompt.Messages {
		if heartbeatMessageContentContainsRandPlaceholder(message.Content) {
			return true
		}
	}
	return false
}

func heartbeatMessageContentContainsRandPlaceholder(content any) bool {
	switch typed := content.(type) {
	case string:
		return strings.Contains(typed, HeartbeatRandPlaceholder)
	case []any:
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if block["type"] != "text" {
				continue
			}
			text, ok := block["text"].(string)
			if ok && strings.Contains(text, HeartbeatRandPlaceholder) {
				return true
			}
		}
	}
	return false
}

func validateAndNormalizeHeartbeatPrompt(taskType string, prompt *HeartbeatPromptConfig) error {
	hasText := strings.TrimSpace(prompt.Text) != ""
	hasContent := len(prompt.Content) > 0
	hasMessages := len(prompt.Messages) > 0

	modes := 0
	if hasText {
		modes++
	}
	if hasContent {
		modes++
	}
	if hasMessages {
		modes++
	}
	if modes == 0 {
		return fmt.Errorf("exactly one of text, content, or messages is required")
	}
	if modes > 1 {
		return fmt.Errorf("text, content, and messages are mutually exclusive")
	}

	switch taskType {
	case "sd":
		if hasContent {
			return fmt.Errorf("content is not supported for sd tasks")
		}
		if hasMessages {
			return fmt.Errorf("messages is not supported for sd tasks")
		}
		return nil
	case "llm":
		if hasContent {
			return validateAndNormalizeHeartbeatContent(prompt)
		}
		if hasMessages {
			return validateHeartbeatMessages(prompt.Messages)
		}
		return nil
	default:
		return fmt.Errorf("unsupported heartbeat task type %q", taskType)
	}
}

func validateAndNormalizeHeartbeatContent(prompt *HeartbeatPromptConfig) error {
	if len(prompt.Content) == 0 {
		return fmt.Errorf("content must be a non-empty array")
	}

	for i := range prompt.Content {
		block := &prompt.Content[i]
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) == "" {
				return fmt.Errorf("content[%d]: text is required when type is text", i)
			}
			if block.Base64 != "" {
				return fmt.Errorf("content[%d]: base64 is not allowed when type is text", i)
			}
			if strings.TrimSpace(block.ImagePath) != "" {
				return fmt.Errorf("content[%d]: image_path is not allowed when type is text", i)
			}
		case "image":
			if strings.TrimSpace(block.Text) != "" {
				return fmt.Errorf("content[%d]: text is not allowed when type is image", i)
			}
			if strings.TrimSpace(block.ImagePath) != "" {
				return fmt.Errorf("content[%d]: image_path must be resolved before validation", i)
			}
			normalized, err := utils.NormalizeImageBase64(block.Base64)
			if err != nil {
				return fmt.Errorf("content[%d]: %w", i, err)
			}
			block.Base64 = normalized
		default:
			return fmt.Errorf("content[%d]: unsupported type %q", i, block.Type)
		}
	}
	return nil
}

func validateHeartbeatMessages(messages []HeartbeatMessageConfig) error {
	if len(messages) == 0 {
		return fmt.Errorf("messages must be a non-empty array")
	}
	for i, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		switch role {
		case "system", "user":
			if err := validateHeartbeatMessageContent(i, message); err != nil {
				return err
			}
			if message.ToolCallID != "" {
				return fmt.Errorf("messages[%d]: tool_call_id is not allowed for role %q", i, message.Role)
			}
			if len(message.ToolCalls) > 0 {
				return fmt.Errorf("messages[%d]: tool_calls is not allowed for role %q", i, message.Role)
			}
		case "assistant":
			if err := validateHeartbeatAssistantMessage(i, message); err != nil {
				return err
			}
		case "tool":
			if strings.TrimSpace(message.ToolCallID) == "" {
				return fmt.Errorf("messages[%d]: tool_call_id is required for role tool", i)
			}
			if len(message.ToolCalls) > 0 {
				return fmt.Errorf("messages[%d]: tool_calls is not allowed for role tool", i)
			}
			content, ok := message.Content.(string)
			if !ok || strings.TrimSpace(content) == "" {
				return fmt.Errorf("messages[%d]: content must be a non-empty string for role tool", i)
			}
		default:
			return fmt.Errorf("messages[%d]: unsupported role %q", i, message.Role)
		}
	}
	return nil
}

func validateHeartbeatMessageContent(messageIndex int, message HeartbeatMessageConfig) error {
	switch content := message.Content.(type) {
	case string:
		if strings.TrimSpace(content) == "" {
			return fmt.Errorf("messages[%d]: content must be a non-empty string", messageIndex)
		}
		return nil
	case []any:
		if len(content) == 0 {
			return fmt.Errorf("messages[%d]: content blocks must be non-empty", messageIndex)
		}
		return nil
	default:
		if content == nil {
			return fmt.Errorf("messages[%d]: content is required", messageIndex)
		}
		return fmt.Errorf("messages[%d]: content must be a string or content block list", messageIndex)
	}
}

func validateHeartbeatAssistantMessage(messageIndex int, message HeartbeatMessageConfig) error {
	if message.ToolCallID != "" {
		return fmt.Errorf("messages[%d]: tool_call_id is not allowed for role assistant", messageIndex)
	}
	if len(message.ToolCalls) == 0 {
		return validateHeartbeatMessageContent(messageIndex, message)
	}
	for toolCallIndex, toolCall := range message.ToolCalls {
		if strings.TrimSpace(toolCall.ID) == "" {
			return fmt.Errorf("messages[%d].tool_calls[%d]: id is required", messageIndex, toolCallIndex)
		}
		if toolCall.Type != "function" {
			return fmt.Errorf("messages[%d].tool_calls[%d]: type must be function", messageIndex, toolCallIndex)
		}
		if strings.TrimSpace(toolCall.Function.Name) == "" {
			return fmt.Errorf("messages[%d].tool_calls[%d]: function.name is required", messageIndex, toolCallIndex)
		}
		if err := validateHeartbeatToolCallArguments(messageIndex, toolCallIndex, toolCall.Function.Arguments); err != nil {
			return err
		}
	}
	return nil
}

func validateHeartbeatToolCallArguments(messageIndex, toolCallIndex int, raw string) error {
	arguments := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil || arguments == nil {
		return fmt.Errorf("messages[%d].tool_calls[%d].function.arguments must be a JSON object string", messageIndex, toolCallIndex)
	}
	return nil
}

func heartbeatMessagesHaveAssistantToolCalls(messages []HeartbeatMessageConfig) bool {
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") && len(message.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func validateHeartbeatToolChoice(toolChoice json.RawMessage, tools json.RawMessage) error {
	if !heartbeatRawJSONPresent(toolChoice) {
		return nil
	}
	toolsLen, err := heartbeatJSONArrayLen(tools)
	if err != nil {
		return fmt.Errorf("tools %w", err)
	}
	var value any
	if err := json.Unmarshal(toolChoice, &value); err != nil {
		return fmt.Errorf("tool_choice must be a string or object")
	}
	switch typed := value.(type) {
	case string:
		switch typed {
		case "none":
			return nil
		case "auto", "required":
			if toolsLen == 0 {
				return fmt.Errorf("tools must be non-empty when tool_choice is %q", typed)
			}
			return nil
		default:
			return fmt.Errorf("tool_choice string must be none, auto, or required")
		}
	case map[string]interface{}:
		typeValue, ok := typed["type"].(string)
		if !ok || typeValue != "function" {
			return fmt.Errorf("tool_choice object type must be function")
		}
		functionValue, ok := typed["function"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("tool_choice.function must be an object")
		}
		name, ok := functionValue["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return fmt.Errorf("tool_choice.function.name must be a non-empty string")
		}
		if toolsLen == 0 {
			return fmt.Errorf("tools must be non-empty when tool_choice is a named function")
		}
		if !heartbeatToolsContainFunctionName(tools, name) {
			return fmt.Errorf("tool_choice.function.name %q must match a tools entry", name)
		}
		return nil
	default:
		return fmt.Errorf("tool_choice must be a string or object")
	}
}

func heartbeatToolsContainFunctionName(tools json.RawMessage, name string) bool {
	var decoded []map[string]interface{}
	if err := json.Unmarshal(tools, &decoded); err != nil {
		return false
	}
	for _, tool := range decoded {
		functionValue, ok := tool["function"].(map[string]interface{})
		if !ok {
			continue
		}
		toolName, ok := functionValue["name"].(string)
		if ok && toolName == name {
			return true
		}
	}
	return false
}

func validateHeartbeatResponseFormat(responseFormat json.RawMessage) error {
	if !heartbeatRawJSONPresent(responseFormat) {
		return nil
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(responseFormat, &decoded); err != nil {
		return fmt.Errorf("response_format must be an object")
	}
	typeValue, ok := decoded["type"].(string)
	if !ok {
		return fmt.Errorf("response_format.type must be a string")
	}
	switch typeValue {
	case "json_object":
		return nil
	case "json_schema":
		schemaValue, ok := decoded["json_schema"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("response_format.json_schema must be an object")
		}
		name, ok := schemaValue["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return fmt.Errorf("response_format.json_schema.name must be a non-empty string")
		}
		schema, ok := schemaValue["schema"].(map[string]interface{})
		if !ok || schema == nil {
			return fmt.Errorf("response_format.json_schema.schema must be an object")
		}
		return nil
	default:
		return fmt.Errorf("response_format.type must be json_object or json_schema")
	}
}

func validateHeartbeatQuantizationConfig(quantizationConfig json.RawMessage) error {
	if !heartbeatRawJSONPresent(quantizationConfig) {
		return nil
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(quantizationConfig, &decoded); err != nil {
		return fmt.Errorf("quantization_config must be an object")
	}
	if len(decoded) == 0 {
		return nil
	}
	quantMethod, ok := decoded["quant_method"].(string)
	if !ok || strings.TrimSpace(quantMethod) == "" {
		return fmt.Errorf("quantization_config.quant_method must be a non-empty string")
	}
	return nil
}
