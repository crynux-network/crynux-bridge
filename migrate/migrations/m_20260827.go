package migrations

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type inferenceTaskFee20260827 struct {
	ID      uint   `gorm:"column:id;primaryKey"`
	TaskFee string `gorm:"column:task_fee;type:varchar(78)"`
}

func (inferenceTaskFee20260827) TableName() string {
	return "inference_tasks"
}

type clientIDColumn20260827 struct {
	ClientID string `gorm:"column:client_id;type:varchar(255)"`
}

func (clientIDColumn20260827) TableName() string {
	return "clients"
}

type clientIDIndex20260827 struct {
	ClientID string `gorm:"column:client_id;type:varchar(255);uniqueIndex:idx_clients_client_id"`
}

func (clientIDIndex20260827) TableName() string {
	return "clients"
}

func isMissingIndexError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such index") ||
		strings.Contains(msg, "check that column/key exists") ||
		strings.Contains(msg, "can't drop")
}

func ensureNoDuplicateClientIDs(tx *gorm.DB) error {
	var duplicate struct {
		ClientID string
		Count    int64
	}
	err := tx.Model(&clientIDColumn20260827{}).
		Select("client_id, COUNT(*) AS count").
		Group("client_id").
		Having("COUNT(*) > 1").
		First(&duplicate).Error
	if err == nil {
		return fmt.Errorf("cannot create unique client_id index: duplicate client_id %q has %d rows", duplicate.ClientID, duplicate.Count)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

func replaceUniqueClientIDIndex(tx *gorm.DB) error {
	if err := tx.Migrator().DropIndex(&clientIDIndex20260827{}, "idx_clients_client_id"); err != nil && !isMissingIndexError(err) {
		return err
	}
	return tx.Migrator().CreateIndex(&clientIDIndex20260827{}, "idx_clients_client_id")
}

func M20260827(db *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{{
		ID: "M20260827",
		Migrate: func(tx *gorm.DB) error {
			if err := ensureNoDuplicateClientIDs(tx); err != nil {
				return err
			}
			if err := tx.Migrator().AlterColumn(&inferenceTaskFee20260827{}, "TaskFee"); err != nil {
				return err
			}
			if err := tx.Migrator().AlterColumn(&clientIDColumn20260827{}, "ClientID"); err != nil {
				return err
			}
			return replaceUniqueClientIDIndex(tx)
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}})
}
