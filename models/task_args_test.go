package models_test

import (
	"crynux_bridge/models"
	"encoding/json"
	"slices"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

const sdTaskArgsJSON = `{"version":"3.7.0","base_model":{"name":"crynux-network/sdxl-turbo"},"prompt":"a lazy man sitting on a brown chair","negative_prompt":"","task_config":{"image_width":512,"image_height":512,"steps":1,"num_images_per_prompt":2,"seed":70813424,"guidance_scale":0}}`

func TestValidateSDTaskArgs(t *testing.T) {

	jsonStr := sdTaskArgsJSON
	sdInferenceTaskSchema, err := jsonschema.Compile("https://raw.githubusercontent.com/crynux-network/stable-diffusion-task/main/schema/image-task.json")

	if err != nil {
		t.Fatalf("failed to compile schema: %v", err)
	}

	var v interface{}
	if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
		t.Fatalf("failed to unmarshal json: %v", err)
	}

	validationError := sdInferenceTaskSchema.Validate(v)

	if validationError != nil {
		t.Fatalf("validation error: %v", validationError)
	}
}

func TestGetSDTaskConfigBaseModel(t *testing.T) {
	baseModel, err := models.GetSDTaskConfigBaseModel(sdTaskArgsJSON)
	if err != nil {
		t.Fatalf("failed to get base model: %v", err)
	}
	if baseModel != "crynux-network/sdxl-turbo" {
		t.Fatalf("base model = %q", baseModel)
	}
}

func TestGetSDTaskConfigModelIDs(t *testing.T) {
	modelIDs, err := models.GetTaskConfigModelIDs(sdTaskArgsJSON, models.TaskTypeSD)
	if err != nil {
		t.Fatalf("failed to get model ids: %v", err)
	}
	if !slices.Equal(modelIDs, []string{"base:crynux-network/sdxl-turbo"}) {
		t.Fatalf("model ids = %v", modelIDs)
	}
}

func TestGetSDTaskConfigModelIDsIncludesLoraAndComponents(t *testing.T) {
	jsonStr := `{"base_model":{"name":"a/base","revision":"abc"},"lora":{"model":"a/lora"},"components":{"vae":{"format":"diffusers","repo":"a/vae"},"text_encoder":{"format":"diffusers","repo":"a/te","revision":"def"}},"prompt":"p","task_config":{}}`
	modelIDs, err := models.GetTaskConfigModelIDs(jsonStr, models.TaskTypeSD)
	if err != nil {
		t.Fatalf("failed to get model ids: %v", err)
	}
	want := []string{"base:a/base", "lora:a/lora", "component:text_encoder:a/te", "component:vae:a/vae"}
	if !slices.Equal(modelIDs, want) {
		t.Fatalf("model ids = %v, want %v", modelIDs, want)
	}
}

func TestGetLLMTaskConfigModelIDsIgnoresRevision(t *testing.T) {
	modelIDs, err := models.GetTaskConfigModelIDs(`{"model":"a/llm","revision":"abc","messages":[]}`, models.TaskTypeLLM)
	if err != nil {
		t.Fatalf("failed to get model ids: %v", err)
	}
	if !slices.Equal(modelIDs, []string{"base:a/llm"}) {
		t.Fatalf("model ids = %v", modelIDs)
	}
}

func TestGetTaskConfigNumImages(t *testing.T) {
	num, err := models.GetTaskConfigNumImages(sdTaskArgsJSON)
	if err != nil || num != 2 {
		t.Fatalf("num images = %d, err = %v", num, err)
	}
	num, err = models.GetTaskConfigNumImages(`{"base_model":{"name":"a/base"},"task_config":{}}`)
	if err != nil || num != 1 {
		t.Fatalf("default num images = %d, err = %v", num, err)
	}
}

func TestIsNil(t *testing.T) {
	// Test cases for isNil function
	tests := []struct {
		name     string
		input    interface{}
		expected bool
	}{
		{"nil interface{}", nil, true},
		{"nil string pointer", (*string)(nil), true},
		{"nil map", (map[string]interface{})(nil), true},
		{"empty string", "", false},
		{"zero int", 0, false},
		{"empty map", map[string]interface{}{}, false},
		{"json null as interface{}", json.RawMessage("null"), false}, // This will be parsed as nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := models.IsNil(tt.input)
			if result != tt.expected {
				t.Errorf("isNil(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestJsonNullHandling(t *testing.T) {
	jsonStr := `{
		"base_model": null,
		"lora": {
			"model": null
		},
		"components": null,
		"task_config": {
			"num_images_per_prompt": null
		}
	}`

	var taskArgsMap map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &taskArgsMap)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if !models.IsNil(taskArgsMap["base_model"]) {
		t.Error("base_model should be nil")
	}

	if !models.IsNil(taskArgsMap["components"]) {
		t.Error("components should be nil")
	}

	lora, ok := taskArgsMap["lora"].(map[string]interface{})
	if !ok {
		t.Fatal("lora should be a map")
	}

	if !models.IsNil(lora["model"]) {
		t.Error("lora.model should be nil")
	}

	taskConfig, ok := taskArgsMap["task_config"].(map[string]interface{})
	if !ok {
		t.Fatal("task_config should be a map")
	}

	if !models.IsNil(taskConfig["num_images_per_prompt"]) {
		t.Error("task_config.num_images_per_prompt should be nil")
	}
}

func TestGetTaskConfigNumImagesWithNull(t *testing.T) {
	num, err := models.GetTaskConfigNumImages(`{"task_config":{"num_images_per_prompt":null}}`)
	if err != nil || num != 1 {
		t.Errorf("null num_images_per_prompt = %d, err = %v, want default 1", num, err)
	}
}

func TestGetSDTaskConfigBaseModelWithNull(t *testing.T) {
	// Test with null base_model
	jsonStr := `{
		"base_model": null
	}`

	_, err := models.GetSDTaskConfigBaseModel(jsonStr)
	if err == nil {
		t.Error("Expected error when base_model is null")
	}
	if err.Error() != "base_model.name is missing or empty" {
		t.Errorf("Expected error message about null base_model, got: %v", err)
	}
}
