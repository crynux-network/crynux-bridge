package llm

import (
	"crynux_bridge/models"
	"encoding/json"
	"fmt"
)

type qwenXMLToolCallMessage struct {
	Role       models.LLMRole         `json:"role" validate:"required"`
	Content    any                    `json:"content,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	ToolCalls  []qwenXMLToolCallEntry `json:"tool_calls,omitempty"`
}

type qwenXMLToolCallEntry struct {
	Id       string                  `json:"id"`
	Type     string                  `json:"type"`
	Function qwenXMLToolCallFunction `json:"function"`
}

type qwenXMLToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func shouldAdaptQwenXMLToolCallMessages(model string, tools []map[string]interface{}) bool {
	return len(tools) > 0 && isQwenXMLToolCallModel(model)
}

func buildQwenXMLToolCallMessages(messages []models.Message) ([]qwenXMLToolCallMessage, error) {
	adaptedMessages := make([]qwenXMLToolCallMessage, len(messages))
	for messageIndex, message := range messages {
		adaptedMessage := qwenXMLToolCallMessage{
			Role:       message.Role,
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
		}
		if message.Role == models.LLMRoleAssistant && len(message.ToolCalls) > 0 {
			toolCalls, err := buildQwenXMLToolCalls(messageIndex, message)
			if err != nil {
				return nil, err
			}
			adaptedMessage.ToolCalls = toolCalls
		}
		adaptedMessages[messageIndex] = adaptedMessage
	}
	return adaptedMessages, nil
}

func buildQwenXMLToolCalls(messageIndex int, message models.Message) ([]qwenXMLToolCallEntry, error) {
	toolCalls := make([]qwenXMLToolCallEntry, len(message.ToolCalls))
	for toolCallIndex, toolCall := range message.ToolCalls {
		arguments := map[string]any{}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &arguments); err != nil {
			return nil, fmt.Errorf("messages[%d].tool_calls[%d].function.arguments must be a JSON object", messageIndex, toolCallIndex)
		}
		if arguments == nil {
			return nil, fmt.Errorf("messages[%d].tool_calls[%d].function.arguments must be a JSON object", messageIndex, toolCallIndex)
		}
		toolCalls[toolCallIndex] = qwenXMLToolCallEntry{
			Id:   toolCall.Id,
			Type: toolCall.Type,
			Function: qwenXMLToolCallFunction{
				Name:      toolCall.Function.Name,
				Arguments: arguments,
			},
		}
	}
	return toolCalls, nil
}
