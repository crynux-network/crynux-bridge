package models

import (
	"crynux_bridge/api/v1/llm/structs"
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
