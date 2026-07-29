package llm

import (
	"crynux_bridge/models"
	"encoding/json"
	"fmt"
	"strings"
)

type templateToolCallMessage struct {
	Role       models.LLMRole          `json:"role" validate:"required"`
	Content    any                     `json:"content,omitempty"`
	ToolCallID string                  `json:"tool_call_id,omitempty"`
	ToolCalls  []templateToolCallEntry `json:"tool_calls,omitempty"`
}

type templateToolCallEntry struct {
	Id       string                   `json:"id"`
	Type     string                   `json:"type"`
	Function templateToolCallFunction `json:"function"`
}

type templateToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func buildTemplateToolCallMessages(model string, messages []models.Message) (any, error) {
	if !hasAssistantToolCalls(messages) {
		return messages, nil
	}
	if usesStringToolCallArguments(model) {
		if err := validateToolCallArguments(messages); err != nil {
			return nil, err
		}
		return messages, nil
	}

	adaptedMessages := make([]templateToolCallMessage, len(messages))
	for messageIndex, message := range messages {
		adaptedMessage := templateToolCallMessage{
			Role:       message.Role,
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
		}
		if message.Role == models.LLMRoleAssistant && len(message.ToolCalls) > 0 {
			toolCalls, err := buildObjectToolCalls(messageIndex, message)
			if err != nil {
				return nil, err
			}
			adaptedMessage.ToolCalls = toolCalls
		}
		adaptedMessages[messageIndex] = adaptedMessage
	}
	return adaptedMessages, nil
}

func hasAssistantToolCalls(messages []models.Message) bool {
	for _, message := range messages {
		if message.Role == models.LLMRoleAssistant && len(message.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func usesStringToolCallArguments(model string) bool {
	normalized := strings.ToLower(model)
	if separator := strings.LastIndexAny(normalized, `/\`); separator >= 0 {
		normalized = normalized[separator+1:]
	}
	normalized = strings.ReplaceAll(normalized, "_", "-")

	if strings.HasPrefix(normalized, "deepseek-v3") {
		return true
	}
	return strings.HasPrefix(normalized, "deepseek-r1") &&
		!strings.HasPrefix(normalized, "deepseek-r1-distill-")
}

func validateToolCallArguments(messages []models.Message) error {
	for messageIndex, message := range messages {
		if message.Role != models.LLMRoleAssistant {
			continue
		}
		for toolCallIndex, toolCall := range message.ToolCalls {
			if _, err := parseToolCallArguments(messageIndex, toolCallIndex, toolCall.Function.Arguments); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildObjectToolCalls(messageIndex int, message models.Message) ([]templateToolCallEntry, error) {
	toolCalls := make([]templateToolCallEntry, len(message.ToolCalls))
	for toolCallIndex, toolCall := range message.ToolCalls {
		arguments, err := parseToolCallArguments(messageIndex, toolCallIndex, toolCall.Function.Arguments)
		if err != nil {
			return nil, err
		}
		toolCalls[toolCallIndex] = templateToolCallEntry{
			Id:   toolCall.Id,
			Type: toolCall.Type,
			Function: templateToolCallFunction{
				Name:      toolCall.Function.Name,
				Arguments: arguments,
			},
		}
	}
	return toolCalls, nil
}

func parseToolCallArguments(messageIndex, toolCallIndex int, raw string) (map[string]any, error) {
	arguments := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil || arguments == nil {
		return nil, fmt.Errorf("messages[%d].tool_calls[%d].function.arguments must be a JSON object", messageIndex, toolCallIndex)
	}
	return arguments, nil
}
