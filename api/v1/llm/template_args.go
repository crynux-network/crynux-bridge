package llm

import "strings"

func buildLLMTemplateArgs(model string, tools []map[string]interface{}) map[string]interface{} {
	if len(tools) == 0 || !isQwenXMLToolCallModel(model) {
		return nil
	}
	return map[string]interface{}{
		"enable_thinking": false,
	}
}

func isQwenXMLToolCallModel(model string) bool {
	normalized := strings.ToLower(model)
	return strings.HasPrefix(normalized, "qwen/qwen3.5") ||
		strings.HasPrefix(normalized, "qwen3.5") ||
		strings.HasPrefix(normalized, "qwen/qwen3.6") ||
		strings.HasPrefix(normalized, "qwen3.6")
}
