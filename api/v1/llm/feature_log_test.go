package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeLLMRequestLogPayloadTruncatesPromptAndCountsTools(t *testing.T) {
	longPrompt := strings.Repeat("a", 120) + strings.Repeat("b", 120)
	payload := map[string]any{
		"request": map[string]any{
			"prompt": longPrompt,
			"tools": []any{
				map[string]any{"type": "function"},
				map[string]any{"type": "function"},
			},
		},
	}

	sanitized := sanitizeLLMRequestLogPayload(payload)
	b, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("failed to marshal sanitized payload: %v", err)
	}
	text := string(b)

	if strings.Contains(text, longPrompt) {
		t.Fatal("expected prompt to be truncated")
	}
	if !strings.Contains(text, strings.Repeat("a", 100)+"..."+strings.Repeat("b", 100)) {
		t.Fatalf("expected prompt to keep the first and last 100 characters, got %s", text)
	}
	if strings.Contains(text, `"tools"`) {
		t.Fatalf("expected tools payload to be removed, got %s", text)
	}
	if !strings.Contains(text, `"tools_count":2`) {
		t.Fatalf("expected tools count to be logged, got %s", text)
	}
}
