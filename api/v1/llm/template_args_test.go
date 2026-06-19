package llm

import "testing"

func TestBuildLLMTemplateArgsDisablesThinkingForQwenXMLToolCallModels(t *testing.T) {
	tools := []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name": "write_file",
			},
		},
	}

	tests := []string{
		"Qwen/Qwen3.5-27B",
		"Qwen/Qwen3.6-27B",
		"qwen3.5-4b",
		"qwen3.6-35b-a3b",
	}

	for _, model := range tests {
		templateArgs := buildLLMTemplateArgs(model, tools)
		if templateArgs == nil {
			t.Fatalf("expected template args for model %s", model)
		}
		if templateArgs["enable_thinking"] != false {
			t.Fatalf("expected enable_thinking=false for model %s, got %#v", model, templateArgs)
		}
	}
}

func TestBuildLLMTemplateArgsSkipsNonQwenXMLToolCallModels(t *testing.T) {
	tools := []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name": "write_file",
			},
		},
	}

	for _, model := range []string{"Qwen/Qwen3-8B", "Qwen/Qwen2.5-72B", "DeepSeek-V3.2"} {
		if templateArgs := buildLLMTemplateArgs(model, tools); templateArgs != nil {
			t.Fatalf("expected no template args for model %s, got %#v", model, templateArgs)
		}
	}
}

func TestBuildLLMTemplateArgsSkipsRequestsWithoutTools(t *testing.T) {
	if templateArgs := buildLLMTemplateArgs("Qwen/Qwen3.6-27B", nil); templateArgs != nil {
		t.Fatalf("expected no template args without tools, got %#v", templateArgs)
	}
}
