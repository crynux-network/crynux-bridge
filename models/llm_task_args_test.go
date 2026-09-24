package models

import (
	"encoding/json"
	"testing"
)

func TestGPTTaskArgsPreservesStructuredOutputFields(t *testing.T) {
	input := []byte(`{"model":"m","messages":[],"tools":[{"type":"function","function":{"name":"f","strict":true}}],"tool_choice":{"type":"function","function":{"name":"f"}},"response_format":{"type":"json_object"},"seed":0}`)
	var args GPTTaskArgs
	if err := json.Unmarshal(input, &args); err != nil {
		t.Fatal(err)
	}
	output, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]interface{}
	if err := json.Unmarshal(output, &value); err != nil {
		t.Fatal(err)
	}
	if value["tool_choice"] == nil || value["response_format"] == nil {
		t.Fatalf("structured fields missing from %s", output)
	}
}
