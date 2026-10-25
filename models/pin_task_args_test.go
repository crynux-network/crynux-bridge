package models

import (
	"encoding/json"
	"testing"
)

func TestPinTaskArgsRevisionsLLM(t *testing.T) {
	files := ModelFileEntries{{
		SourceType: "huggingface",
		Repository: "org/model",
		Revision:   "sha-llm",
		Format:     "",
	}}
	pinned, err := PinTaskArgsRevisions(
		`{"version":"3.7.0","model":"org/model","revision":"","messages":[]}`,
		TaskTypeLLM,
		files,
	)
	if err != nil {
		t.Fatal(err)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(pinned), &args); err != nil {
		t.Fatal(err)
	}
	if args["revision"] != "sha-llm" {
		t.Fatalf("revision = %#v", args["revision"])
	}
	if args["model"] != "org/model" || args["version"] != "3.7.0" {
		t.Fatalf("unexpected pinned args: %#v", args)
	}
}

func TestPinTaskArgsRevisionsSD(t *testing.T) {
	files := ModelFileEntries{
		{SourceType: "huggingface", Repository: "org/base", Revision: "sha-base", Format: "diffusers"},
		{SourceType: "huggingface", Repository: "org/vae", Revision: "sha-vae", Format: "diffusers"},
	}
	pinned, err := PinTaskArgsRevisions(
		`{"version":"3.7.0","base_model":{"name":"org/base"},"components":{"vae":{"format":"diffusers","repo":"org/vae"}},"prompt":"p","task_config":{}}`,
		TaskTypeSD,
		files,
	)
	if err != nil {
		t.Fatal(err)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(pinned), &args); err != nil {
		t.Fatal(err)
	}
	base := args["base_model"].(map[string]any)
	if base["revision"] != "sha-base" {
		t.Fatalf("base revision = %#v", base["revision"])
	}
	vae := args["components"].(map[string]any)["vae"].(map[string]any)
	if vae["revision"] != "sha-vae" {
		t.Fatalf("vae revision = %#v", vae["revision"])
	}
}

func TestPinTaskArgsRevisionsRejectsUnmatchedFile(t *testing.T) {
	files := ModelFileEntries{{Repository: "org/other", Revision: "sha", Format: ""}}
	if _, err := PinTaskArgsRevisions(`{"model":"org/model","messages":[]}`, TaskTypeLLM, files); err == nil {
		t.Fatal("expected unmatched file to fail")
	}
}

func TestPinTaskArgsRevisionsMatchesNormalizedRepositoryCase(t *testing.T) {
	files := ModelFileEntries{
		{
			SourceType:   "huggingface",
			Repository:   "unsloth/qwen-image-2.1-gguf",
			Revision:     "sha-gguf",
			Format:       "gguf",
			FormatKwargs: map[string]any{"filename": "qwen-image-2.1-Q4_K_M.gguf"},
		},
		{
			SourceType: "huggingface",
			Repository: "qwen/qwen-image-2.1",
			Revision:   "sha-base",
			Format:     "diffusers",
		},
	}
	pinned, err := PinTaskArgsRevisions(
		`{"version":"3.7.0","base_model":{"name":"qwen/qwen-image-2.1"},"components":{"transformer":{"format":"gguf","repo":"unsloth/Qwen-Image-2.1-GGUF","format_kwargs":{"filename":"qwen-image-2.1-Q4_K_M.gguf"}}},"prompt":"p","task_config":{}}`,
		TaskTypeSD,
		files,
	)
	if err != nil {
		t.Fatal(err)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(pinned), &args); err != nil {
		t.Fatal(err)
	}
	base := args["base_model"].(map[string]any)
	if base["revision"] != "sha-base" {
		t.Fatalf("base revision = %#v", base["revision"])
	}
	transformer := args["components"].(map[string]any)["transformer"].(map[string]any)
	if transformer["revision"] != "sha-gguf" {
		t.Fatalf("transformer revision = %#v", transformer["revision"])
	}
}
