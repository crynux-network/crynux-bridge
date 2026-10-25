package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type inferenceTaskQueueTimeoutForM20261025 struct {
	QueueTimeout uint64 `gorm:"column:queue_timeout;not null;default:0"`
}

func (inferenceTaskQueueTimeoutForM20261025) TableName() string { return "inference_tasks" }

func M20261025(db *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{{
		ID: "M20261025",
		Migrate: func(tx *gorm.DB) error {
			return tx.Migrator().AddColumn(&inferenceTaskQueueTimeoutForM20261025{}, "QueueTimeout")
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropColumn(&inferenceTaskQueueTimeoutForM20261025{}, "QueueTimeout")
		},
	}})
}
