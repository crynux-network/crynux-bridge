package migrations

import (
	"crynux_bridge/models"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const legacyInferenceTaskNeedCancelStatus = 12

func M20260806(db *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		{
			ID: "M20260806",
			Migrate: func(tx *gorm.DB) error {
				if err := tx.Model(&models.InferenceTask{}).
					Where("status = ?", legacyInferenceTaskNeedCancelStatus).
					Where("task_id_commitment <> ?", "").
					Update("status", models.InferenceTaskPending).Error; err != nil {
					return err
				}
				return tx.Model(&models.InferenceTask{}).
					Where("status = ?", legacyInferenceTaskNeedCancelStatus).
					Where("task_id_commitment = ? OR task_id_commitment IS NULL", "").
					Updates(map[string]any{
						"status":       models.InferenceTaskEndAborted,
						"abort_reason": models.TaskAbortTimeout,
					}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return nil
			},
		},
	})
}
