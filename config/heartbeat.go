package config

import (
	"crynux_bridge/utils"
	"fmt"
	"strings"
)

func validateHeartbeatTasksConfig(appConfig *AppConfig) error {
	for i, task := range appConfig.Task.HeartbeatTasks.Tasks {
		if task.Ratio > 0 && task.MaxPendingTasks == 0 {
			return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: max_pending_tasks must be > 0 when ratio > 0", i)
		}
		taskType := strings.ToLower(task.Type)
		switch taskType {
		case "llm":
			if task.MaxNewTokens == 0 {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: max_new_tokens must be > 0 for llm tasks", i)
			}
		case "sd":
			if task.MaxNewTokens != 0 {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d]: max_new_tokens is not supported for sd tasks", i)
			}
		}
		for j := range task.Prompts {
			if err := validateAndNormalizeHeartbeatPrompt(taskType, &appConfig.Task.HeartbeatTasks.Tasks[i].Prompts[j]); err != nil {
				return fmt.Errorf("task.heartbeat_tasks.tasks[%d].prompts[%d]: %w", i, j, err)
			}
		}
	}
	return nil
}

func validateAndNormalizeHeartbeatPrompt(taskType string, prompt *HeartbeatPromptConfig) error {
	hasText := strings.TrimSpace(prompt.Text) != ""
	hasContent := len(prompt.Content) > 0

	if hasText && hasContent {
		return fmt.Errorf("text and content are mutually exclusive")
	}
	if !hasText && !hasContent {
		return fmt.Errorf("either text or content is required")
	}

	switch taskType {
	case "sd":
		if hasContent {
			return fmt.Errorf("content is not supported for sd tasks")
		}
		return nil
	case "llm":
		if !hasContent {
			return nil
		}
		return validateAndNormalizeHeartbeatContent(prompt)
	default:
		return fmt.Errorf("unsupported heartbeat task type %q", taskType)
	}
}

func validateAndNormalizeHeartbeatContent(prompt *HeartbeatPromptConfig) error {
	if len(prompt.Content) == 0 {
		return fmt.Errorf("content must be a non-empty array")
	}

	for i := range prompt.Content {
		block := &prompt.Content[i]
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) == "" {
				return fmt.Errorf("content[%d]: text is required when type is text", i)
			}
			if block.Base64 != "" {
				return fmt.Errorf("content[%d]: base64 is not allowed when type is text", i)
			}
		case "image":
			if strings.TrimSpace(block.Text) != "" {
				return fmt.Errorf("content[%d]: text is not allowed when type is image", i)
			}
			normalized, err := utils.NormalizeImageBase64(block.Base64)
			if err != nil {
				return fmt.Errorf("content[%d]: %w", i, err)
			}
			block.Base64 = normalized
		default:
			return fmt.Errorf("content[%d]: unsupported type %q", i, block.Type)
		}
	}
	return nil
}
