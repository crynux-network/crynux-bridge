package structs

/* Request */

type CompletionsRequest struct {
	Model             string             `json:"model" validate:"required" description:"Huggingface model ID used to generate the response"`
	Prompt            string             `json:"prompt" validate:"required" description:"The prompt to generate a completion for"`
	Stream            bool               `json:"stream" description:"Enable streaming of results."` // default to false
	MaxTokens         *int               `json:"max_tokens" description:"Maximum number of tokens "`
	Temperature       float64            `json:"temperature" description:"Sampling temperature (range: [0, 2])."`
	Seed              int                `json:"seed" description:"Seed for deterministic outputs."`
	TopP              *float64           `json:"top_p" description:"Top-p sampling value (range: (0, 1])."`
	TopK              *int               `json:"top_k" description:"Top-k sampling value (range: [1, Infinity))."`
	FrequencyPenalty  *float64           `json:"frequency_penalty" description:"Frequency penalty (range: [-2, 2])."`
	PresencePenalty   *float64           `json:"presence_penalty" description:"Presence penalty (range: [-2, 2])."`
	RepetitionPenalty *float64           `json:"repetition_penalty" description:"Repetition penalty (range: [0, 2])."`
	LogitBias         map[string]float64 `json:"logit_bias" description:"Mapping of token IDs to bias values."`
	TopLogprobs       int                `json:"top_logprobs" description:"Number of top log probabilities to return."`
	MinP              *float64           `json:"min_p" description:"Minimum probability threshold (range: [0, 1])."`
	TopA              *float64           `json:"top_a" description:"Alternate top sampling parameter (range: [0, 1])."`

	// Transforms []string `json:"transforms"` // openrouter only
	// Models     []string `json:"models"`
	// Provider   TODO     `json:"provider"`
	// Reasoning  TODO     `json:"reasoning"`

	Stop []string `json:"stop"`

	BestOf        int               `json:"best_of" description:"No use for now. For compatibility with Openai."` // defaults to 1
	Echo          bool              `json:"echo" description:"No use for now. For compatibility with Openai."`
	LogProbs      int               `json:"logprobs" description:"No use for now. For compatibility with Openai."`
	N             int               `json:"n" default:"1" description:"Number of completions to generate."`
	StreamOptions CReqStreamOptions `json:"stream_options" description:"No use for now. For compatibility with Openai."`
	Suffix        string            `json:"suffix" description:"No use for now. For compatibility with Openai."`
	User          string            `json:"user" description:"No use for now. For compatibility with Openai."`

	// params for crynux task
	MinVram *uint64 `json:"min_vram" description:"mimimal gpu vram required for the crynux task"`
}

func (cr *CompletionsRequest) SetDefaultValues() {
	if cr.BestOf == 0 {
		cr.BestOf = 1
	}
}

type CReqStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

/* Response */

type CompletionsResponse struct {
	Id                string       `json:"id"`
	Object            string       `json:"object"`
	Created           int64        `json:"created"`
	Model             string       `json:"model"`
	SystemFingerprint string       `json:"system_fingerprint"`
	Choices           []CResChoice `json:"choices"`
	Usage             CResUsage    `json:"usage"`
}

type CResChoice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	LogProbs     string `json:"logprobs"`
	FinishReason string `json:"finish_reason"`
}

type CResUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
