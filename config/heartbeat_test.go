package config

import "testing"

func TestValidateHeartbeatPromptTextOK(t *testing.T) {
	prompt := HeartbeatPromptConfig{Text: "hello"}
	if err := validateAndNormalizeHeartbeatPrompt("llm", &prompt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateAndNormalizeHeartbeatPrompt("sd", &prompt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHeartbeatPromptMutuallyExclusive(t *testing.T) {
	prompt := HeartbeatPromptConfig{
		Text: "hello",
		Content: []HeartbeatContentBlock{
			{Type: "text", Text: "world"},
		},
	}
	if err := validateAndNormalizeHeartbeatPrompt("llm", &prompt); err == nil {
		t.Fatalf("expected mutual exclusion error")
	}
}

func TestValidateHeartbeatPromptEmpty(t *testing.T) {
	prompt := HeartbeatPromptConfig{}
	if err := validateAndNormalizeHeartbeatPrompt("llm", &prompt); err == nil {
		t.Fatalf("expected empty prompt error")
	}
}

func TestValidateHeartbeatPromptSDRejectsContent(t *testing.T) {
	prompt := HeartbeatPromptConfig{
		Content: []HeartbeatContentBlock{
			{Type: "text", Text: "hello"},
		},
	}
	if err := validateAndNormalizeHeartbeatPrompt("sd", &prompt); err == nil {
		t.Fatalf("expected sd content rejection")
	}
}

func TestValidateHeartbeatPromptLLMNormalizesImageBase64(t *testing.T) {
	prompt := HeartbeatPromptConfig{
		Content: []HeartbeatContentBlock{
			{Type: "text", Text: "What is in this image?"},
			{Type: "image", Base64: "data:image/png;base64,aGVsbG8="},
		},
	}
	if err := validateAndNormalizeHeartbeatPrompt("llm", &prompt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompt.Content[1].Base64 != "aGVsbG8=" {
		t.Fatalf("expected normalized base64, got %q", prompt.Content[1].Base64)
	}
}

func TestValidateHeartbeatPromptInvalidImageBase64(t *testing.T) {
	prompt := HeartbeatPromptConfig{
		Content: []HeartbeatContentBlock{
			{Type: "image", Base64: "%%%"},
		},
	}
	if err := validateAndNormalizeHeartbeatPrompt("llm", &prompt); err == nil {
		t.Fatalf("expected invalid base64 error")
	}
}

func TestValidateHeartbeatTasksConfig(t *testing.T) {
	appConfig := &AppConfig{}
	appConfig.Task.HeartbeatTasks.Tasks = []HeartbeatTaskConfig{
		{
			Type:            "llm",
			Ratio:           1.0,
			MaxPendingTasks: 5,
			MaxNewTokens:    250,
			Prompts: []HeartbeatPromptConfig{
				{Text: "ok"},
				{
					Content: []HeartbeatContentBlock{
						{Type: "image", Base64: "aGVsbG8="},
					},
				},
			},
		},
	}
	if err := validateHeartbeatTasksConfig(appConfig); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHeartbeatTasksConfigRequiresMaxPendingTasks(t *testing.T) {
	appConfig := &AppConfig{}
	appConfig.Task.HeartbeatTasks.Tasks = []HeartbeatTaskConfig{
		{
			Type:         "llm",
			Ratio:        1.0,
			Model:        "Qwen/Qwen2.5-7B",
			MaxNewTokens: 250,
		},
	}
	if err := validateHeartbeatTasksConfig(appConfig); err == nil {
		t.Fatalf("expected max_pending_tasks validation error")
	}
}

func TestValidateHeartbeatTasksConfigAllowsZeroMaxPendingWhenRatioZero(t *testing.T) {
	appConfig := &AppConfig{}
	appConfig.Task.HeartbeatTasks.Tasks = []HeartbeatTaskConfig{
		{
			Type:         "llm",
			Ratio:        0,
			Model:        "Qwen/Qwen2.5-7B",
			MaxNewTokens: 250,
		},
	}
	if err := validateHeartbeatTasksConfig(appConfig); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHeartbeatTasksConfigRequiresMaxNewTokensForLLM(t *testing.T) {
	appConfig := &AppConfig{}
	appConfig.Task.HeartbeatTasks.Tasks = []HeartbeatTaskConfig{
		{
			Type:            "llm",
			Ratio:           1.0,
			Model:           "Qwen/Qwen2.5-7B",
			MaxPendingTasks: 5,
		},
	}
	if err := validateHeartbeatTasksConfig(appConfig); err == nil {
		t.Fatalf("expected max_new_tokens validation error")
	}
}

func TestValidateHeartbeatTasksConfigRejectsMaxNewTokensForSD(t *testing.T) {
	appConfig := &AppConfig{}
	appConfig.Task.HeartbeatTasks.Tasks = []HeartbeatTaskConfig{
		{
			Type:            "sd",
			Ratio:           1.0,
			Model:           "crynux-network/sdxl-turbo",
			MaxPendingTasks: 5,
			MaxNewTokens:    250,
		},
	}
	if err := validateHeartbeatTasksConfig(appConfig); err == nil {
		t.Fatalf("expected max_new_tokens rejection for sd tasks")
	}
}
