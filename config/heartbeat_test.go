package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

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

	prompt = HeartbeatPromptConfig{
		Text: "hello",
		Messages: []HeartbeatMessageConfig{
			{Role: "user", Content: "world"},
		},
	}
	if err := validateAndNormalizeHeartbeatPrompt("llm", &prompt); err == nil {
		t.Fatalf("expected text/messages mutual exclusion error")
	}
}

func TestValidateHeartbeatPromptEmpty(t *testing.T) {
	prompt := HeartbeatPromptConfig{}
	if err := validateAndNormalizeHeartbeatPrompt("llm", &prompt); err == nil {
		t.Fatalf("expected empty prompt error")
	}
}

func TestValidateHeartbeatPromptMessagesOK(t *testing.T) {
	prompt := HeartbeatPromptConfig{
		Messages: []HeartbeatMessageConfig{
			{Role: "user", Content: "search docs"},
			{
				Role: "assistant",
				ToolCalls: []HeartbeatMessageToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: HeartbeatMessageToolCallFunction{
							Name:      "search_docs",
							Arguments: `{"query":"vram"}`,
						},
					},
				},
			},
			{Role: "tool", ToolCallID: "call_1", Content: "VRAM is video memory."},
		},
	}
	if err := validateAndNormalizeHeartbeatPrompt("llm", &prompt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHeartbeatPromptSDRejectsMessages(t *testing.T) {
	prompt := HeartbeatPromptConfig{
		Messages: []HeartbeatMessageConfig{
			{Role: "user", Content: "hello"},
		},
	}
	if err := validateAndNormalizeHeartbeatPrompt("sd", &prompt); err == nil {
		t.Fatalf("expected sd messages rejection")
	}
}

func TestValidateHeartbeatTasksConfigRequiresToolsForToolCalls(t *testing.T) {
	appConfig := &AppConfig{}
	appConfig.Task.HeartbeatTasks.Tasks = []HeartbeatTaskConfig{
		{
			Type:            "llm",
			Ratio:           1.0,
			MaxPendingTasks: 5,
			MaxNewTokens:    128,
			Prompts: []HeartbeatPromptConfig{
				{
					Messages: []HeartbeatMessageConfig{
						{Role: "user", Content: "search"},
						{
							Role: "assistant",
							ToolCalls: []HeartbeatMessageToolCall{
								{
									ID:   "call_1",
									Type: "function",
									Function: HeartbeatMessageToolCallFunction{
										Name:      "search_docs",
										Arguments: `{"query":"x"}`,
									},
								},
							},
						},
						{Role: "tool", ToolCallID: "call_1", Content: "result"},
					},
				},
			},
		},
	}
	if err := validateHeartbeatTasksConfig(appConfig); err == nil {
		t.Fatalf("expected tools required error")
	}

	appConfig.Task.HeartbeatTasks.Tasks[0].Tools = []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name": "search_docs",
			},
		},
	}
	if err := validateHeartbeatTasksConfig(appConfig); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
			Steps:           1,
			MaxNewTokens:    250,
		},
	}
	if err := validateHeartbeatTasksConfig(appConfig); err == nil {
		t.Fatalf("expected max_new_tokens rejection for sd tasks")
	}
}

func TestValidateHeartbeatTasksConfigRequiresStepsForSD(t *testing.T) {
	appConfig := &AppConfig{}
	appConfig.Task.HeartbeatTasks.Tasks = []HeartbeatTaskConfig{
		{
			Type:            "sd",
			Ratio:           1.0,
			Model:           "crynux-network/sdxl-turbo",
			MaxPendingTasks: 5,
		},
	}
	if err := validateHeartbeatTasksConfig(appConfig); err == nil {
		t.Fatalf("expected steps validation error")
	}
}

func TestValidateHeartbeatTasksConfigRejectsStepsForLLM(t *testing.T) {
	appConfig := &AppConfig{}
	appConfig.Task.HeartbeatTasks.Tasks = []HeartbeatTaskConfig{
		{
			Type:            "llm",
			Ratio:           1.0,
			Model:           "Qwen/Qwen2.5-7B",
			MaxPendingTasks: 5,
			MaxNewTokens:    64,
			Steps:           1,
		},
	}
	if err := validateHeartbeatTasksConfig(appConfig); err == nil {
		t.Fatalf("expected steps rejection for llm tasks")
	}
}

func writeHeartbeatTasksFixture(t *testing.T, dir string, jsonBody string) string {
	t.Helper()
	path := filepath.Join(dir, "heartbeat_tasks.json")
	if err := os.WriteFile(path, []byte(jsonBody), 0o644); err != nil {
		t.Fatalf("write tasks file: %v", err)
	}
	return path
}

func TestLoadHeartbeatTasksFileRelativePathAndImagePath(t *testing.T) {
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "vl_sample.png")
	pngBytes := []byte("png-bytes")
	if err := os.WriteFile(pngPath, pngBytes, 0o644); err != nil {
		t.Fatalf("write png: %v", err)
	}

	writeHeartbeatTasksFixture(t, dir, `{
  "tasks": [
    {
      "task_version": "3.5.0",
      "type": "llm",
      "ratio": 1.0,
      "model": "qwen/qwen3.6-27b",
      "min_vram": 120,
      "fee_cnx": 0.0006,
      "max_pending_tasks": 1,
      "max_new_tokens": 256,
      "prompts": [
        {
          "content": [
            {"type": "text", "text": "What is in this image?"},
            {"type": "image", "image_path": "vl_sample.png"}
          ]
        }
      ]
    }
  ]
}`)

	appConfig := &AppConfig{}
	appConfig.Task.HeartbeatTasks.TasksFile = "heartbeat_tasks.json"
	if err := loadHeartbeatTasksFile(appConfig, dir); err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if len(appConfig.Task.HeartbeatTasks.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(appConfig.Task.HeartbeatTasks.Tasks))
	}
	block := appConfig.Task.HeartbeatTasks.Tasks[0].Prompts[0].Content[1]
	if block.ImagePath != "" {
		t.Fatalf("expected image_path cleared after load, got %q", block.ImagePath)
	}
	want := base64.StdEncoding.EncodeToString(pngBytes)
	if block.Base64 != want {
		t.Fatalf("expected base64 %q, got %q", want, block.Base64)
	}
	if err := validateHeartbeatTasksConfig(appConfig); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
}

func TestLoadHeartbeatTasksFileDefaultName(t *testing.T) {
	dir := t.TempDir()
	writeHeartbeatTasksFixture(t, dir, `{
  "tasks": [
    {
      "task_version": "3.5.0",
      "type": "sd",
      "ratio": 1.0,
      "model": "crynux-network/sdxl-turbo",
      "min_vram": 14,
      "fee_cnx": 0.0001,
      "max_pending_tasks": 1,
      "steps": 1,
      "prompts": [{"text": "a cat"}]
    }
  ]
}`)

	appConfig := &AppConfig{}
	if err := loadHeartbeatTasksFile(appConfig, dir); err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if got := appConfig.Task.HeartbeatTasks.Tasks[0].Prompts[0].Text; got != "a cat" {
		t.Fatalf("unexpected prompt text %q", got)
	}
}

func TestLoadHeartbeatTasksFileRejectsImagePathAndBase64(t *testing.T) {
	dir := t.TempDir()
	writeHeartbeatTasksFixture(t, dir, `{
  "tasks": [
    {
      "task_version": "3.5.0",
      "type": "llm",
      "ratio": 1.0,
      "model": "qwen/qwen3.6-27b",
      "min_vram": 120,
      "fee_cnx": 0.0006,
      "max_pending_tasks": 1,
      "max_new_tokens": 256,
      "prompts": [
        {
          "content": [
            {"type": "image", "image_path": "vl_sample.png", "base64": "aGVsbG8="}
          ]
        }
      ]
    }
  ]
}`)

	appConfig := &AppConfig{}
	if err := loadHeartbeatTasksFile(appConfig, dir); err == nil {
		t.Fatalf("expected mutual exclusion error")
	}
}

func TestLoadAndValidateRepoHeartbeatTasksJSON(t *testing.T) {
	configDir := filepath.Join("..", "config")
	tasksPath := filepath.Join(configDir, "heartbeat_tasks.json")
	if _, err := os.Stat(tasksPath); err != nil {
		t.Skip("repo config/heartbeat_tasks.json is not present")
	}

	appConfig := &AppConfig{}
	if err := loadHeartbeatTasksFile(appConfig, configDir); err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if err := validateHeartbeatTasksConfig(appConfig); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
	if len(appConfig.Task.HeartbeatTasks.Tasks) < 1 {
		t.Fatal("expected at least one heartbeat task")
	}

	sdStepsByModel := map[string]map[uint64]struct{}{}
	for _, task := range appConfig.Task.HeartbeatTasks.Tasks {
		if task.Type != "sd" {
			continue
		}
		if sdStepsByModel[task.Model] == nil {
			sdStepsByModel[task.Model] = map[uint64]struct{}{}
		}
		sdStepsByModel[task.Model][task.Steps] = struct{}{}
	}
	if len(sdStepsByModel) == 0 {
		t.Fatal("expected at least one sd heartbeat task")
	}
	for model, steps := range sdStepsByModel {
		if len(steps) < 2 {
			t.Fatalf("sd model %s has %d distinct steps, want at least 2", model, len(steps))
		}
	}
}

func TestLoadHeartbeatTasksFileAbsoluteTasksFile(t *testing.T) {
	dir := t.TempDir()
	absTasks := writeHeartbeatTasksFixture(t, dir, `{
  "tasks": [
    {
      "task_version": "3.5.0",
      "type": "llm",
      "ratio": 0,
      "model": "qwen/qwen3-8b",
      "min_vram": 24,
      "fee_cnx": 0.0003,
      "max_new_tokens": 64,
      "prompts": [{"text": "hello"}]
    }
  ]
}`)

	appConfig := &AppConfig{}
	appConfig.Task.HeartbeatTasks.TasksFile = absTasks
	if err := loadHeartbeatTasksFile(appConfig, filepath.Join(dir, "unused")); err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if got := appConfig.Task.HeartbeatTasks.Tasks[0].Prompts[0].Text; got != "hello" {
		t.Fatalf("unexpected prompt text %q", got)
	}
}
