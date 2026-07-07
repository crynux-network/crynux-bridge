package llm

import "testing"

func TestResolveMaxNewTokensUsesMaxTokensFirst(t *testing.T) {
	maxTokens := 128
	maxCompletionTokens := 256
	defaultMaxCompletionTokens := 4096

	resolved := resolveMaxNewTokens(&maxTokens, &maxCompletionTokens, defaultMaxCompletionTokens)

	if resolved == nil {
		t.Fatalf("expected max new tokens")
	}
	if *resolved != maxTokens {
		t.Fatalf("expected max tokens %d, got %d", maxTokens, *resolved)
	}
}

func TestResolveMaxNewTokensUsesMaxCompletionTokens(t *testing.T) {
	maxCompletionTokens := 256
	defaultMaxCompletionTokens := 4096

	resolved := resolveMaxNewTokens(nil, &maxCompletionTokens, defaultMaxCompletionTokens)

	if resolved == nil {
		t.Fatalf("expected max new tokens")
	}
	if *resolved != maxCompletionTokens {
		t.Fatalf("expected max completion tokens %d, got %d", maxCompletionTokens, *resolved)
	}
}

func TestResolveMaxNewTokensUsesConfiguredDefault(t *testing.T) {
	defaultMaxCompletionTokens := 4096

	resolved := resolveMaxNewTokens(nil, nil, defaultMaxCompletionTokens)

	if resolved == nil {
		t.Fatalf("expected max new tokens")
	}
	if *resolved != defaultMaxCompletionTokens {
		t.Fatalf("expected configured default %d, got %d", defaultMaxCompletionTokens, *resolved)
	}
}

func TestResolveMaxNewTokensSkipsUnsetConfiguredDefault(t *testing.T) {
	if resolved := resolveMaxNewTokens(nil, nil, 0); resolved != nil {
		t.Fatalf("expected no max new tokens, got %d", *resolved)
	}
}
