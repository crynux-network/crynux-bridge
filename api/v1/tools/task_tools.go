package tools

import (
	"context"

	"crynux_bridge/models"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// get Client from local db
func GetClient(ctx context.Context, db *gorm.DB, clientID string) (*models.Client, error) {
	client := models.Client{ClientId: clientID}

	err := func() error {
		dbCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		return db.WithContext(dbCtx).Where(&client).First(&client).Error
	}()

	return &client, err
}

// create Client
func CreateClient(ctx context.Context, db *gorm.DB, clientID string) (*models.Client, error) {
	client := models.Client{ClientId: clientID}
	err := func() error {
		dbCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		return db.WithContext(dbCtx).Create(&client).Error
	}()

	return &client, err
}

func CreateClientIfNotExist(ctx context.Context, db *gorm.DB, clientID string) (*models.Client, error) {
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	client := models.Client{ClientId: clientID}
	if err := db.WithContext(dbCtx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}},
		DoNothing: true,
	}).Create(&client).Error; err != nil {
		return nil, err
	}
	return GetClient(ctx, db, clientID)
}

// create ClientTask for the given Client
func CreateClientTask(ctx context.Context, db *gorm.DB, client *models.Client) (*models.ClientTask, error) {
	clientTask := models.ClientTask{
		Client:         *client,
		RepeatExpanded: true,
	}
	err := func() error {
		dbCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		return db.WithContext(dbCtx).Create(&clientTask).Error
	}()

	return &clientTask, err
}

// get ClientTask from local db, if not exist, return a new one
func GetClientTask(ctx context.Context, db *gorm.DB, clientID uint, clientTaskID uint) (*models.ClientTask, error) {
	clientTask := models.ClientTask{
		RootModel: models.RootModel{ID: clientTaskID},
		ClientID:  clientID,
	}

	err := func() error {
		dbCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		return db.WithContext(dbCtx).Model(&clientTask).Where(&clientTask).Preload("InferenceTasks").First(&clientTask).Error
	}()

	return &clientTask, err
}
