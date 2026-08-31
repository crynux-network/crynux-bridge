package migrations

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type legacyClientTask20260828 struct {
	ID        uint `gorm:"primaryKey"`
	ClientID  uint
	Status    string
	FailedCount int
}

func (legacyClientTask20260828) TableName() string {
	return "client_tasks"
}

type legacyInferenceTask20260828 struct {
	ID               uint   `gorm:"primaryKey"`
	ClientTaskID     uint
	Status           uint8
	TaskID           string
	Sequence         uint64
}

func (legacyInferenceTask20260828) TableName() string {
	return "inference_tasks"
}

func TestM20260828AddsTaskEngineState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacyClientTask20260828{}, &legacyInferenceTask20260828{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&legacyClientTask20260828{ClientID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := M20260828(db).Migrate(); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasColumn(&clientTaskEngineFields20260828{}, "RepeatExpanded") {
		t.Fatal("missing client_tasks.repeat_expanded")
	}
	if !db.Migrator().HasColumn(&inferenceTaskEngineFields20260828{}, "OperationStatus") {
		t.Fatal("missing inference_tasks.operation_status")
	}
	for _, index := range []struct {
		model any
		name  string
	}{
		{&clientTaskEngineFields20260828{}, "idx_client_task_expansion"},
		{&clientTaskEngineFields20260828{}, "idx_client_task_pending_submission"},
		{&inferenceTaskEngineFields20260828{}, "idx_inference_task_due"},
		{&inferenceTaskEngineFields20260828{}, "idx_inference_task_operation"},
		{&inferenceTaskEngineFields20260828{}, "idx_inference_task_group"},
		{&inferenceTaskEngineFields20260828{}, "idx_inference_task_client_status"},
	} {
		if !db.Migrator().HasIndex(index.model, index.name) {
			t.Fatalf("missing index %s", index.name)
		}
	}
}
