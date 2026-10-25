package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeModelName(t *testing.T) {
	if got := NormalizeModelName("Qwen/Qwen-Image-2.1"); got != "qwen/qwen-image-2.1" {
		t.Fatalf("got %q", got)
	}
	url := "https://Example.com/Org/Repo"
	if got := NormalizeModelName(url); got != url {
		t.Fatalf("url got %q", got)
	}
}

func assertTaskArgsJSON(t *testing.T, taskArgs string, taskType ChainTaskType, wantArgs string) {
	t.Helper()
	got, err := NormalizeTaskArgsModelNames(taskArgs, taskType)
	if err != nil {
		t.Fatalf("NormalizeTaskArgsModelNames error: %v", err)
	}
	var gotMap, wantMap map[string]interface{}
	if err := json.Unmarshal([]byte(got), &gotMap); err != nil {
		t.Fatalf("normalized task args is not valid json: %v", err)
	}
	if err := json.Unmarshal([]byte(wantArgs), &wantMap); err != nil {
		t.Fatalf("want task args is not valid json: %v", err)
	}
	if !reflect.DeepEqual(gotMap, wantMap) {
		t.Fatalf("unexpected normalized task args, got %s, want %s", got, wantArgs)
	}
}

func TestNormalizeTaskArgsModelNamesLLM(t *testing.T) {
	taskArgs := `{"model":"Qwen/Qwen3.5-9B","messages":[{"role":"user","content":"Hi"}],"generation_config":{"temperature":0.8},"seed":42}`
	wantArgs := `{"model":"qwen/qwen3.5-9b","messages":[{"role":"user","content":"Hi"}],"generation_config":{"temperature":0.8},"seed":42}`
	assertTaskArgsJSON(t, taskArgs, TaskTypeLLM, wantArgs)
}

func TestNormalizeTaskArgsModelNamesSD(t *testing.T) {
	taskArgs := `{"base_model":{"name":"Qwen/Qwen-Image-2.1"},"prompt":"a cat","lora":{"model":"Org/LoRA"},"components":{"transformer":{"format":"gguf","repo":"Unsloth/Qwen-Image-2.1-GGUF","format_kwargs":{"filename":"qwen-image-2.1-Q4_K_M.gguf"}}},"task_config":{"num_images":1}}`
	wantArgs := `{"base_model":{"name":"qwen/qwen-image-2.1"},"prompt":"a cat","lora":{"model":"org/lora"},"components":{"transformer":{"format":"gguf","repo":"unsloth/qwen-image-2.1-gguf","format_kwargs":{"filename":"qwen-image-2.1-Q4_K_M.gguf"}}},"task_config":{"num_images":1}}`
	assertTaskArgsJSON(t, taskArgs, TaskTypeSD, wantArgs)
}

func TestNormalizeTaskArgsModelNamesSDStringBaseModel(t *testing.T) {
	taskArgs := `{"base_model":"Crynux-Network/SDXL-Turbo","prompt":"a cat","task_config":{"num_images":1}}`
	wantArgs := `{"base_model":"crynux-network/sdxl-turbo","prompt":"a cat","task_config":{"num_images":1}}`
	assertTaskArgsJSON(t, taskArgs, TaskTypeSD, wantArgs)
}

func TestNormalizeTaskArgsModelNamesSDURLLora(t *testing.T) {
	taskArgs := `{"base_model":"Crynux-Network/SDXL-Turbo","prompt":"a cat","lora":{"model":"https://example.com/Models/MyLora.safetensors"},"task_config":{"num_images":1}}`
	wantArgs := `{"base_model":"crynux-network/sdxl-turbo","prompt":"a cat","lora":{"model":"https://example.com/Models/MyLora.safetensors"},"task_config":{"num_images":1}}`
	assertTaskArgsJSON(t, taskArgs, TaskTypeSD, wantArgs)
}

func TestNormalizeTaskArgsModelNamesSDFTLora(t *testing.T) {
	taskArgs := `{"model":{"name":"Crynux-Network/Stable-Diffusion-v1-5","variant":"fp16"},"dataset_name":"lambdalabs/naruto-blip-captions"}`
	wantArgs := `{"model":{"name":"crynux-network/stable-diffusion-v1-5","variant":"fp16"},"dataset_name":"lambdalabs/naruto-blip-captions"}`
	assertTaskArgsJSON(t, taskArgs, TaskTypeSDFTLora, wantArgs)
}

func TestNormalizeTaskArgsModelNamesPreservesNumberLiterals(t *testing.T) {
	taskArgs := `{"model":"Org/LLM","quantization_config":{"quant_method":"bitsandbytes","quant_kwargs":{"llm_int8_threshold":6.0,"load_in_8bit":true}}}`
	got, err := NormalizeTaskArgsModelNames(taskArgs, TaskTypeLLM)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"llm_int8_threshold":6.0`) {
		t.Fatalf("expected 6.0 literal preserved, got %s", got)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["model"] != "org/llm" {
		t.Fatalf("model = %#v", payload["model"])
	}
}
