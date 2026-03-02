package structs

import (
	"encoding/json"
	"testing"
)

func TestCCReqMessageContentUnmarshalString(t *testing.T) {
	var content CCReqMessageContent
	err := json.Unmarshal([]byte(`"hello"`), &content)
	if err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if content.Text == nil || *content.Text != "hello" {
		t.Fatalf("expected string content to be preserved")
	}
	if len(content.Parts) != 0 {
		t.Fatalf("expected no content parts for string content")
	}
}

func TestCCReqMessageContentUnmarshalParts(t *testing.T) {
	var content CCReqMessageContent
	payload := `[{"type":"text","text":"describe this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8="}}]`
	err := json.Unmarshal([]byte(payload), &content)
	if err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if content.Text != nil {
		t.Fatalf("expected nil text when content uses parts")
	}
	if len(content.Parts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(content.Parts))
	}
	if content.Parts[0].Type != "text" || content.Parts[0].Text != "describe this" {
		t.Fatalf("unexpected first content part")
	}
	if content.Parts[1].Type != "image_url" || content.Parts[1].ImageURL == nil {
		t.Fatalf("unexpected second content part")
	}
}

func TestCCReqMessageContentUnmarshalInvalidType(t *testing.T) {
	var content CCReqMessageContent
	err := json.Unmarshal([]byte(`123`), &content)
	if err == nil {
		t.Fatalf("expected error for invalid content type")
	}
}
