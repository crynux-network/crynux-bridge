package utils

import (
	"crynux_bridge/api/v1/llm/structs"
	"crynux_bridge/models"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

func ChatCompletionsRoleToRole(role structs.ChatCompletionsRole) models.LLMRole {
	switch role {
	case structs.ChatCompletionsRoleDeveloper:
		return models.LLMRoleUnknown
	case structs.ChatCompletionsRoleSystem:
		return models.LLMRoleSystem
	case structs.ChatCompletionsRoleUser:
		return models.LLMRoleUser
	case structs.ChatCompletionsRoleAssistant:
		return models.LLMRoleAssistant
	case structs.ChatCompletionsRoleTool:
		return models.LLMRoleTool
	}
	return models.LLMRoleUnknown
}

func RoleToChatCompletionsRole(role models.LLMRole) structs.ChatCompletionsRole {
	switch role {
	case models.LLMRoleUnknown:
		return structs.ChatCompletionsRoleUnknown
	case models.LLMRoleSystem:
		return structs.ChatCompletionsRoleSystem
	case models.LLMRoleUser:
		return structs.ChatCompletionsRoleUser
	case models.LLMRoleAssistant:
		return structs.ChatCompletionsRoleAssistant
	case models.LLMRoleTool:
		return structs.ChatCompletionsRoleTool
	}
	return structs.ChatCompletionsRoleUnknown
}

func CCReqMessageToolCallToToolCall(ccrMessagetoolCall structs.CCReqMessageToolCall) map[string]interface{} {
	toolCall := make(map[string]interface{})
	toolCall["id"] = ccrMessagetoolCall.ID
	toolCall["type"] = ccrMessagetoolCall.Type

	function := make(map[string]string)
	function["name"] = ccrMessagetoolCall.Function.Name
	function["arguments"] = ccrMessagetoolCall.Function.Arguments

	toolCall["function"] = function
	return toolCall
}

func CCReqMessageToMessage(ccrMessage structs.CCReqMessage) (models.Message, error) {
	var message models.Message
	message.Role = ChatCompletionsRoleToRole(ccrMessage.Role)
	content, err := ConvertReqContentToTaskContent(ccrMessage.Content)
	if err != nil {
		return models.Message{}, err
	}
	message.Content = content
	message.ToolCallID = ccrMessage.ToolCallID

	if len(ccrMessage.ToolCalls) > 0 {
		message.ToolCalls = make([]structs.ToolCall, len(ccrMessage.ToolCalls))
		for i, reqToolCall := range ccrMessage.ToolCalls {
			message.ToolCalls[i] = structs.ToolCall{
				Id:   reqToolCall.ID,
				Type: reqToolCall.Type,
				Function: structs.FunctionCall{
					Name:      reqToolCall.Function.Name,
					Arguments: reqToolCall.Function.Arguments,
				},
			}
		}
	}

	return message, nil
}

func ConvertReqContentToTaskContent(content *structs.CCReqMessageContent) (any, error) {
	if content == nil {
		return nil, fmt.Errorf("content is required")
	}

	if content.Text != nil {
		return *content.Text, nil
	}

	if len(content.Parts) == 0 {
		return nil, fmt.Errorf("content must be a string or a non-empty content parts array")
	}

	blocks := make([]models.MessageContentBlock, 0, len(content.Parts))
	for i, part := range content.Parts {
		switch part.Type {
		case "text":
			if part.Text == "" {
				return nil, fmt.Errorf("content part %d: text is required when type is text", i)
			}
			blocks = append(blocks, models.MessageContentBlock{
				Type: "text",
				Text: part.Text,
			})
		case "image_url":
			if part.ImageURL == nil {
				return nil, fmt.Errorf("content part %d: image_url is required when type is image_url", i)
			}
			base64Payload, err := extractBase64PayloadFromDataURL(part.ImageURL.URL)
			if err != nil {
				return nil, fmt.Errorf("content part %d: %w", i, err)
			}
			blocks = append(blocks, models.MessageContentBlock{
				Type:   "image",
				Base64: base64Payload,
			})
		default:
			return nil, fmt.Errorf("content part %d: unsupported type %q", i, part.Type)
		}
	}

	return blocks, nil
}

func extractBase64PayloadFromDataURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("image_url.url is required")
	}

	commaIdx := strings.Index(rawURL, ",")
	if commaIdx <= 0 || commaIdx == len(rawURL)-1 {
		return "", fmt.Errorf("image_url.url must be a non-empty data URL with base64 payload")
	}

	metadata := rawURL[:commaIdx]
	payload := rawURL[commaIdx+1:]
	if !strings.HasPrefix(strings.ToLower(metadata), "data:") || !strings.Contains(strings.ToLower(metadata), ";base64") {
		return "", fmt.Errorf("image_url.url must use data:*;base64,<payload> format")
	}

	if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
		if _, rawErr := base64.RawStdEncoding.DecodeString(payload); rawErr != nil {
			return "", fmt.Errorf("image_url.url contains invalid base64 payload")
		}
	}

	return payload, nil
}

func MessageContentToString(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func MessageToCCResMessage(message models.Message) structs.CCResMessage {
	var ccResMessage structs.CCResMessage
	ccResMessage.Role = RoleToChatCompletionsRole(message.Role)
	ccResMessage.Content = MessageContentToString(message.Content)
	// ccResMessage.Refusal = ""
	// ccResMessage.Annotations = nil
	// ccResMessage.Audio = nil
	ccResMessage.ToolCalls = message.ToolCalls

	return ccResMessage
}

func ResponseChoiceToCCResChoice(responseChoice models.ResponseChoice) structs.CCResChoice {
	var ccResChoice structs.CCResChoice
	ccResChoice.Index = responseChoice.Index
	ccResChoice.Message = MessageToCCResMessage(responseChoice.Message)
	ccResChoice.LogProbs = nil
	ccResChoice.FinishReason = string(responseChoice.FinishReason)
	return ccResChoice
}

func UsageToCCResUsage(usage models.Usage) structs.CCResUsage {
	var ccResUsage structs.CCResUsage
	ccResUsage.PromptTokens = usage.PromptTokens
	ccResUsage.CompletionTokens = usage.CompletionTokens
	ccResUsage.TotalTokens = usage.TotalTokens
	// ccResUsage.PromptTokensDetails = structs.PromptTokensDetails{}
	// ccResUsage.CompletionTokensDetails = structs.CompletionTokensDetails{}
	return ccResUsage
}

func ResponseChoiceToCResChoice(responseChoice models.ResponseChoice) (structs.CResChoice, error) {
	var cResChoice structs.CResChoice
	cResChoice.Index = responseChoice.Index
	cResChoice.Text = MessageContentToString(responseChoice.Message.Content)
	// ccResChoice.LogProbs = ""
	cResChoice.FinishReason = string(responseChoice.FinishReason)
	return cResChoice, nil
}

func UsageToCResUsage(usage models.Usage) structs.CResUsage {
	var cResUsage structs.CResUsage
	cResUsage.PromptTokens = usage.PromptTokens
	cResUsage.CompletionTokens = usage.CompletionTokens
	cResUsage.TotalTokens = usage.TotalTokens
	return cResUsage
}
