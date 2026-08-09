package tasks

import (
	"context"
	"errors"
	"testing"

	"crynux_bridge/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSiblingCancelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Client{}, &models.ClientTask{}, &models.InferenceTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCancelQueuedValidationSiblingsAbortsOtherMembers(t *testing.T) {
	db := setupSiblingCancelTestDB(t)
	ctx := context.Background()

	client := models.Client{ClientId: "sibling-cancel"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	clientTask := models.ClientTask{ClientID: client.ID}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatal(err)
	}

	taskID := "0xgroup"
	tasks := []models.InferenceTask{
		{
			ClientID:         client.ID,
			ClientTaskID:     clientTask.ID,
			TaskID:           taskID,
			TaskIDCommitment: "0xa",
			Status:           models.InferenceTaskEndAborted,
			AbortReason:      models.TaskAbortCreatorValidationTimeout,
		},
		{
			ClientID:         client.ID,
			ClientTaskID:     clientTask.ID,
			TaskID:           taskID,
			TaskIDCommitment: "0xb",
			Status:           models.InferenceTaskCreated,
		},
		{
			ClientID:         client.ID,
			ClientTaskID:     clientTask.ID,
			TaskID:           taskID,
			TaskIDCommitment: "0xc",
			Status:           models.InferenceTaskCreated,
		},
	}
	for i := range tasks {
		if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&tasks[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	var aborted []string
	cancelQueuedValidationSiblingsWith(ctx, db, &tasks[0], func(_ context.Context, commitment string) error {
		aborted = append(aborted, commitment)
		return nil
	})

	if len(aborted) != 2 {
		t.Fatalf("aborted count = %d, want 2: %v", len(aborted), aborted)
	}
	got := map[string]bool{}
	for _, c := range aborted {
		got[c] = true
	}
	if !got["0xb"] || !got["0xc"] {
		t.Fatalf("aborted commitments = %v, want 0xb and 0xc", aborted)
	}
}

func TestCancelQueuedValidationSiblingsSkipsEndInvalidated(t *testing.T) {
	db := setupSiblingCancelTestDB(t)
	ctx := context.Background()

	task := &models.InferenceTask{
		TaskID:           "0xgroup",
		TaskIDCommitment: "0xa",
		Status:           models.InferenceTaskEndInvalidated,
	}
	called := false
	cancelQueuedValidationSiblingsWith(ctx, db, task, func(context.Context, string) error {
		called = true
		return nil
	})
	if called {
		t.Fatal("EndInvalidated must not abort siblings")
	}
}

func TestCancelQueuedValidationSiblingsRequiresGroupOfThree(t *testing.T) {
	db := setupSiblingCancelTestDB(t)
	ctx := context.Background()

	client := models.Client{ClientId: "single"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	clientTask := models.ClientTask{ClientID: client.ID}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatal(err)
	}
	task := models.InferenceTask{
		ClientID:         client.ID,
		ClientTaskID:     clientTask.ID,
		TaskID:           "0xalone",
		TaskIDCommitment: "0xa",
		Status:           models.InferenceTaskEndAborted,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	called := false
	cancelQueuedValidationSiblingsWith(ctx, db, &task, func(context.Context, string) error {
		called = true
		return nil
	})
	if called {
		t.Fatal("single-task group must not abort siblings")
	}
}

func TestCancelQueuedValidationSiblingsIgnoresAbortErrors(t *testing.T) {
	db := setupSiblingCancelTestDB(t)
	ctx := context.Background()

	client := models.Client{ClientId: "err"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	clientTask := models.ClientTask{ClientID: client.ID}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatal(err)
	}
	taskID := "0xgroup"
	tasks := []models.InferenceTask{
		{
			ClientID: client.ID, ClientTaskID: clientTask.ID, TaskID: taskID,
			TaskIDCommitment: "0xa", Status: models.InferenceTaskEndAborted,
		},
		{
			ClientID: client.ID, ClientTaskID: clientTask.ID, TaskID: taskID,
			TaskIDCommitment: "0xb", Status: models.InferenceTaskCreated,
		},
		{
			ClientID: client.ID, ClientTaskID: clientTask.ID, TaskID: taskID,
			TaskIDCommitment: "0xc", Status: models.InferenceTaskCreated,
		},
	}
	for i := range tasks {
		if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&tasks[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	var aborted []string
	cancelQueuedValidationSiblingsWith(ctx, db, &tasks[0], func(_ context.Context, commitment string) error {
		aborted = append(aborted, commitment)
		return errors.New("Task can no longer be cancelled")
	})
	if len(aborted) != 2 {
		t.Fatalf("must still attempt both siblings despite errors, got %v", aborted)
	}
}
