package migrations

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type legacyInferenceTask20261013 struct {
	ID uint `gorm:"primaryKey"`
}

func (legacyInferenceTask20261013) TableName() string {
	return "inference_tasks"
}

func TestM20261013AddsModelFileColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacyInferenceTask20261013{}); err != nil {
		t.Fatal(err)
	}
	migration := M20261013(db)
	if err := migration.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"RequiredModelFiles", "SelectedExecutionModelFiles"} {
		if !db.Migrator().HasColumn(&inferenceTaskModelFiles20261013{}, column) {
			t.Fatalf("missing inference_tasks column %s", column)
		}
	}
	if err := migration.RollbackLast(); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"RequiredModelFiles", "SelectedExecutionModelFiles"} {
		if db.Migrator().HasColumn(&inferenceTaskModelFiles20261013{}, column) {
			t.Fatalf("column %s remains after rollback", column)
		}
	}
}
