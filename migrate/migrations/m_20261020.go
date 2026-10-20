package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type clientTaskQuantizationKey20261020 struct {
	SubmissionQuantizationKey string `gorm:"column:submission_quantization_key;type:varchar(64);not null;default:''"`
}

func (clientTaskQuantizationKey20261020) TableName() string {
	return "client_tasks"
}

type inferenceTaskQuantizationKey20261020 struct {
	QuantizationKey string `gorm:"column:quantization_key;type:varchar(64);not null;default:''"`
}

func (inferenceTaskQuantizationKey20261020) TableName() string {
	return "inference_tasks"
}

func M20261020(db *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{{
		ID: "M20261020",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Migrator().AddColumn(&clientTaskQuantizationKey20261020{}, "SubmissionQuantizationKey"); err != nil {
				return err
			}
			if err := tx.Migrator().AddColumn(&inferenceTaskQuantizationKey20261020{}, "QuantizationKey"); err != nil {
				return err
			}
			if err := tx.Migrator().DropIndex(&clientTaskQuantizationKey20261020{}, "idx_client_task_pending_submission"); err != nil {
				return err
			}
			indexes := []string{
				"CREATE INDEX idx_client_task_pending_submission ON client_tasks (client_id, repeat_expanded, submission_task_type, submission_model_id, submission_quantization_key)",
				"CREATE INDEX idx_inference_task_pending_quant ON inference_tasks (client_id, task_type, quantization_key)",
			}
			for _, sql := range indexes {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Migrator().DropIndex(&inferenceTaskQuantizationKey20261020{}, "idx_inference_task_pending_quant"); err != nil {
				return err
			}
			if err := tx.Migrator().DropIndex(&clientTaskQuantizationKey20261020{}, "idx_client_task_pending_submission"); err != nil {
				return err
			}
			if err := tx.Migrator().DropColumn(&inferenceTaskQuantizationKey20261020{}, "QuantizationKey"); err != nil {
				return err
			}
			if err := tx.Migrator().DropColumn(&clientTaskQuantizationKey20261020{}, "SubmissionQuantizationKey"); err != nil {
				return err
			}
			return tx.Exec("CREATE INDEX idx_client_task_pending_submission ON client_tasks (client_id, repeat_expanded, submission_task_type, submission_model_id)").Error
		},
	}})
}
