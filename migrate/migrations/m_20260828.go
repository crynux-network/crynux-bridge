package migrations

import (
	"crynux_bridge/models"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M20260828(db *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{{
		ID: "M20260828",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.AutoMigrate(&models.ClientTask{}, &models.InferenceTask{}); err != nil {
				return err
			}
			if err := tx.Model(&models.ClientTask{}).
				Where("submission = ? OR submission IS NULL", "").
				Update("repeat_expanded", true).Error; err != nil {
				return err
			}
			indexes := []struct {
				model any
				name  string
				sql   string
			}{
				{&models.ClientTask{}, "idx_client_task_expansion", "CREATE INDEX idx_client_task_expansion ON client_tasks (repeat_expanded, next_action_at, id)"},
				{&models.ClientTask{}, "idx_client_task_pending_submission", "CREATE INDEX idx_client_task_pending_submission ON client_tasks (client_id, repeat_expanded, submission_task_type, submission_model_id)"},
				{&models.InferenceTask{}, "idx_inference_task_due", "CREATE INDEX idx_inference_task_due ON inference_tasks (operation_status, next_action_at, id)"},
				{&models.InferenceTask{}, "idx_inference_task_operation", "CREATE INDEX idx_inference_task_operation ON inference_tasks (operation_status, id)"},
				{&models.InferenceTask{}, "idx_inference_task_group", "CREATE INDEX idx_inference_task_group ON inference_tasks (task_id, sequence, id)"},
				{&models.InferenceTask{}, "idx_inference_task_client_status", "CREATE INDEX idx_inference_task_client_status ON inference_tasks (client_task_id, status, id)"},
			}
			for _, index := range indexes {
				if tx.Migrator().HasIndex(index.model, index.name) {
					if err := tx.Migrator().DropIndex(index.model, index.name); err != nil {
						return err
					}
				}
				if err := tx.Exec(index.sql).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}})
}
