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

func TestBuildCreateTaskInputPreservesWeiTaskFee(t *testing.T) {
	const taskFeeWei = "123456789012345"
	input := buildCreateTaskInput(&models.InferenceTask{}, taskFeeWei)
	if input.TaskFee != taskFeeWei {
		t.Fatalf("task fee = %q, want %q", input.TaskFee, taskFeeWei)
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

func TestBuildCreateTaskInputOmitsDerivedParallelFields(t *testing.T) {
	plain, err := json.Marshal(buildCreateTaskInput(&models.InferenceTask{
		TaskModelIDs: models.StringArray{"base:org/model"},
		TaskVersion:  "3.7.0",
		TaskSize:     2,
		TaskArgs:     `{"version":"3.7.0","model":"org/model","messages":[]}`,
	}, "1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"task_model_ids", "task_version", "task_size", "required_model_files"} {
		if strings.Contains(string(plain), field) {
			t.Fatalf("create input must omit %s: %s", field, plain)
		}
	}
	if !strings.Contains(string(plain), `"task_args":"{\"version\":\"3.7.0\",\"model\":\"org/model\",\"messages\":[]}"`) &&
		!strings.Contains(string(plain), `"task_args":`) {
		t.Fatalf("create input must keep task_args: %s", plain)
	}
}

func TestBatchStatusItemDecodesExecutionModelFiles(t *testing.T) {
	var item BatchStatusItem
	payload := `{"task_id_commitment":"0x1","found":true,"execution_gpu":"gpu","execution_gpu_vram":24,"execution_model_files":[{"source_type":"huggingface","repository":"org/model","revision":"abc","format":"","format_kwargs":{}}]}`
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		t.Fatal(err)
	}
	if len(item.ExecutionModelFiles) != 1 || item.ExecutionModelFiles[0].Repository != "org/model" || item.ExecutionModelFiles[0].Revision != "abc" {
		t.Fatalf("execution model files = %#v", item.ExecutionModelFiles)
	}
}
