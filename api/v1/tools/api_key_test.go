package tools

import (
	"context"
	"sync"
	"testing"

	"crynux_bridge/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestReadAuthorizationIgnoresExhaustedCreateQuota(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ClientAPIKey{}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	key, _, err := GenerateAPIKey(ctx, db, "client")
	if err != nil {
		t.Fatal(err)
	}
	apiKey, err := models.GetAPIKeyByClientID(ctx, db, "client")
	if err != nil {
		t.Fatal(err)
	}
	if err := AddAPIKeyRole(ctx, db, apiKey, models.RoleChat); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(apiKey).Updates(map[string]interface{}{
		"use_limit":  1,
		"used_count": 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	authorization := "Bearer " + key
	if _, err := ValidateAuthorization(ctx, db, authorization); err == nil {
		t.Fatal("create authorization accepted exhausted quota")
	}
	if _, err := ValidateReadAuthorization(ctx, db, authorization); err != nil {
		t.Fatalf("read authorization rejected exhausted quota: %v", err)
	}
}

func TestValidateAPIKeyRejectsShortEncodedKey(t *testing.T) {
	if _, err := ValidateAPIKey(context.Background(), nil, "YQ=="); err == nil {
		t.Fatal("short API key was accepted")
	}
}

func TestGetClientTaskRejectsAnotherClient(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Client{}, &models.ClientTask{}, &models.InferenceTask{}); err != nil {
		t.Fatal(err)
	}

	client := models.Client{ClientId: "owner"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	clientTask := models.ClientTask{ClientID: client.ID}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := GetClientTask(context.Background(), db, client.ID+1, clientTask.ID); err == nil {
		t.Fatal("cross-client task lookup succeeded")
	}
}

func TestCreateClientIfNotExistConcurrent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:client-upsert?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.Client{}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := CreateClientIfNotExist(context.Background(), db, "same-client")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Model(&models.Client{}).Where("client_id = ?", "same-client").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("client count = %d, want 1", count)
	}
}
