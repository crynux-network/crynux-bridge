package models_test

import (
	"context"
	"crynux_bridge/models"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTaskAbortReasonMatchesRelayValues(t *testing.T) {
	values := []models.TaskAbortReason{
		models.TaskAbortReasonNone,
		models.TaskAbortTimeout,
		models.TaskAbortModelDownloadFailed,
		models.TaskAbortIncorrectResult,
		models.TaskAbortTaskFeeTooLow,
		models.TaskAbortGroupTimeout,
		models.TaskAbortErrorReported,
		models.TaskAbortCreatorCancelled,
		models.TaskAbortCreatorValidationTimeout,
		models.TaskAbortResultUploadTimeout,
		models.TaskAbortNodeSlashed,
	}
	for expected, value := range values {
		if int(value) != expected {
			t.Fatalf("abort reason at index %d has value %d", expected, value)
		}
	}
}

func TestRelayTerminalTaskStatus(t *testing.T) {
	terminal := []models.TaskStatus{
		models.InferenceTaskEndAborted,
		models.InferenceTaskEndGroupRefund,
		models.InferenceTaskEndInvalidated,
		models.InferenceTaskEndSuccess,
		models.InferenceTaskResultDownloaded,
	}
	for _, status := range terminal {
		if !models.IsRelayTerminalTaskStatus(status) {
			t.Fatalf("status %d must be terminal", status)
		}
	}
	for _, status := range []models.TaskStatus{
		models.InferenceTaskPending,
		models.InferenceTaskCreated,
		models.InferenceTaskStarted,
		models.InferenceTaskParamsUploaded,
		models.InferenceTaskScoreReady,
		models.InferenceTaskErrorReported,
		models.InferenceTaskValidated,
	} {
		if models.IsRelayTerminalTaskStatus(status) {
			t.Fatalf("status %d must not be terminal", status)
		}
	}
}

func TestWaitTaskGroupReturnsWhenTaskEndsWithoutVRFData(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	if err := db.AutoMigrate(&models.InferenceTask{}); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	task := models.InferenceTask{
		TaskID:           "0xtask",
		TaskIDCommitment: "0xcommitment",
		AbortReason:      models.TaskAbortTimeout,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	if err := db.Model(&task).Update("status", models.InferenceTaskEndAborted).Error; err != nil {
		t.Fatalf("failed to update task status: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	taskGroup, err := models.WaitTaskGroup(ctx, db, &task)
	if err != nil {
		t.Fatalf("expected single-task group, got error: %v", err)
	}
	if len(taskGroup) != 1 {
		t.Fatalf("task group size = %d, want 1", len(taskGroup))
	}
	if taskGroup[0].Status != models.InferenceTaskEndAborted {
		t.Fatalf("task group member status = %d, want %d", taskGroup[0].Status, models.InferenceTaskEndAborted)
	}
}

func TestWaitResultTaskHandlesMultipleDownloadedResults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	if err := db.AutoMigrate(&models.InferenceTask{}); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	tasks := make([]models.InferenceTask, 20)
	for i := range tasks {
		task := models.InferenceTask{
			TaskID:           "0xtask",
			TaskIDCommitment: "0xcommitment",
			Status:           models.InferenceTaskResultDownloaded,
		}
		if err := db.Create(&task).Error; err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
		if err := db.Model(&task).Update("status", models.InferenceTaskResultDownloaded).Error; err != nil {
			t.Fatalf("failed to update task status: %v", err)
		}
		tasks[i] = task
		tasks[i].Status = models.InferenceTaskResultDownloaded
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	result, err := models.WaitResultTask(ctx, db, tasks)
	if err != nil {
		t.Fatalf("expected downloaded task, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected downloaded task, got nil")
	}

	time.Sleep(100 * time.Millisecond)
}
