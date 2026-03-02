package utils

import (
	"crynux_bridge/api/v1/llm/structs"
	"crynux_bridge/models"
	"testing"
)

func TestConvertReqContentToTaskContentWithString(t *testing.T) {
	text := "hello"
	content, err := ConvertReqContentToTaskContent(&structs.CCReqMessageContent{Text: &text})
	if err != nil {
		t.Fatalf("unexpected conversion error: %v", err)
	}

	converted, ok := content.(string)
	if !ok || converted != "hello" {
		t.Fatalf("expected string content to pass through")
	}
}

func TestConvertReqContentToTaskContentWithTextAndImageParts(t *testing.T) {
	content := &structs.CCReqMessageContent{
		Parts: []structs.CCReqMessageContentPart{
			{Type: "text", Text: "what is this image"},
			{
				Type: "image_url",
				ImageURL: &structs.CCReqMessageImageURL{
					URL: "data:image/png;base64,aGVsbG8=",
				},
			},
		},
	}

	converted, err := ConvertReqContentToTaskContent(content)
	if err != nil {
		t.Fatalf("unexpected conversion error: %v", err)
	}

	blocks, ok := converted.([]models.MessageContentBlock)
	if !ok {
		t.Fatalf("expected converted content to be []models.MessageContentBlock")
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != "text" || blocks[0].Text != "what is this image" {
		t.Fatalf("unexpected text block result")
	}
	if blocks[1].Type != "image" || blocks[1].Base64 != "aGVsbG8=" {
		t.Fatalf("unexpected image block result")
	}
}

func TestConvertReqContentToTaskContentWithInvalidDataURL(t *testing.T) {
	content := &structs.CCReqMessageContent{
		Parts: []structs.CCReqMessageContentPart{
			{
				Type: "image_url",
				ImageURL: &structs.CCReqMessageImageURL{
					URL: "https://example.com/image.png",
				},
			},
		},
	}

	_, err := ConvertReqContentToTaskContent(content)
	if err == nil {
		t.Fatalf("expected conversion error for non-data URL image input")
	}
}

func TestConvertReqContentToTaskContentWithUnsupportedPartType(t *testing.T) {
	content := &structs.CCReqMessageContent{
		Parts: []structs.CCReqMessageContentPart{
			{Type: "image", Text: "not supported"},
		},
	}

	_, err := ConvertReqContentToTaskContent(content)
	if err == nil {
		t.Fatalf("expected conversion error for unsupported content part type")
	}
}
