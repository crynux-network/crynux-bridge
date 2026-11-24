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

// IsCommitSHA reports whether revision is a 40-character hexadecimal Git commit.
func IsCommitSHA(revision string) bool {
	if len(revision) != 40 {
		return false
	}
	for i := 0; i < len(revision); i++ {
		c := revision[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// NormalizeCommitRevision lowercases a 40-character hex commit SHA. Branch, tag,
// main, and empty revisions are returned unchanged.
func NormalizeCommitRevision(revision string) string {
	if IsCommitSHA(revision) {
		return strings.ToLower(revision)
	}
	return revision
}

// NormalizeModelID lowercases a model ID prefix and repository name. URL-based
// repository names keep their original case. Component names are lowercased.
// A 40-character hex commit in @revision is lowercased; weight file names after
// # keep their original case.
func NormalizeModelID(modelID string) string {
	prefix, rest, ok := strings.Cut(modelID, ":")
	if !ok {
		return strings.ToLower(modelID)
	}
	lowerPrefix := strings.ToLower(prefix)
	if strings.HasPrefix(rest, "http://") || strings.HasPrefix(rest, "https://") {
		return lowerPrefix + ":" + rest
	}
	if lowerPrefix == "component" {
		compName, repo, hasRepo := strings.Cut(rest, ":")
		if hasRepo {
			return "component:" + strings.ToLower(compName) + ":" + NormalizeModelName(repo)
		}
	}
	repoAndMeta, weightFile, hasWeight := strings.Cut(rest, "#")
	repoPart, revPart, hasRev := strings.Cut(repoAndMeta, "@")
	normalized := lowerPrefix + ":" + strings.ToLower(repoPart)
	if hasRev && revPart != "" {
		normalized += "@" + NormalizeCommitRevision(revPart)
	}
	if hasWeight && weightFile != "" {
		normalized += "#" + weightFile
	}
	return normalized
}

// NormalizeModelIDs applies NormalizeModelID to every entry.
func NormalizeModelIDs(modelIDs []string) []string {
	normalized := make([]string, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		normalized = append(normalized, NormalizeModelID(modelID))
	}
	return normalized
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
	case TaskTypeVideo:
		normalizeObjectModelName(argsMap, "base_model", "name")
		normalizeComponentsRepos(argsMap)
		normalizeVideoLoras(argsMap)
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

func normalizeVideoLoras(argsMap map[string]interface{}) {
	loras, ok := argsMap["loras"].([]interface{})
	if !ok {
		return
	}
	for _, item := range loras {
		object, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if modelName, ok := object["model"].(string); ok {
			object["model"] = NormalizeModelName(modelName)
		}
	}
}
