package models

import (
	"encoding/json"
	"strings"
)

// NormalizeModelName lowercases a Hugging Face model repository name so that
// names differing only in letter case map to the same model. URL-based model
// names are kept unchanged because URL paths are case sensitive.
func NormalizeModelName(name string) string {
	if strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://") {
		return name
	}
	return strings.ToLower(name)
}

// NormalizeTaskArgsModelNames rewrites the model name fields inside the task
// args JSON string with NormalizeModelName, so persisted TaskArgs, model IDs,
// Relay create, and VSS pin matching use the same casing.
func NormalizeTaskArgsModelNames(taskArgs string, taskType ChainTaskType) (string, error) {
	decoder := json.NewDecoder(strings.NewReader(taskArgs))
	decoder.UseNumber()
	var argsMap map[string]interface{}
	if err := decoder.Decode(&argsMap); err != nil {
		return "", err
	}

	switch taskType {
	case TaskTypeSD:
		if baseModel, ok := argsMap["base_model"].(string); ok {
			argsMap["base_model"] = NormalizeModelName(baseModel)
		} else {
			normalizeObjectModelName(argsMap, "base_model", "name")
		}
		normalizeObjectModelName(argsMap, "lora", "model")
		normalizeObjectModelName(argsMap, "controlnet", "model")
		normalizeObjectModelName(argsMap, "refiner", "model")
		normalizeStringModelName(argsMap, "unet")
		normalizeStringModelName(argsMap, "vae")
		normalizeStringModelName(argsMap, "textual_inversion")
		normalizeComponentsRepos(argsMap)
	case TaskTypeLLM:
		if model, ok := argsMap["model"].(string); ok {
			argsMap["model"] = NormalizeModelName(model)
		}
	case TaskTypeSDFTLora:
		normalizeObjectModelName(argsMap, "model", "name")
	}

	normalized, err := json.Marshal(argsMap)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func normalizeStringModelName(argsMap map[string]interface{}, key string) {
	if name, ok := argsMap[key].(string); ok {
		argsMap[key] = NormalizeModelName(name)
	}
}

func normalizeObjectModelName(argsMap map[string]interface{}, objectKey, nameKey string) {
	object, ok := argsMap[objectKey].(map[string]interface{})
	if !ok {
		return
	}
	if name, ok := object[nameKey].(string); ok {
		object[nameKey] = NormalizeModelName(name)
	}
}

func normalizeComponentsRepos(argsMap map[string]interface{}) {
	components, ok := argsMap["components"].(map[string]interface{})
	if !ok {
		return
	}
	for _, raw := range components {
		object, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if repo, ok := object["repo"].(string); ok {
			object["repo"] = NormalizeModelName(repo)
		}
	}
}
