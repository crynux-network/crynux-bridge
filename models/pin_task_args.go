package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// PinTaskArgsRevisions returns a TaskArgs copy with each execution model file's
// revision written into the matching TaskArgs field by repository and format.
func PinTaskArgsRevisions(taskArgs string, taskType ChainTaskType, files ModelFileEntries) (string, error) {
	if len(files) == 0 {
		return "", errors.New("execution model files are required to pin task args revisions")
	}
	decoder := json.NewDecoder(strings.NewReader(taskArgs))
	decoder.UseNumber()
	var args map[string]any
	if err := decoder.Decode(&args); err != nil {
		return "", fmt.Errorf("invalid task args: %w", err)
	}

	matched := make(map[int]struct{}, len(files))
	switch taskType {
	case TaskTypeLLM:
		model, _ := args["model"].(string)
		if model == "" {
			return "", errors.New("task args model is empty")
		}
		pinned := false
		for i, file := range files {
			if !sameRepositoryFormat(file.Repository, file.Format, model, "") {
				continue
			}
			if pinned {
				return "", fmt.Errorf("duplicate execution model file for %q", model)
			}
			if file.Revision == "" {
				return "", fmt.Errorf("execution model file for %q has no revision", model)
			}
			args["revision"] = file.Revision
			matched[i] = struct{}{}
			pinned = true
		}
	case TaskTypeSD:
		baseModel, _ := args["base_model"].(map[string]any)
		if baseModel == nil {
			return "", errors.New("task args base_model is missing")
		}
		baseName, _ := baseModel["name"].(string)
		if baseName == "" {
			return "", errors.New("task args base_model.name is empty")
		}
		basePinned := false
		for i, file := range files {
			if !sameRepositoryFormat(file.Repository, file.Format, baseName, "diffusers") {
				continue
			}
			if basePinned {
				return "", fmt.Errorf("duplicate execution model file for base model %q", baseName)
			}
			if file.Revision == "" {
				return "", fmt.Errorf("execution model file for base model %q has no revision", baseName)
			}
			baseModel["revision"] = file.Revision
			args["base_model"] = baseModel
			matched[i] = struct{}{}
			basePinned = true
		}
		components, _ := args["components"].(map[string]any)
		for name, raw := range components {
			component, ok := raw.(map[string]any)
			if !ok {
				return "", fmt.Errorf("task args components.%s is invalid", name)
			}
			repo, _ := component["repo"].(string)
			format, _ := component["format"].(string)
			if repo == "" || format == "" {
				return "", fmt.Errorf("task args components.%s requires repo and format", name)
			}
			componentPinned := false
			for i, file := range files {
				if !sameRepositoryFormat(file.Repository, file.Format, repo, format) {
					continue
				}
				if componentPinned {
					return "", fmt.Errorf("duplicate execution model file for component %q", name)
				}
				if file.Revision == "" {
					return "", fmt.Errorf("execution model file for component %q has no revision", name)
				}
				component["revision"] = file.Revision
				components[name] = component
				matched[i] = struct{}{}
				componentPinned = true
			}
		}
		if components != nil {
			args["components"] = components
		}
	default:
		return "", fmt.Errorf("task type %d does not support pinned task args revisions", taskType)
	}

	if len(matched) != len(files) {
		return "", errors.New("execution model file does not match any model of the task args")
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func sameRepositoryFormat(fileRepo, fileFormat, repo, format string) bool {
	return NormalizeModelName(fileRepo) == NormalizeModelName(repo) && fileFormat == format
}
