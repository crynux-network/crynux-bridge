package utils

import "testing"

func TestNormalizeImageBase64Raw(t *testing.T) {
	got, err := NormalizeImageBase64("aGVsbG8=")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "aGVsbG8=" {
		t.Fatalf("expected raw base64 to pass through, got %q", got)
	}
}

func TestNormalizeImageBase64DataURL(t *testing.T) {
	got, err := NormalizeImageBase64("data:image/png;base64,aGVsbG8=")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "aGVsbG8=" {
		t.Fatalf("expected data URL payload, got %q", got)
	}
}

func TestNormalizeImageBase64Invalid(t *testing.T) {
	if _, err := NormalizeImageBase64("not-base64!!"); err == nil {
		t.Fatalf("expected error for invalid base64")
	}
	if _, err := NormalizeImageBase64("https://example.com/a.png"); err == nil {
		t.Fatalf("expected error for non-data URL")
	}
	if _, err := NormalizeImageBase64(""); err == nil {
		t.Fatalf("expected error for empty base64")
	}
}

func TestExtractBase64PayloadFromDataURL(t *testing.T) {
	got, err := ExtractBase64PayloadFromDataURL("data:image/jpeg;base64,aGVsbG8=")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "aGVsbG8=" {
		t.Fatalf("unexpected payload %q", got)
	}
}
