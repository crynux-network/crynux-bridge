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
	taskArgs, err := buildLLMHeartbeatTaskArgs(config.HeartbeatTaskConfig{
		Model:        "Qwen/Qwen2.5-7B",
		MaxNewTokens: 64,
	}, config.HeartbeatPromptConfig{}, true)
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
	if _, ok := parsed["tools"]; ok {
		t.Fatalf("expected tools to be omitted when unset, got %#v", parsed["tools"])
	}
	generationConfig := parsed["generation_config"].(map[string]interface{})
	if generationConfig["max_new_tokens"] != float64(64) {
		t.Fatalf("unexpected max_new_tokens: %#v", generationConfig["max_new_tokens"])
	}
}

func TestBuildLLMHeartbeatTaskArgsText(t *testing.T) {
	taskArgs, err := buildLLMHeartbeatTaskArgs(
		config.HeartbeatTaskConfig{
			Model:        "Qwen/Qwen2.5-7B",
			MaxNewTokens: 128,
		},
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
	if parsed.GenerationConfig == nil || parsed.GenerationConfig.MaxNewTokens != 128 {
		t.Fatalf("unexpected max_new_tokens %#v", parsed.GenerationConfig)
	}
}

func TestBuildLLMHeartbeatTaskArgsContentBlocks(t *testing.T) {
	taskArgs, err := buildLLMHeartbeatTaskArgs(
		config.HeartbeatTaskConfig{
			Model:        "Qwen/Qwen2.5-VL-7B-Instruct",
			MaxNewTokens: 250,
		},
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

func TestBuildLLMHeartbeatTaskArgsToolsAndMessages(t *testing.T) {
	taskArgs, err := buildLLMHeartbeatTaskArgs(
		config.HeartbeatTaskConfig{
			Model:        "Qwen/Qwen3-8B",
			MaxNewTokens: 128,
			Tools: []map[string]interface{}{
				{
					"type": "function",
					"function": map[string]interface{}{
						"name": "search_docs",
						"parameters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"query": map[string]interface{}{"type": "string"},
							},
						},
					},
				},
			},
		},
		config.HeartbeatPromptConfig{
			Messages: []config.HeartbeatMessageConfig{
				{Role: "user", Content: "Find the GPU scheduling docs."},
				{
					Role: "assistant",
					ToolCalls: []config.HeartbeatMessageToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: config.HeartbeatMessageToolCallFunction{
								Name:      "search_docs",
								Arguments: `{"query":"gpu scheduling"}`,
							},
						},
					},
				},
				{Role: "tool", ToolCallID: "call_1", Content: "GPU scheduling uses VRAM demand and QoS."},
				{Role: "user", Content: "Summarize that result in one sentence."},
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
	if _, ok := parsed["tools"]; !ok {
		t.Fatal("expected tools in task args")
	}
	messages := parsed["messages"].([]interface{})
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}
	assistant := messages[1].(map[string]interface{})
	toolCalls := assistant["tool_calls"].([]interface{})
	function := toolCalls[0].(map[string]interface{})["function"].(map[string]interface{})
	arguments, ok := function["arguments"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected adapted arguments object, got %#v", function["arguments"])
	}
	if arguments["query"] != "gpu scheduling" {
		t.Fatalf("unexpected arguments %#v", arguments)
	}
}

func TestBuildSDHeartbeatTaskArgsConfiguredPrompt(t *testing.T) {
	taskArgs, err := buildSDHeartbeatTaskArgs(
		config.HeartbeatTaskConfig{
			Type:  "sd",
			Model: "crynux-network/sdxl-turbo",
			Steps: 4,
		},
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
	taskConfig := parsed["task_config"].(map[string]interface{})
	if taskConfig["steps"] != float64(4) {
		t.Fatalf("unexpected steps %#v", taskConfig["steps"])
	}
}

func TestBuildHeartbeatTaskArgsWithPrompts(t *testing.T) {
	taskArgs, taskType, err := buildHeartbeatTaskArgs(config.HeartbeatTaskConfig{
		Type:         "llm",
		Model:        "Qwen/Qwen2.5-7B",
		MaxNewTokens: 64,
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
	generationConfig := parsed["generation_config"].(map[string]interface{})
	if generationConfig["max_new_tokens"] != float64(64) {
		t.Fatalf("unexpected max_new_tokens: %#v", generationConfig["max_new_tokens"])
	}
}

func TestSelectHeartbeatTaskConfigExcludesOverPendingLimit(t *testing.T) {
	configs := []config.HeartbeatTaskConfig{
		{
			Type:            "llm",
			Model:           "Qwen/Qwen2.5-7B",
			Ratio:           1.0,
			MaxPendingTasks: 2,
		},
		{
			Type:            "llm",
			Model:           "Qwen/Qwen2.5-14B",
			Ratio:           1.0,
			MaxPendingTasks: 2,
		},
	}
	pendingCounts := map[string]uint64{
		"llm|Qwen/Qwen2.5-7B":  3,
		"llm|Qwen/Qwen2.5-14B": 1,
	}
	batchIncrements := map[string]uint64{}

	for i := 0; i < 20; i++ {
		selected, err := selectHeartbeatTaskConfig(configs, pendingCounts, batchIncrements)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected.Model != "Qwen/Qwen2.5-14B" {
			t.Fatalf("expected only 14B to remain eligible, got %q", selected.Model)
		}
	}
}

func TestSelectHeartbeatTaskConfigSharedModelUsesOwnLimit(t *testing.T) {
	configs := []config.HeartbeatTaskConfig{
		{
			Type:            "llm",
			Model:           "Qwen/Qwen2.5-7B",
			Ratio:           1.0,
			MaxPendingTasks: 1,
		},
		{
			Type:            "llm",
			Model:           "Qwen/Qwen2.5-7B",
			Ratio:           1.0,
			MaxPendingTasks: 5,
			MinVram:         24,
		},
	}
	pendingCounts := map[string]uint64{
		"llm|Qwen/Qwen2.5-7B": 2,
	}
	batchIncrements := map[string]uint64{}

	for i := 0; i < 20; i++ {
		selected, err := selectHeartbeatTaskConfig(configs, pendingCounts, batchIncrements)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected.MaxPendingTasks != 5 {
			t.Fatalf("expected only the looser limit item to remain eligible, got %#v", selected)
		}
	}
}

func TestSelectHeartbeatTaskConfigNoEligible(t *testing.T) {
	configs := []config.HeartbeatTaskConfig{
		{
			Type:            "llm",
			Model:           "Qwen/Qwen2.5-7B",
			Ratio:           1.0,
			MaxPendingTasks: 1,
		},
		{
			Type:            "sd",
			Model:           "crynux-network/sdxl-turbo",
			Ratio:           1.0,
			MaxPendingTasks: 1,
		},
	}
	pendingCounts := map[string]uint64{
		"llm|Qwen/Qwen2.5-7B":               2,
		"sd|crynux-network/sdxl-turbo": 2,
	}
	_, err := selectHeartbeatTaskConfig(configs, pendingCounts, map[string]uint64{})
	if err == nil {
		t.Fatalf("expected no eligible heartbeat task config error")
	}
}

func TestSelectHeartbeatTaskConfigBatchIncrementsBlockFurtherSamples(t *testing.T) {
	configs := []config.HeartbeatTaskConfig{
		{
			Type:            "llm",
			Model:           "Qwen/Qwen2.5-7B",
			Ratio:           1.0,
			MaxPendingTasks: 1,
		},
	}
	pendingCounts := map[string]uint64{
		"llm|Qwen/Qwen2.5-7B": 1,
	}
	batchIncrements := map[string]uint64{}

	selected, err := selectHeartbeatTaskConfig(configs, pendingCounts, batchIncrements)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Model != "Qwen/Qwen2.5-7B" {
		t.Fatalf("unexpected model %q", selected.Model)
	}
	batchIncrements[heartbeatTypeModelKey(selected.Type, selected.Model)]++

	_, err = selectHeartbeatTaskConfig(configs, pendingCounts, batchIncrements)
	if err == nil {
		t.Fatalf("expected no eligible heartbeat task config after batch increment")
	}
}

func TestHeartbeatTaskTypeAndModelID(t *testing.T) {
	llmType, llmModelID, err := heartbeatTaskTypeAndModelID(config.HeartbeatTaskConfig{
		Type:  "llm",
		Model: "Qwen/Qwen2.5-7B",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llmType != models.TaskTypeLLM || llmModelID != "base:Qwen/Qwen2.5-7B" {
		t.Fatalf("unexpected llm identity: type=%v modelID=%q", llmType, llmModelID)
	}

	sdType, sdModelID, err := heartbeatTaskTypeAndModelID(config.HeartbeatTaskConfig{
		Type:  "sd",
		Model: "crynux-network/sdxl-turbo",
		Steps: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sdType != models.TaskTypeSD || sdModelID != "base:crynux-network/sdxl-turbo+fp16" {
		t.Fatalf("unexpected sd identity: type=%v modelID=%q", sdType, sdModelID)
	}
}
