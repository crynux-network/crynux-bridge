package models_test

import (
	"crynux_bridge/models"
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestValidateSDTaskArgs(t *testing.T) {

	jsonStr := `{"base_model":"crynux-network/sdxl-turbo","prompt":"a lazy man sitting on a brown chair","negative_prompt":"","lora":null,"controlnet":null,"task_config":{"image_width":512,"image_height":512,"steps":1,"num_images":1,"seed":70813424,"safety_checker":false,"cfg":0},"scheduler":{"method":"EulerAncestralDiscreteScheduler","args":{"timestep_spacing":"trailing"}}}`
	sdInferenceTaskSchema, err := jsonschema.Compile("https://raw.githubusercontent.com/crynux-ai/stable-diffusion-task/main/schema/stable-diffusion-inference-task.json")

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
	jsonStr := `{"base_model":"crynux-ai/sdxl-turbo","prompt":"a lazy man sitting on a brown chair","negative_prompt":"","lora":null,"controlnet":null,"task_config":{"image_width":512,"image_height":512,"steps":1,"num_images":1,"seed":70813424,"safety_checker":false,"cfg":0},"scheduler":{"method":"EulerAncestralDiscreteScheduler","args":{"timestep_spacing":"trailing"}}}`
	baseModel, err := models.GetSDTaskConfigBaseModel(jsonStr)
	if err != nil {
		t.Fatalf("failed to get base model: %v", err)
	}
	t.Logf("base model: %s", baseModel)
}

func TestGetSDTaskConfigModelIDs(t *testing.T) {
	jsonStr := `{"base_model":"crynux-ai/sdxl-turbo","prompt":"a lazy man sitting on a brown chair","negative_prompt":"","lora":null,"controlnet":null,"task_config":{"image_width":512,"image_height":512,"steps":1,"num_images":1,"seed":70813424,"safety_checker":false,"cfg":0},"scheduler":{"method":"EulerAncestralDiscreteScheduler","args":{"timestep_spacing":"trailing"}}}`
	modelIDs, err := models.GetTaskConfigModelIDs(jsonStr, models.TaskTypeSD)
	if err != nil {
		t.Fatalf("failed to get model ids: %v", err)
	}
	t.Logf("model ids: %v", modelIDs)
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
	// Test JSON with null values
	jsonStr := `{
		"base_model": null,
		"lora": {
			"model": null
		},
		"controlnet": null,
		"task_config": {
			"num_images": null
		}
	}`

	var taskArgsMap map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &taskArgsMap)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Test that null values are properly detected
	if !models.IsNil(taskArgsMap["base_model"]) {
		t.Error("base_model should be nil")
	}

	if !models.IsNil(taskArgsMap["controlnet"]) {
		t.Error("controlnet should be nil")
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

	if !models.IsNil(taskConfig["num_images"]) {
		t.Error("task_config.num_images should be nil")
	}
}

func TestGetTaskConfigNumImagesWithNull(t *testing.T) {
	// Test with null num_images
	jsonStr := `{
		"task_config": {
			"num_images": null
		}
	}`

	_, err := models.GetTaskConfigNumImages(jsonStr)
	if err == nil {
		t.Error("Expected error when num_images is null")
	}
	if err.Error() != "num_images is missing or null" {
		t.Errorf("Expected error message about null num_images, got: %v", err)
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
	if err.Error() != "base_model is missing or null" {
		t.Errorf("Expected error message about null base_model, got: %v", err)
	}
}
