package models

import (
	"crynux_bridge/api/v1/llm/structs"
	"encoding/json"
	"fmt"
	"strings"
)

type LLMRole string

const (
	LLMRoleSystem    LLMRole = "system"
	LLMRoleUser      LLMRole = "user"
	LLMRoleAssistant LLMRole = "assistant"
	LLMRoleTool      LLMRole = "tool"
	LLMRoleUnknown   LLMRole = "unknown role" // default value
)

type FinishReason string

const (
	FinishReasonStop      FinishReason = "stop"
	FinishReasonLength    FinishReason = "length"
	FinishReasonToolCalls FinishReason = "tool_calls"
)

type DType string

const (
	DTypeFloat16  DType = "float16"
	DTypeBFloat16 DType = "bfloat16"
	DTypeFloat32  DType = "float32"
	DTypeAuto     DType = "auto"
	DTypeUnknown  DType = "auto" // default value
)

type QuantizeBits int

const (
	QuantizeBits4 QuantizeBits = 4
	QuantizeBits8 QuantizeBits = 8
)

type Message struct {
	Role       LLMRole            `json:"role" validate:"required"` // Required
	Content    any                `json:"content,omitempty"`        // Optional, supports string or multimodal blocks
	ToolCallID string             `json:"tool_call_id,omitempty"`   // Optional
	ToolCalls  []structs.ToolCall `json:"tool_calls,omitempty"`     // Optional, uses structs.ToolCall
}

type MessageContentBlock struct {
	Type   string `json:"type" validate:"required"`
	Text   string `json:"text,omitempty"`
	Base64 string `json:"base64,omitempty"`
}

type GPTGenerationConfig struct {
	MaxNewTokens       int      `json:"max_new_tokens,omitempty"`
	StopStrings        []string `json:"stop_strings,omitempty"`
	DoSample           bool     `json:"do_sample,omitempty"`
	NumBeams           int      `json:"num_beams,omitempty"`
	Temperature        float64  `json:"temperature,omitempty"`
	TypicalP           float64  `json:"typical_p,omitempty"`
	TopK               int      `json:"top_k,omitempty"`
	TopP               float64  `json:"top_p,omitempty"`
	MinP               float64  `json:"min_p,omitempty"`
	RepetitionPenalty  float64  `json:"repetition_penalty,omitempty"`
	NumReturnSequences int      `json:"num_return_sequences,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type StreamChoice struct {
	Index        int           `json:"index"`
	Delta        Message       `json:"delta"`
	FinishReason *FinishReason `json:"finish_reason,omitempty"`
}

type GPTTaskStreamResponse struct {
	Model   string         `json:"model" validate:"required"`
	Choices []StreamChoice `json:"choices"`
	Usage   Usage          `json:"usage"`
}

type GPTTaskArgs struct {
	Model            string                   `json:"model" validate:"required"`    // Required
	Messages         []Message                `json:"messages" validate:"required"` // Required
	Tools            []map[string]interface{} `json:"tools,omitempty"`              // Optional
	GenerationConfig *GPTGenerationConfig     `json:"generation_config,omitempty"`  // Optional
	Seed             int                      `json:"seed"`                         // Optional, default 0
	DType            DType                    `json:"dtype,omitempty"`              // Optional, default "auto"
	QuantizeBits     QuantizeBits             `json:"quantize_bits,omitempty"`      // Optional
}

type ResponseChoice struct {
	Index        int          `json:"index"`
	Message      Message      `json:"message"`
	FinishReason FinishReason `json:"finish_reason"`
}

type GPTTaskResponse struct {
	Model   string           `json:"model" validate:"required"`
	Choices []ResponseChoice `json:"choices"`
	Usage   Usage            `json:"usage"`
}

// ValidateGPTTaskArgsContentJSON validates multimodal content semantics for GPT task args.
func ValidateGPTTaskArgsContentJSON(taskArgsJSON string) error {
	var taskArgs GPTTaskArgs
	if err := json.Unmarshal([]byte(taskArgsJSON), &taskArgs); err != nil {
		return err
	}
	return ValidateGPTTaskArgsContent(taskArgs)
}

// ValidateGPTTaskArgsContent validates message content shape for GPT tasks.
func ValidateGPTTaskArgsContent(taskArgs GPTTaskArgs) error {
	for messageIdx, message := range taskArgs.Messages {
		if err := validateMessageContent(message.Content); err != nil {
			return fmt.Errorf("messages[%d].content: %w", messageIdx, err)
		}
	}
	return nil
}

func validateMessageContent(content any) error {
	if IsNil(content) {
		return nil
	}

	switch value := content.(type) {
	case string:
		return nil
	case []interface{}:
		for partIdx, part := range value {
			if err := validateContentPart(part); err != nil {
				return fmt.Errorf("parts[%d]: %w", partIdx, err)
			}
		}
		return nil
	case []MessageContentBlock:
		for partIdx, part := range value {
			if err := validateMessageContentBlock(part); err != nil {
				return fmt.Errorf("parts[%d]: %w", partIdx, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("must be string, null, or array")
	}
}

func validateContentPart(part any) error {
	switch value := part.(type) {
	case map[string]interface{}:
		return validateContentPartMap(value)
	case MessageContentBlock:
		return validateMessageContentBlock(value)
	default:
		return fmt.Errorf("must be an object")
	}
}

func validateContentPartMap(part map[string]interface{}) error {
	typeValue, ok := part["type"]
	if !ok || IsNil(typeValue) {
		return fmt.Errorf("type is required")
	}

	typeName, ok := typeValue.(string)
	if !ok {
		return fmt.Errorf("type must be a string")
	}

	switch typeName {
	case "text":
		textValue, ok := part["text"]
		if !ok || IsNil(textValue) {
			return fmt.Errorf("text is required for text type")
		}
		text, ok := textValue.(string)
		if !ok {
			return fmt.Errorf("text must be a string")
		}
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("text must not be empty for text type")
		}
	case "image":
		base64Value, ok := part["base64"]
		if !ok || IsNil(base64Value) {
			return fmt.Errorf("base64 is required for image type")
		}
		base64Text, ok := base64Value.(string)
		if !ok {
			return fmt.Errorf("base64 must be a string")
		}
		if strings.TrimSpace(base64Text) == "" {
			return fmt.Errorf("base64 must not be empty for image type")
		}
	default:
		return fmt.Errorf("unsupported type %q", typeName)
	}

	return nil
}

func validateMessageContentBlock(part MessageContentBlock) error {
	switch part.Type {
	case "text":
		if strings.TrimSpace(part.Text) == "" {
			return fmt.Errorf("text must not be empty for text type")
		}
	case "image":
		if strings.TrimSpace(part.Base64) == "" {
			return fmt.Errorf("base64 must not be empty for image type")
		}
	default:
		return fmt.Errorf("unsupported type %q", part.Type)
	}
	return nil
}
