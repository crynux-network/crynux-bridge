package migrations

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type legacyInferenceTask20260827 struct {
	ID      uint   `gorm:"primaryKey"`
	TaskFee uint64 `gorm:"column:task_fee"`
}

func (legacyInferenceTask20260827) TableName() string {
	return "inference_tasks"
}

type legacyClient20260827 struct {
	ID       uint   `gorm:"primaryKey"`
	ClientID string `gorm:"column:client_id"`
}

func (legacyClient20260827) TableName() string {
	return "clients"
}

func setupLegacy20260827DB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacyInferenceTask20260827{}, &legacyClient20260827{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE INDEX idx_clients_client_id ON clients(client_id)").Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestM20260827ConvertsLegacyFeesAndReplacesClientIndex(t *testing.T) {
	db := setupLegacy20260827DB(t)
	tasks := []legacyInferenceTask20260827{
		{TaskFee: 0},
		{TaskFee: 1},
		{TaskFee: 9_223_372_036_854_775_807},
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]legacyClient20260827{{ClientID: "one"}, {ClientID: "two"}}).Error; err != nil {
		t.Fatal(err)
	}

	if err := M20260827(db).Migrate(); err != nil {
		t.Fatal(err)
	}
	columnTypes, err := db.Migrator().ColumnTypes(&inferenceTaskFee20260827{})
	if err != nil {
		t.Fatal(err)
	}
	foundText := false
	for _, column := range columnTypes {
		if column.Name() == "task_fee" {
			foundText = strings.Contains(strings.ToLower(column.DatabaseTypeName()), "varchar") ||
				strings.Contains(strings.ToLower(column.DatabaseTypeName()), "text")
		}
	}
	if !foundText {
		t.Fatal("task_fee is not a text column")
	}

	var loaded []inferenceTaskFee20260827
	if err := db.Order("id").Find(&loaded).Error; err != nil {
		t.Fatal(err)
	}
	wantFees := []string{
		"0",
		"1000000000",
		"9223372036854775807000000000",
	}
	for i, want := range wantFees {
		if loaded[i].TaskFee != want {
			t.Fatalf("task %d fee = %q, want %q", loaded[i].ID, loaded[i].TaskFee, want)
		}
	}

	if err := db.Create(&legacyClient20260827{ClientID: "unique"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&legacyClient20260827{ClientID: "unique"}).Error; err == nil {
		t.Fatal("duplicate client_id was accepted")
	}
}

func TestM20260827RejectsDuplicateClientIDs(t *testing.T) {
	db := setupLegacy20260827DB(t)
	if err := db.Create(&[]legacyClient20260827{
		{ClientID: "duplicate"},
		{ClientID: "duplicate"},
	}).Error; err != nil {
		t.Fatal(err)
	}

	err := M20260827(db).Migrate()
	if err == nil {
		t.Fatal("migration accepted duplicate client_id values")
	}
	if !strings.Contains(err.Error(), `duplicate client_id "duplicate"`) {
		t.Fatalf("migration error = %v", err)
	}

	if !db.Migrator().HasIndex(&legacyClient20260827{}, "idx_clients_client_id") {
		t.Fatal("legacy client_id index was removed after migration failure")
	}
	var clients []legacyClient20260827
	if queryErr := db.Find(&clients).Error; queryErr != nil {
		t.Fatal(queryErr)
	}
	if len(clients) != 2 {
		t.Fatalf("client count = %d, want 2", len(clients))
	}
}
