package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type inferenceTaskModelFiles20261013 struct {
	RequiredModelFiles          string `gorm:"column:required_model_files;type:longtext"`
	SelectedExecutionModelFiles string `gorm:"column:selected_execution_model_files;type:longtext"`
}

func (inferenceTaskModelFiles20261013) TableName() string {
	return "inference_tasks"
}

func M20261013(db *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{{
		ID: "M20261013",
		Migrate: func(tx *gorm.DB) error {
			table := &inferenceTaskModelFiles20261013{}
			for _, column := range []string{"RequiredModelFiles", "SelectedExecutionModelFiles"} {
				if err := tx.Migrator().AddColumn(table, column); err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			table := &inferenceTaskModelFiles20261013{}
			for _, column := range []string{"SelectedExecutionModelFiles", "RequiredModelFiles"} {
				if err := tx.Migrator().DropColumn(table, column); err != nil {
					return err
				}
			}
			return nil
		},
	}})
}
