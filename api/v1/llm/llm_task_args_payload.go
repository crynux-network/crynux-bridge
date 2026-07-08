package llm

import "crynux_bridge/models"

type llmTaskArgsPayload struct {
	Model            string                      `json:"model" validate:"required"`
	Messages         any                         `json:"messages" validate:"required"`
	Tools            []map[string]interface{}    `json:"tools,omitempty"`
	GenerationConfig *models.GPTGenerationConfig `json:"generation_config,omitempty"`
	TemplateArgs     map[string]interface{}      `json:"template_args,omitempty"`
	Seed             int                         `json:"seed"`
	DType            models.DType                `json:"dtype,omitempty"`
	QuantizeBits     models.QuantizeBits         `json:"quantize_bits,omitempty"`
}

func buildLLMTaskArgsPayload(args models.GPTTaskArgs, messages any) llmTaskArgsPayload {
	return llmTaskArgsPayload{
		Model:            args.Model,
		Messages:         messages,
		Tools:            args.Tools,
		GenerationConfig: args.GenerationConfig,
		TemplateArgs:     args.TemplateArgs,
		Seed:             args.Seed,
		DType:            args.DType,
		QuantizeBits:     args.QuantizeBits,
	}
}
