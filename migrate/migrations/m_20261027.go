package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type clientTaskCreatorCancelledForM20261027 struct {
	CreatorCancelled bool `gorm:"column:creator_cancelled;not null;default:0"`
}

func (clientTaskCreatorCancelledForM20261027) TableName() string { return "client_tasks" }

func M20261027(db *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{{
		ID: "M20261027",
		Migrate: func(tx *gorm.DB) error {
			return tx.Migrator().AddColumn(&clientTaskCreatorCancelledForM20261027{}, "CreatorCancelled")
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropColumn(&clientTaskCreatorCancelledForM20261027{}, "CreatorCancelled")
		},
	}})
}
