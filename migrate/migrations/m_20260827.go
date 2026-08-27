package migrations

import (
	"errors"
	"fmt"
	"math/big"

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

type clientID20260827 struct {
	ClientID string `gorm:"column:client_id;uniqueIndex:idx_clients_client_id"`
}

func (clientID20260827) TableName() string {
	return "clients"
}

func convertTaskFeesToWei(tx *gorm.DB) error {
	rows, err := tx.Model(&inferenceTaskFee20260827{}).Select("id", "task_fee").Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	type feeUpdate struct {
		id  uint
		wei string
	}
	updates := make([]feeUpdate, 0)
	gweiToWei := big.NewInt(1_000_000_000)
	for rows.Next() {
		var id uint
		var feeGWei string
		if err := rows.Scan(&id, &feeGWei); err != nil {
			return err
		}
		fee, ok := new(big.Int).SetString(feeGWei, 10)
		if !ok || fee.Sign() < 0 {
			return fmt.Errorf("invalid legacy task fee %q for inference task %d", feeGWei, id)
		}
		updates = append(updates, feeUpdate{
			id:  id,
			wei: new(big.Int).Mul(fee, gweiToWei).String(),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, update := range updates {
		if err := tx.Model(&inferenceTaskFee20260827{}).
			Where("id = ?", update.id).
			Update("task_fee", update.wei).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureUniqueClientIDIndex(tx *gorm.DB) error {
	var duplicate struct {
		ClientID string
		Count    int64
	}
	err := tx.Model(&clientID20260827{}).
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

	if tx.Migrator().HasIndex(&clientID20260827{}, "idx_clients_client_id") {
		if err := tx.Migrator().DropIndex(&clientID20260827{}, "idx_clients_client_id"); err != nil {
			return err
		}
	}
	return tx.Migrator().CreateIndex(&clientID20260827{}, "idx_clients_client_id")
}

func M20260827(db *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{{
		ID: "M20260827",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Migrator().AlterColumn(&inferenceTaskFee20260827{}, "TaskFee"); err != nil {
				return err
			}
			if err := convertTaskFeesToWei(tx); err != nil {
				return err
			}
			return ensureUniqueClientIDIndex(tx)
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}})
}
