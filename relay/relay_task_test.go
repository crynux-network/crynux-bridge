package relay

import (
	"crynux_bridge/models"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
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

func TestBuildAbortTaskInputUsesCreatorCancelled(t *testing.T) {
	input := buildAbortTaskInput("0xcommitment")
	if input.TaskIDCommitment != "0xcommitment" {
		t.Fatalf("task_id_commitment = %q, want 0xcommitment", input.TaskIDCommitment)
	}
	if input.AbortReason != models.TaskAbortCreatorCancelled {
		t.Fatalf("abort_reason = %d, want %d", input.AbortReason, models.TaskAbortCreatorCancelled)
	}

	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pkHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	timestamp, signature, err := SignData(input, pkHex)
	if err != nil {
		t.Fatal(err)
	}
	if timestamp == 0 {
		t.Fatal("timestamp must be set")
	}
	if signature == "" {
		t.Fatal("signature must be set")
	}

	input.Timestamp = timestamp
	input.Signature = signature
	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"abort_reason":7`) {
		t.Fatalf("abort payload missing creator-cancelled reason: %s", payload)
	}
	if !strings.Contains(string(payload), `"task_id_commitment":"0xcommitment"`) {
		t.Fatalf("abort payload missing commitment: %s", payload)
	}
}
