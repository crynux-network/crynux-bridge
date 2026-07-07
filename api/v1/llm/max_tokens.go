package llm

func resolveMaxNewTokens(maxTokens *int, maxCompletionTokens *int, defaultMaxCompletionTokens int) *int {
	if maxTokens != nil {
		return maxTokens
	}
	if maxCompletionTokens != nil {
		return maxCompletionTokens
	}
	if defaultMaxCompletionTokens > 0 {
		return &defaultMaxCompletionTokens
	}
	return nil
}
