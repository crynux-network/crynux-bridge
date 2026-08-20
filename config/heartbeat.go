package config

import (
	"crynux_bridge/utils"
	"encoding/json"
	"fmt"
	"strings"
)

func validateHeartbeatTasksConfig(appConfig *AppConfig) error {
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
		case "sd":
			if task.Steps == 0 {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: steps must be > 0 for sd tasks", i)
			}
			if task.MaxNewTokens != 0 {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: max_new_tokens is not supported for sd tasks", i)
			}
			if len(task.Tools) > 0 {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: tools is not supported for sd tasks", i)
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
			if heartbeatMessagesHaveAssistantToolCalls(prompt.Messages) {
				promptHasToolCalls = true
			}
		}
		if promptHasToolCalls && len(task.Tools) == 0 {
			return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: tools must be non-empty when prompts include assistant tool_calls", i)
		}
	}
	return nil
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
