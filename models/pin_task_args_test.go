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
