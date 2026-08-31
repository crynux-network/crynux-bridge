package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M20250422(db *gorm.DB) *gormigrate.Gormigrate {
	type Client struct {
		ClientId string `json:"client_id" gorm:"type:varchar(255);index"`
	}

	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		{
			ID: "M20250422",
			Migrate: func(tx *gorm.DB) error {
				if err := tx.Migrator().AlterColumn(&Client{}, "ClientId"); err != nil {
					return err
				}
				if !tx.Migrator().HasIndex(&Client{}, "ClientId") {
					if err := tx.Migrator().CreateIndex(&Client{}, "ClientId"); err != nil {
						return err
					}
				}

				return nil
			},
			Rollback: func(tx *gorm.DB) error {
				if tx.Migrator().HasIndex(&Client{}, "ClientId") {
					if err := tx.Migrator().DropIndex(&Client{}, "ClientId"); err != nil {
						return err
					}
				}
				if err := tx.Migrator().AlterColumn(&Client{}, "ClientId"); err != nil {
					return err
				}
				return nil
			},
		},
	})
}
