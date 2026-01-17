package structs

import "encoding/json"

/* Request */

type ChatCompletionsRequest struct {
	Model             string             `json:"model" validate:"required" description:"Huggingface model ID used to generate the response"`
	Messages          []CCReqMessage     `json:"messages" validate:"required" description:"A list of messages comprising the conversation so far. "`
	Stream            bool               `json:"stream" description:"Enable streaming of results."` // default to false
	MaxTokens         *int               `json:"max_tokens" description:"Maximum number of tokens "`
	Temperature       float64            `json:"temperature" description:"Sampling temperature (range: [0, 2])."`
	Seed              int                `json:"seed" description:"Seed for deterministic outputs."`
	TopP              *float64           `json:"top_p" description:"Top-p sampling value (range: (0, 1]). "`
	TopK              *int               `json:"top_k" description:"Top-k sampling value (range: [1, Infinity)). "`
	FrequencyPenalty  *float64           `json:"frequency_penalty" description:"Frequency penalty (range: [-2, 2]). "`
	PresencePenalty   *float64           `json:"presence_penalty" description:"Presence penalty (range: [-2, 2]). "`
	RepetitionPenalty *float64           `json:"repetition_penalty" description:"Repetition penalty (range: [0, 2]). "`
	LogitBias         map[string]float64 `json:"logit_bias" description:"Mapping of token IDs to bias values."`
	TopLogprobs       int                `json:"top_logprobs" description:"Number of top log probabilities to return."`
	MinP              *float64           `json:"min_p" description:"Minimum probability threshold (range: [0, 1])."`
	TopA              *float64           `json:"top_a" description:"Alternate top sampling parameter (range: [0, 1])."`

	// Transforms []string `json:"transforms"` // llm only
	// Models     []string `json:"models"`
	// Provider   TODO     `json:"provider"`
	// Reasoning  TODO     `json:"reasoning"`

	Stop []string `json:"stop"`

	Audio             *CCReqAudio              `json:"audio" description:"Parameters for audio output. No use for now."`
	LogProbs          bool                     `json:"logprobs" description:"Whether to return log probabilities of the output tokens or not. No use for now."`
	MetaData          map[string]string        `json:"metadata" description:"No use for now. For compatibility with Openai."`
	Modalities        []string                 `json:"modalities" description:"No use for now. For compatibility with Openai."`
	N                 int                      `json:"n" default:"1" description:"Number of completions to generate."`
	Prediction        *CCReqPrediction         `json:"prediction" description:"No use for now. For compatibility with Openai."`
	ReasoningEffort   string                   `json:"reasoning_effort" description:"No use for now. For compatibility with Openai."`
	ResponseFormat    map[string]interface{}   `json:"response_format" description:"No use for now. For compatibility with Openai."`
	StructuredOutputs bool                     `json:"structured_outputs" description:"No use for now. For compatibility with Openai."`
	ServiceTier       string                   `json:"service_tier" description:"No use for now. For compatibility with Openai."`
	Store             bool                     `json:"store" description:"No use for now. For compatibility with Openai."`
	StreamOptions     *CCReqStreamOptions      `json:"stream_options" description:"No use for now. For compatibility with Openai."`
	ToolChoice        any                      `json:"tool_choice" description:"Controls which (if any) tool is called by the model. "`
	Tools             []map[string]interface{} `json:"tools" description:"A list of tools the model may call. "`
	User              string                   `json:"user" description:"No use for now. For compatibility with Openai."`
	WebSearchOptions  json.RawMessage          `json:"web_search_options" description:"No use for now. For compatibility with Openai."`

	// params for crynux task
	MinVram *uint64 `json:"min_vram" description:"mimimal gpu vram required for the crynux task"`
}

func (ccr *ChatCompletionsRequest) SetDefaultValues() {
	if ccr.N == 0 {
		ccr.N = 1 // defaults to 1
	}
}

// Chat Completions Request Message
type CCReqMessage struct {
	Role       ChatCompletionsRole    `json:"role" validate:"required"`
	Content    string                 `json:"content" validate:"required"`
	Name       string                 `json:"name"`
	Audio      *CCReqMessageAudio     `json:"audio"`
	Refusal    string                 `json:"refusal"`
	ToolCalls  []CCReqMessageToolCall `json:"tool_calls"`
	ToolCallID string                 `json:"tool_call_id"`
}

type ChatCompletionsRole string

const (
	ChatCompletionsRoleDeveloper ChatCompletionsRole = "developer"
	ChatCompletionsRoleSystem    ChatCompletionsRole = "system"
	ChatCompletionsRoleUser      ChatCompletionsRole = "user"
	ChatCompletionsRoleAssistant ChatCompletionsRole = "assistant"
	ChatCompletionsRoleTool      ChatCompletionsRole = "tool"
	ChatCompletionsRoleUnknown   ChatCompletionsRole = "unknown role"
)

type CCReqMessageAudio struct {
	ID string `json:"id" validate:"required"`
}

type CCReqMessageToolCall struct {
	ID       string                       `json:"id" validate:"required"`
	Function CCReqMessageToolCallFunction `json:"function" validate:"required"`
	Type     string                       `json:"type" validate:"required"`
}

type CCReqMessageToolCallFunction struct {
	Name      string `json:"name" validate:"required"`
	Arguments string `json:"arguments" validate:"required"`
}

type CCReqPrediction struct {
	StaticContent StaticContent
}

type StaticContent struct {
	Content json.RawMessage `json:"content" validate:"required"`
	Type    string          `json:"type" validate:"required"`
}

type CCReqStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type CCReqTool struct {
	Function json.RawMessage `json:"function"`
	Type     string          `json:"type"`
}

type CCReqAudio struct {
	Format string `json:"format" validate:"required"`
	Voice  string `json:"voice" validate:"required"`
}

/* Response */

type ChatCompletionsResponse struct {
	Id          string        `json:"id"`
	Object      string        `json:"object"`
	Created     int64         `json:"created"`
	Model       string        `json:"model"`
	Choices     []CCResChoice `json:"choices"`
	Usage       CCResUsage    `json:"usage"`
	ServiceTier string        `json:"service_tier"`
}

type CCResChoice struct {
	Index        int          `json:"index"`
	Message      CCResMessage `json:"message"`
	LogProbs     interface{}  `json:"logprobs"`
	FinishReason string       `json:"finish_reason"`
}

type CCResMessage struct {
	Role        ChatCompletionsRole `json:"role"`
	Content     string              `json:"content"`
	Refusal     string              `json:"refusal"`
	Annotations []interface{}       `json:"annotations"`
	Audio       interface{}         `json:"audio"`
	ToolCalls   []ToolCall          `json:"tool_calls,omitempty"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	Id       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type CCResUsage struct {
	PromptTokens            int                     `json:"prompt_tokens"`
	CompletionTokens        int                     `json:"completion_tokens"`
	TotalTokens             int                     `json:"total_tokens"`
	PromptTokensDetails     PromptTokensDetails     `json:"prompt_tokens_details"`
	CompletionTokensDetails CompletionTokensDetails `json:"completion_tokens_details"`
}

type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
	AudioTokens  int `json:"audio_tokens"`
}

type CompletionTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens"`
	AudioTokens              int `json:"audio_tokens"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
}
