package relay

import (
	"crynux_bridge/models"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildCreateTaskInputTimeoutByTaskType(t *testing.T) {
	for _, taskType := range []models.ChainTaskType{models.TaskTypeSD, models.TaskTypeLLM} {
		input := buildCreateTaskInput(&models.InferenceTask{
			TaskType: taskType,
			Timeout:  123,
		}, "1")
		if input.Timeout != nil {
			t.Fatalf("task type %d must omit timeout", taskType)
		}
		payload, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), `"timeout"`) {
			t.Fatalf("task type %d JSON contains timeout: %s", taskType, payload)
		}
	}

	input := buildCreateTaskInput(&models.InferenceTask{
		TaskType: models.TaskTypeSDFTLora,
		Timeout:  456,
	}, "1")
	if input.Timeout == nil || *input.Timeout != 456 {
		t.Fatalf("SDFT timeout = %v, want 456", input.Timeout)
	}
	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"timeout":456`) {
		t.Fatalf("SDFT JSON does not contain timeout: %s", payload)
	}
}
