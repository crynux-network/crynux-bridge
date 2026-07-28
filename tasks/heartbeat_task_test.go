package tasks

import (
	"crynux_bridge/config"
	"crynux_bridge/models"
	"encoding/json"
	"testing"
)

func TestSelectHeartbeatPromptEmptyUsesDefault(t *testing.T) {
	_, useDefault, err := selectHeartbeatPrompt(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !useDefault {
		t.Fatalf("expected useDefault=true for empty prompts")
	}
}

func TestSelectHeartbeatPromptReturnsConfigured(t *testing.T) {
	prompts := []config.HeartbeatPromptConfig{
		{Text: "a"},
		{Text: "b"},
	}
	prompt, useDefault, err := selectHeartbeatPrompt(prompts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if useDefault {
		t.Fatalf("expected useDefault=false")
	}
	if prompt.Text != "a" && prompt.Text != "b" {
		t.Fatalf("unexpected selected prompt %#v", prompt)
	}
}

func TestBuildLLMHeartbeatTaskArgsDefault(t *testing.T) {
	taskArgs, err := buildLLMHeartbeatTaskArgs("Qwen/Qwen2.5-7B", config.HeartbeatPromptConfig{}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(taskArgs), &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	messages := parsed["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	if msg["content"] != "I want to create an AI agent. Any suggestions?" {
		t.Fatalf("unexpected default content: %#v", msg["content"])
	}
}

func TestBuildLLMHeartbeatTaskArgsText(t *testing.T) {
	taskArgs, err := buildLLMHeartbeatTaskArgs(
		"Qwen/Qwen2.5-7B",
		config.HeartbeatPromptConfig{Text: "hello heartbeat"},
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed models.GPTTaskArgs
	if err := json.Unmarshal([]byte(taskArgs), &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(parsed.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(parsed.Messages))
	}
	content, ok := parsed.Messages[0].Content.(string)
	if !ok || content != "hello heartbeat" {
		t.Fatalf("unexpected content %#v", parsed.Messages[0].Content)
	}
}

func TestBuildLLMHeartbeatTaskArgsContentBlocks(t *testing.T) {
	taskArgs, err := buildLLMHeartbeatTaskArgs(
		"Qwen/Qwen2.5-VL-7B-Instruct",
		config.HeartbeatPromptConfig{
			Content: []config.HeartbeatContentBlock{
				{Type: "text", Text: "What is in this image?"},
				{Type: "image", Base64: "aGVsbG8="},
			},
		},
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(taskArgs), &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	messages := parsed["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	blocks := msg["content"].([]interface{})
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(blocks))
	}
	textBlock := blocks[0].(map[string]interface{})
	imageBlock := blocks[1].(map[string]interface{})
	if textBlock["type"] != "text" || textBlock["text"] != "What is in this image?" {
		t.Fatalf("unexpected text block %#v", textBlock)
	}
	if imageBlock["type"] != "image" || imageBlock["base64"] != "aGVsbG8=" {
		t.Fatalf("unexpected image block %#v", imageBlock)
	}
}

func TestBuildSDHeartbeatTaskArgsConfiguredPrompt(t *testing.T) {
	taskArgs, err := buildSDHeartbeatTaskArgs(
		"crynux-network/sdxl-turbo",
		config.HeartbeatPromptConfig{
			Text:           "custom sd prompt",
			NegativePrompt: "blurry",
		},
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(taskArgs), &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if parsed["prompt"] != "custom sd prompt" {
		t.Fatalf("unexpected prompt %#v", parsed["prompt"])
	}
	if parsed["negative_prompt"] != "blurry" {
		t.Fatalf("unexpected negative_prompt %#v", parsed["negative_prompt"])
	}
}

func TestBuildHeartbeatTaskArgsWithPrompts(t *testing.T) {
	taskArgs, taskType, err := buildHeartbeatTaskArgs(config.HeartbeatTaskConfig{
		Type:  "llm",
		Model: "Qwen/Qwen2.5-7B",
		Prompts: []config.HeartbeatPromptConfig{
			{Text: "only-one"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if taskType != models.TaskTypeLLM {
		t.Fatalf("unexpected task type %v", taskType)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(taskArgs), &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	messages := parsed["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	if msg["content"] != "only-one" {
		t.Fatalf("unexpected content %#v", msg["content"])
	}
}
