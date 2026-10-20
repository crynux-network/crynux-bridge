package migrations

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type legacyClientTask20261020 struct {
	ID                 uint   `gorm:"primaryKey"`
	ClientID           uint
	RepeatExpanded     bool
	SubmissionTaskType uint8
	SubmissionModelID  string `gorm:"type:varchar(255)"`
}

func (legacyClientTask20261020) TableName() string {
	return "client_tasks"
}

type legacyInferenceTask20261020 struct {
	ID           uint   `gorm:"primaryKey"`
	ClientID     uint
	TaskType     uint8
	TaskModelIDs string
}

func (legacyInferenceTask20261020) TableName() string {
	return "inference_tasks"
}

func TestM20261020AddsQuantizationKeys(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacyClientTask20261020{}, &legacyInferenceTask20261020{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE INDEX idx_client_task_pending_submission ON client_tasks (client_id, repeat_expanded, submission_task_type, submission_model_id)").Error; err != nil {
		t.Fatal(err)
	}
	migration := M20261020(db)
	if err := migration.Migrate(); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasColumn(&clientTaskQuantizationKey20261020{}, "SubmissionQuantizationKey") {
		t.Fatal("missing client_tasks.submission_quantization_key")
	}
	if !db.Migrator().HasColumn(&inferenceTaskQuantizationKey20261020{}, "QuantizationKey") {
		t.Fatal("missing inference_tasks.quantization_key")
	}
	if !db.Migrator().HasIndex(&clientTaskQuantizationKey20261020{}, "idx_client_task_pending_submission") {
		t.Fatal("missing idx_client_task_pending_submission")
	}
	if !db.Migrator().HasIndex(&inferenceTaskQuantizationKey20261020{}, "idx_inference_task_pending_quant") {
		t.Fatal("missing idx_inference_task_pending_quant")
	}
	if err := migration.RollbackLast(); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn(&legacyClientTask20261020{}, "SubmissionQuantizationKey") {
		t.Fatal("submission_quantization_key remains after rollback")
	}
	if db.Migrator().HasColumn(&legacyInferenceTask20261020{}, "QuantizationKey") {
		t.Fatal("quantization_key remains after rollback")
	}
	if !db.Migrator().HasIndex(&legacyClientTask20261020{}, "idx_client_task_pending_submission") {
		t.Fatal("idx_client_task_pending_submission missing after rollback")
	}
	if db.Migrator().HasIndex(&legacyInferenceTask20261020{}, "idx_inference_task_pending_quant") {
		t.Fatal("idx_inference_task_pending_quant remains after rollback")
	}
}
