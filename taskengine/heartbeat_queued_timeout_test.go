package taskengine

import (
	"context"
	"crynux_bridge/models"
	"crynux_bridge/relay"
	"encoding/json"
	"testing"
	"time"

	"gorm.io/gorm"
)

func createHeartbeatClientAndTask(t *testing.T, db *gorm.DB, createdAt time.Time, commitment string) (models.Client, models.InferenceTask) {
	t.Helper()
	client := models.Client{ClientId: HeartbeatClientID}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	clientTask := models.ClientTask{ClientID: client.ID}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatal(err)
	}
	task := models.InferenceTask{
		RootModel:        models.RootModel{CreatedAt: createdAt, UpdatedAt: createdAt},
		ClientID:         client.ID,
		ClientTaskID:     clientTask.ID,
		TaskType:         models.TaskTypeLLM,
		TaskModelIDs:     models.StringArray{"base:test"},
		QuantizationKey:  "quant-a",
		TaskIDCommitment: commitment,
		Status:           models.InferenceTaskCreated,
		RelayChecked:     true,
		RelayFound:       true,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&task).Updates(map[string]any{
		"created_at": createdAt,
		"updated_at": createdAt,
		"status":     models.InferenceTaskCreated,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	return client, task
}

func applyStatus(t *testing.T, engine *Engine, db *gorm.DB, task *models.InferenceTask, status relay.BatchStatusItem) {
	t.Helper()
	if err := db.Transaction(func(tx *gorm.DB) error {
		current := *task
		if err := tx.First(&current, task.ID).Error; err != nil {
			return err
		}
		if err := engine.applyRelayStatus(context.Background(), tx, &current, status); err != nil {
			return err
		}
		*task = current
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
}

func TestHeartbeatQueuedTimeoutAbortsLocally(t *testing.T) {
	engine, db := newTestEngine(t)
	engine.config.HeartbeatMaxQueuedDuration = time.Minute
	_, task := createHeartbeatClientAndTask(t, db, time.Now().Add(-2*time.Minute), "0xabc")

	applyStatus(t, engine, db, &task, relay.BatchStatusItem{
		Found:            true,
		Status:           models.ChainTaskQueued,
		TaskIDCommitment: task.TaskIDCommitment,
	})

	if task.Status != models.InferenceTaskEndAborted {
		t.Fatalf("status = %d, want EndAborted", task.Status)
	}
	if task.AbortReason != models.TaskAbortCreatorCancelled {
		t.Fatalf("abort_reason = %d, want CreatorCancelled", task.AbortReason)
	}
	if !task.SiblingCancellationRequired {
		t.Fatal("expected sibling_cancellation_required for Relay cancel")
	}
}

func TestHeartbeatQueuedTimeoutNotReachedKeepsCreated(t *testing.T) {
	engine, db := newTestEngine(t)
	engine.config.HeartbeatMaxQueuedDuration = time.Minute
	_, task := createHeartbeatClientAndTask(t, db, time.Now().Add(-10*time.Second), "0xabc")

	applyStatus(t, engine, db, &task, relay.BatchStatusItem{
		Found:            true,
		Status:           models.ChainTaskQueued,
		TaskIDCommitment: task.TaskIDCommitment,
	})

	if task.Status != models.InferenceTaskCreated {
		t.Fatalf("status = %d, want Created", task.Status)
	}
	if task.SiblingCancellationRequired {
		t.Fatal("did not expect cancel flag before timeout")
	}
}

func TestHeartbeatQueuedTimeoutSkipsNonHeartbeatClient(t *testing.T) {
	engine, db := newTestEngine(t)
	engine.config.HeartbeatMaxQueuedDuration = time.Minute
	client := models.Client{ClientId: "api-client"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	clientTask := models.ClientTask{ClientID: client.ID}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now().Add(-2 * time.Minute)
	task := models.InferenceTask{
		RootModel:        models.RootModel{CreatedAt: createdAt, UpdatedAt: createdAt},
		ClientID:         client.ID,
		ClientTaskID:     clientTask.ID,
		TaskType:         models.TaskTypeLLM,
		TaskModelIDs:     models.StringArray{"base:test"},
		QuantizationKey:  "quant-a",
		TaskIDCommitment: "0xabc",
		Status:           models.InferenceTaskCreated,
		RelayChecked:     true,
		RelayFound:       true,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&task).Updates(map[string]any{
		"created_at": createdAt,
		"status":     models.InferenceTaskCreated,
	}).Error; err != nil {
		t.Fatal(err)
	}

	applyStatus(t, engine, db, &task, relay.BatchStatusItem{
		Found:            true,
		Status:           models.ChainTaskQueued,
		TaskIDCommitment: task.TaskIDCommitment,
	})

	if task.Status != models.InferenceTaskCreated {
		t.Fatalf("status = %d, want Created for non-heartbeat client", task.Status)
	}
	if task.SiblingCancellationRequired {
		t.Fatal("did not expect cancel flag for non-heartbeat client")
	}
}

func TestHeartbeatQueuedTimeoutDisabledWhenZero(t *testing.T) {
	engine, db := newTestEngine(t)
	engine.config.HeartbeatMaxQueuedDuration = 0
	_, task := createHeartbeatClientAndTask(t, db, time.Now().Add(-2*time.Hour), "0xabc")

	applyStatus(t, engine, db, &task, relay.BatchStatusItem{
		Found:            true,
		Status:           models.ChainTaskQueued,
		TaskIDCommitment: task.TaskIDCommitment,
	})

	if task.Status != models.InferenceTaskCreated {
		t.Fatalf("status = %d, want Created when limit disabled", task.Status)
	}
}

func TestHeartbeatQueuedTimeoutStatusSyncDoesNotOverwriteEndAborted(t *testing.T) {
	engine, db := newTestEngine(t)
	engine.config.HeartbeatMaxQueuedDuration = time.Minute
	_, task := createHeartbeatClientAndTask(t, db, time.Now().Add(-2*time.Minute), "0xabc")

	applyStatus(t, engine, db, &task, relay.BatchStatusItem{
		Found:            true,
		Status:           models.ChainTaskQueued,
		TaskIDCommitment: task.TaskIDCommitment,
	})
	if !task.SiblingCancellationRequired {
		t.Fatal("expected cancel flag after timeout")
	}

	applyStatus(t, engine, db, &task, relay.BatchStatusItem{
		Found:            true,
		Status:           models.ChainTaskQueued,
		TaskIDCommitment: task.TaskIDCommitment,
	})

	if task.Status != models.InferenceTaskEndAborted {
		t.Fatalf("status = %d, want EndAborted after second sync", task.Status)
	}
	if task.AbortReason != models.TaskAbortCreatorCancelled {
		t.Fatalf("abort_reason = %d, want CreatorCancelled", task.AbortReason)
	}
	if !task.SiblingCancellationRequired {
		t.Fatal("expected cancel flag to remain until cancel completes")
	}
}

func TestHeartbeatQueuedTimeoutCancelKeepsEndAbortedAndDropsPending(t *testing.T) {
	engine, db := newTestEngine(t)
	engine.config.HeartbeatMaxQueuedDuration = time.Minute
	_, task := createHeartbeatClientAndTask(t, db, time.Now().Add(-2*time.Minute), "0xabc")

	applyStatus(t, engine, db, &task, relay.BatchStatusItem{
		Found:            true,
		Status:           models.ChainTaskQueued,
		TaskIDCommitment: task.TaskIDCommitment,
	})

	count, err := engine.CountPending(context.Background(), HeartbeatClientID, models.TaskTypeLLM, "base:test", "quant-a")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("pending count = %d, want 0 after local EndAborted", count)
	}

	result, err := json.Marshal(mutationOperationResult{Outcome: relay.BatchOutcomeNotCancellable})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&task).Updates(map[string]any{
		"operation_type":   models.TaskOperationCancel,
		"operation_status": models.TaskOperationStatusCompleted,
		"operation_result": string(result),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := engine.applyOperationResult(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != models.InferenceTaskEndAborted {
		t.Fatalf("status = %d, want EndAborted after cancel", task.Status)
	}
	if task.AbortReason != models.TaskAbortCreatorCancelled {
		t.Fatalf("abort_reason = %d, want CreatorCancelled", task.AbortReason)
	}
	if task.SiblingCancellationRequired {
		t.Fatal("expected cancel flag cleared after cancel result")
	}
}

func TestConfigRejectsNegativeHeartbeatMaxQueuedDuration(t *testing.T) {
	cfg := testConfig(t)
	cfg.HeartbeatMaxQueuedDuration = -time.Second
	if err := cfg.validate(); err == nil {
		t.Fatal("expected negative HeartbeatMaxQueuedDuration error")
	}
}
