package models_test

import (
	"context"
	"crynux_bridge/models"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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
