package llm

import "testing"

func TestResolveMinVramDefault(t *testing.T) {
	minVram, err := resolveMinVram(nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if minVram != 24 {
		t.Fatalf("expected default min VRAM 24, got %d", minVram)
	}
}

func TestResolveMinVramFromBody(t *testing.T) {
	bodyVramLimit := uint64(80)
	minVram, err := resolveMinVram(&bodyVramLimit, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if minVram != 80 {
		t.Fatalf("expected body VRAM limit 80, got %d", minVram)
	}
}

func TestResolveMinVramPathOverridesBody(t *testing.T) {
	bodyVramLimit := uint64(24)
	minVram, err := resolveMinVram(&bodyVramLimit, "80")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if minVram != 80 {
		t.Fatalf("expected path VRAM limit 80, got %d", minVram)
	}
}

func TestResolveMinVramInvalidPath(t *testing.T) {
	_, err := resolveMinVram(nil, "invalid")
	if err == nil {
		t.Fatalf("expected invalid path VRAM limit error")
	}
}
