package taskengine

import (
	"context"
	"crynux_bridge/models"
	"crynux_bridge/relay"
	"crynux_bridge/utils"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/go-ecvrf"
	"gorm.io/gorm"
)

const testVRFPrivateKey = "0000000000000000000000000000000000000000000000000000000000000001"

func testModelFiles() models.ModelFileEntries {
	return models.ModelFileEntries{
		{
			SourceType:   "huggingface",
			Repository:   "org/model",
			Revision:     "0123456789abcdef0123456789abcdef01234567",
			Format:       "diffusers",
			FormatKwargs: map[string]any{"variant": "fp16"},
		},
		{
			SourceType:   "huggingface",
			Repository:   "org/vae",
			Revision:     "89abcdef0123456789abcdef0123456789abcdef",
			Format:       "diffusers",
			FormatKwargs: map[string]any{},
		},
	}
}

func sdTaskArgsForTestFiles() string {
	return `{"version":"3.7.0","base_model":{"name":"org/model"},"components":{"vae":{"format":"diffusers","repo":"org/vae"}},"prompt":"p","task_config":{}}`
}

// vrfValidationSeed returns a sampling seed whose VRF number selects validation.
func vrfValidationSeed(t *testing.T) string {
	t.Helper()
	privateKey, err := hexutil.Decode("0x" + testVRFPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	key := secp256k1.PrivKeyFromBytes(privateKey)
	for i := uint64(0); i < 100000; i++ {
		seed := make([]byte, 32)
		binary.BigEndian.PutUint64(seed, i)
		number, _, err := ecvrf.Secp256k1Sha256Tai.Prove(key.ToECDSA(), seed)
		if err != nil {
			t.Fatal(err)
		}
		if utils.VrfNeedValidation(number) {
			return hexutil.Encode(seed)
		}
	}
	t.Fatal("no validation seed found")
	return ""
}

func TestExecutionAssignedRequiresModelFilesAndLLMGPU(t *testing.T) {
	files := testModelFiles()
	tests := []struct {
		name     string
		taskType models.ChainTaskType
		status   relay.BatchStatusItem
		want     bool
	}{
		{"sd without files", models.TaskTypeSD, relay.BatchStatusItem{}, false},
		{"sd with files", models.TaskTypeSD, relay.BatchStatusItem{ExecutionModelFiles: files}, true},
		{"llm gpu only", models.TaskTypeLLM, relay.BatchStatusItem{SelectedExecutionGPU: "gpu"}, false},
		{"llm files only", models.TaskTypeLLM, relay.BatchStatusItem{ExecutionModelFiles: files}, false},
		{"llm gpu and files", models.TaskTypeLLM, relay.BatchStatusItem{SelectedExecutionGPU: "gpu", ExecutionModelFiles: files}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := executionAssigned(tt.taskType, tt.status); got != tt.want {
				t.Fatalf("executionAssigned = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewVSSMembersPinTaskArgsRevisionsAndExecutionGPU(t *testing.T) {
	files := models.ModelFileEntries{{
		SourceType: "huggingface",
		Repository: "org/model",
		Revision:   "0123456789abcdef0123456789abcdef01234567",
		Format:     "",
	}}
	task := &models.InferenceTask{
		ClientID:     1,
		ClientTaskID: 2,
		TaskArgs:     `{"version":"3.7.0","model":"org/model","revision":"","messages":[]}`,
		TaskType:     models.TaskTypeLLM,
		TaskVersion:  "3.7.0",
		TaskFee:      "1",
		TaskID:       "0x01",
		MinVram:      24,
		RequiredGPU:  "submitted",
	}
	status := relay.BatchStatusItem{
		SamplingSeed:             "0x02",
		SelectedExecutionGPU:     "NVIDIA GPU",
		SelectedExecutionGPUVram: 48,
		ExecutionModelFiles:      files,
	}
	members, err := newVSSMembers(task, status, "0x03", "0x04")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("member count = %d, want 2", len(members))
	}
	for i := range members {
		member := members[i]
		var args map[string]any
		if err := json.Unmarshal([]byte(member.TaskArgs), &args); err != nil {
			t.Fatal(err)
		}
		if args["revision"] != files[0].Revision {
			t.Fatalf("member %d revision = %#v", i, args["revision"])
		}
		if member.TaskArgs == task.TaskArgs {
			t.Fatalf("member %d task args were not pinned", i)
		}
		if len(member.RequiredModelFiles) != 0 {
			t.Fatalf("member %d must not set RequiredModelFiles", i)
		}
		if member.TaskVersion != task.TaskVersion || member.TaskID != task.TaskID {
			t.Fatalf("member %d identity = %#v", i, member)
		}
		if member.RequiredGPU != "NVIDIA GPU" || member.RequiredGPUVram != 48 {
			t.Fatalf("member %d gpu = %q/%d", i, member.RequiredGPU, member.RequiredGPUVram)
		}
		if member.Nonce == "" || member.TaskIDCommitment == "" {
			t.Fatalf("member %d has no commitment", i)
		}
	}
	if members[0].TaskIDCommitment == members[1].TaskIDCommitment {
		t.Fatal("members share one commitment")
	}
}

func TestNewVSSMembersKeepSubmittedGPUForSD(t *testing.T) {
	task := &models.InferenceTask{
		TaskType:        models.TaskTypeSD,
		TaskID:          "0x01",
		RequiredGPU:     "submitted",
		RequiredGPUVram: 16,
		TaskArgs:        `{"version":"3.7.0","base_model":{"name":"org/model"},"prompt":"p","task_config":{}}`,
	}
	status := relay.BatchStatusItem{
		SelectedExecutionGPU: "other",
		ExecutionModelFiles: models.ModelFileEntries{{
			SourceType: "huggingface",
			Repository: "org/model",
			Revision:   "0123456789abcdef0123456789abcdef01234567",
			Format:     "diffusers",
		}},
	}
	members, err := newVSSMembers(task, status, "0x03", "0x04")
	if err != nil {
		t.Fatal(err)
	}
	for i := range members {
		if members[i].RequiredGPU != "submitted" || members[i].RequiredGPUVram != 16 {
			t.Fatalf("member %d gpu = %q/%d", i, members[i].RequiredGPU, members[i].RequiredGPUVram)
		}
	}
}

func TestPersistVRFWaitsForExecutionModelFilesBeforeCreatingMembers(t *testing.T) {
	engine, db := newTestEngine(t)
	engine.config.PrivateKey = testVRFPrivateKey
	seed := vrfValidationSeed(t)
	task := models.InferenceTask{
		TaskType: models.TaskTypeSD,
		TaskID:   "0x01",
		TaskArgs: sdTaskArgsForTestFiles(),
		Status:   models.InferenceTaskStarted,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	apply := func(status relay.BatchStatusItem) {
		t.Helper()
		if err := db.Transaction(func(tx *gorm.DB) error {
			current := task
			return engine.applyRelayStatus(context.Background(), tx, &current, status)
		}); err != nil {
			t.Fatal(err)
		}
	}
	count := func() int64 {
		t.Helper()
		var n int64
		if err := db.Model(&models.InferenceTask{}).Where("task_id = ?", task.TaskID).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}

	apply(relay.BatchStatusItem{Found: true, Status: models.ChainTaskStarted, SamplingSeed: seed})
	if count() != 1 {
		t.Fatalf("members created before execution model files were reported: %d", count())
	}
	var stored models.InferenceTask
	if err := db.First(&stored, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SamplingSeed != "" {
		t.Fatal("VRF data persisted before validation members could be created")
	}

	files := testModelFiles()
	apply(relay.BatchStatusItem{Found: true, Status: models.ChainTaskStarted, SamplingSeed: seed, ExecutionModelFiles: files})
	if count() != 3 {
		t.Fatalf("task group size = %d, want 3", count())
	}
	if err := db.First(&stored, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	wantKey, _ := files.CanonicalKey()
	gotKey, _ := stored.SelectedExecutionModelFiles.CanonicalKey()
	if gotKey != wantKey {
		t.Fatalf("primary execution model files = %#v", stored.SelectedExecutionModelFiles)
	}
	var members []models.InferenceTask
	if err := db.Where("task_id = ? AND id <> ?", task.TaskID, task.ID).Find(&members).Error; err != nil {
		t.Fatal(err)
	}
	for i := range members {
		if len(members[i].RequiredModelFiles) != 0 {
			t.Fatalf("member %d must not set RequiredModelFiles", i)
		}
		var args map[string]any
		if err := json.Unmarshal([]byte(members[i].TaskArgs), &args); err != nil {
			t.Fatal(err)
		}
		base := args["base_model"].(map[string]any)
		if base["revision"] != files[0].Revision {
			t.Fatalf("member %d base revision = %#v", i, base["revision"])
		}
		vae := args["components"].(map[string]any)["vae"].(map[string]any)
		if vae["revision"] != files[1].Revision {
			t.Fatalf("member %d vae revision = %#v", i, vae["revision"])
		}
	}
}

func TestExecuteValidationBatchRefusesInconsistentExecutionModelFiles(t *testing.T) {
	engine, db := newTestEngine(t)
	engine.config.PrivateKey = testVRFPrivateKey
	files := testModelFiles()
	other := models.ModelFileEntries{files[0]}
	group := []models.InferenceTask{
		{TaskType: models.TaskTypeSD, TaskID: "group", Sequence: 1, Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: files},
		{TaskType: models.TaskTypeSD, TaskID: "group", Sequence: 2, Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: files},
		{TaskType: models.TaskTypeSD, TaskID: "group", Sequence: 3, Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: other},
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.InferenceTask{}).Where("task_id = ?", "group").Updates(map[string]any{
		"operation_type":   models.TaskOperationValidate,
		"operation_status": models.TaskOperationStatusRunning,
	}).Error; err != nil {
		t.Fatal(err)
	}
	// An unreachable Relay would fail the batch, so a stored permanent error proves the group never reached Relay.
	batch := []operationDecision{{task: group[0], operation: models.TaskOperationValidate, group: group}}
	if err := engine.executeValidationBatch(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	var owner models.InferenceTask
	if err := db.First(&owner, group[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if owner.OperationStatus != models.TaskOperationStatusCompleted {
		t.Fatalf("operation status = %q", owner.OperationStatus)
	}
	var result mutationOperationResult
	if err := json.Unmarshal([]byte(owner.OperationResult), &result); err != nil {
		t.Fatal(err)
	}
	if result.Outcome != relay.BatchOutcomePermanentError {
		t.Fatalf("outcome = %q, want permanent error", result.Outcome)
	}
}
