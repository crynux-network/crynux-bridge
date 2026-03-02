package models_test

import (
	"crynux_bridge/models"
	"testing"
)

func TestValidateGPTTaskArgsContentJSONWithTextAndImageBlocks(t *testing.T) {
	taskArgsJSON := `{
		"model": "Qwen/Qwen2.5-7B-Instruct",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "Describe this image."},
					{"type": "image", "base64": "aGVsbG8="}
				]
			}
		]
	}`

	if err := models.ValidateGPTTaskArgsContentJSON(taskArgsJSON); err != nil {
		t.Fatalf("expected valid text and image blocks, got error: %v", err)
	}
}

func TestValidateGPTTaskArgsContentJSONRejectsImageWithoutBase64(t *testing.T) {
	taskArgsJSON := `{
		"model": "Qwen/Qwen2.5-7B-Instruct",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "image"}
				]
			}
		]
	}`

	err := models.ValidateGPTTaskArgsContentJSON(taskArgsJSON)
	if err == nil {
		t.Fatalf("expected error for image block without base64")
	}
}

func TestValidateGPTTaskArgsContentJSONRejectsUnsupportedBlockType(t *testing.T) {
	taskArgsJSON := `{
		"model": "Qwen/Qwen2.5-7B-Instruct",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "audio", "data": "xxxx"}
				]
			}
		]
	}`

	err := models.ValidateGPTTaskArgsContentJSON(taskArgsJSON)
	if err == nil {
		t.Fatalf("expected error for unsupported content block type")
	}
}

func TestValidateGPTTaskArgsContentJSONWithPlainTextContent(t *testing.T) {
	taskArgsJSON := `{
		"model": "Qwen/Qwen2.5-7B-Instruct",
		"messages": [
			{
				"role": "user",
				"content": "Hello"
			}
		]
	}`

	if err := models.ValidateGPTTaskArgsContentJSON(taskArgsJSON); err != nil {
		t.Fatalf("expected plain text content to pass, got error: %v", err)
	}
}
