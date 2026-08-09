package tasks

import (
	"context"
	"crynux_bridge/models"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupClientTaskStatusTestDB(t *testing.T) *gorm.DB {
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

func TestUpdateClientTaskStatusWaitsForOtherRepeatAttempt(t *testing.T) {
	db := setupClientTaskStatusTestDB(t)
	ctx := context.Background()

	client := models.Client{ClientId: "repeat-client"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	clientTask := models.ClientTask{ClientID: client.ID, Status: models.ClientTaskStatusRunning}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatal(err)
	}

	failed := models.InferenceTask{
		ClientID:     client.ID,
		ClientTaskID: clientTask.ID,
		TaskID:       "0xgroup1",
		Status:       models.InferenceTaskEndAborted,
	}
	running := models.InferenceTask{
		ClientID:     client.ID,
		ClientTaskID: clientTask.ID,
		TaskID:       "0xgroup2",
		Status:       models.InferenceTaskStarted,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&failed).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&running).Error; err != nil {
		t.Fatal(err)
	}

	if err := updateClientTaskStatus(ctx, db, &failed); err != nil {
		t.Fatal(err)
	}

	updated, err := models.GetClientTaskByID(ctx, db, clientTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != models.ClientTaskStatusRunning {
		t.Fatalf("status = %s, want running while another repeat attempt is unfinished", updated.Status)
	}
}

func TestUpdateClientTaskStatusFailsWhenAllAttemptsFinishedUnsuccessfully(t *testing.T) {
	db := setupClientTaskStatusTestDB(t)
	ctx := context.Background()

	client := models.Client{ClientId: "all-failed"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	clientTask := models.ClientTask{ClientID: client.ID, Status: models.ClientTaskStatusRunning}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatal(err)
	}

	a := models.InferenceTask{
		ClientID: client.ID, ClientTaskID: clientTask.ID, TaskID: "0xa",
		Status: models.InferenceTaskEndAborted,
	}
	b := models.InferenceTask{
		ClientID: client.ID, ClientTaskID: clientTask.ID, TaskID: "0xb",
		Status: models.InferenceTaskEndAborted,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&b).Error; err != nil {
		t.Fatal(err)
	}

	if err := updateClientTaskStatus(ctx, db, &a); err != nil {
		t.Fatal(err)
	}

	updated, err := models.GetClientTaskByID(ctx, db, clientTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != models.ClientTaskStatusFailed {
		t.Fatalf("status = %s, want failed", updated.Status)
	}
	if updated.FailedCount != 1 {
		t.Fatalf("failed_count = %d, want 1", updated.FailedCount)
	}
}

func TestUpdateClientTaskStatusSuccess(t *testing.T) {
	db := setupClientTaskStatusTestDB(t)
	ctx := context.Background()

	client := models.Client{ClientId: "success"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	clientTask := models.ClientTask{ClientID: client.ID, Status: models.ClientTaskStatusRunning}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatal(err)
	}
	task := models.InferenceTask{
		ClientID: client.ID, ClientTaskID: clientTask.ID, TaskID: "0xok",
		Status: models.InferenceTaskResultDownloaded,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	if err := updateClientTaskStatus(ctx, db, &task); err != nil {
		t.Fatal(err)
	}

	updated, err := models.GetClientTaskByID(ctx, db, clientTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != models.ClientTaskStatusSuccess {
		t.Fatalf("status = %s, want success", updated.Status)
	}
}
