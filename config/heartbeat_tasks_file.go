package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultHeartbeatTasksFile = "heartbeat_tasks.json"

func resolveConfigPath(configDir, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(configDir, path)
}

func loadHeartbeatTasksFile(appConfig *AppConfig, configDir string) error {
	tasksFile := strings.TrimSpace(appConfig.Task.HeartbeatTasks.TasksFile)
	if tasksFile == "" {
		tasksFile = defaultHeartbeatTasksFile
	}

	resolved := resolveConfigPath(configDir, tasksFile)
	data, err := os.ReadFile(resolved)
	if err != nil {
		return fmt.Errorf("task.heartbeat_tasks.tasks_file: read %s: %w", resolved, err)
	}

	var file struct {
		Tasks []HeartbeatTaskConfig `json:"tasks"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("task.heartbeat_tasks.tasks_file: parse %s: %w", resolved, err)
	}

	appConfig.Task.HeartbeatTasks.Tasks = file.Tasks
	if err := resolveHeartbeatImagePaths(appConfig, configDir); err != nil {
		return err
	}
	return nil
}

func resolveHeartbeatImagePaths(appConfig *AppConfig, configDir string) error {
	for i := range appConfig.Task.HeartbeatTasks.Tasks {
		for j := range appConfig.Task.HeartbeatTasks.Tasks[i].Prompts {
			prompt := &appConfig.Task.HeartbeatTasks.Tasks[i].Prompts[j]
			for k := range prompt.Content {
				block := &prompt.Content[k]
				if block.Type != "image" {
					continue
				}
				hasBase64 := strings.TrimSpace(block.Base64) != ""
				hasImagePath := strings.TrimSpace(block.ImagePath) != ""
				if hasBase64 && hasImagePath {
					return fmt.Errorf(
						"task.heartbeat_tasks.tasks[%d].prompts[%d].content[%d]: image_path and base64 are mutually exclusive",
						i, j, k,
					)
				}
				if !hasImagePath {
					continue
				}
				imagePath := resolveConfigPath(configDir, block.ImagePath)
				imageBytes, err := os.ReadFile(imagePath)
				if err != nil {
					return fmt.Errorf(
						"task.heartbeat_tasks.tasks[%d].prompts[%d].content[%d]: read image_path %s: %w",
						i, j, k, imagePath, err,
					)
				}
				block.Base64 = base64.StdEncoding.EncodeToString(imageBytes)
				block.ImagePath = ""
			}
		}
	}
	return nil
}
