package migrations

import (
	"crynux_bridge/models"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestM20260828AddsTaskEngineStateAndMarksExistingClientTasksExpanded(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Client{}, &models.ClientTask{}, &models.InferenceTask{}); err != nil {
		t.Fatal(err)
	}
	legacy := models.ClientTask{ClientID: 1}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := M20260828(db).Migrate(); err != nil {
		t.Fatal(err)
	}
	var updated models.ClientTask
	if err := db.First(&updated, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !updated.RepeatExpanded {
		t.Fatal("existing client task was not marked repeat-expanded")
	}
	for _, index := range []string{
		"idx_client_task_expansion",
		"idx_client_task_pending_submission",
		"idx_inference_task_due",
		"idx_inference_task_operation",
		"idx_inference_task_group",
		"idx_inference_task_client_status",
	} {
		model := any(&models.InferenceTask{})
		if index == "idx_client_task_expansion" || index == "idx_client_task_pending_submission" {
			model = &models.ClientTask{}
		}
		if !db.Migrator().HasIndex(model, index) {
			t.Fatalf("missing index %s", index)
		}
	}
}
