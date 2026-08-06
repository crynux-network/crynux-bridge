package migrations

import (
	"crynux_bridge/models"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestM20260806MigratesLegacyStatus12(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.InferenceTask{}); err != nil {
		t.Fatal(err)
	}

	withCommitment := models.InferenceTask{TaskIDCommitment: "0xcommitment"}
	withoutCommitment := models.InferenceTask{}
	if err := db.Create(&withCommitment).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&withoutCommitment).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{withCommitment.ID, withoutCommitment.ID} {
		if err := db.Model(&models.InferenceTask{}).
			Where("id = ?", id).
			Update("status", legacyInferenceTaskNeedCancelStatus).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := M20260806(db).Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&withCommitment, withCommitment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if withCommitment.Status != models.InferenceTaskPending {
		t.Fatalf("committed task status = %d, want Pending", withCommitment.Status)
	}
	if err := db.First(&withoutCommitment, withoutCommitment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if withoutCommitment.Status != models.InferenceTaskEndAborted {
		t.Fatalf("uncommitted task status = %d, want EndAborted", withoutCommitment.Status)
	}
	if withoutCommitment.AbortReason != models.TaskAbortTimeout {
		t.Fatalf("uncommitted task abort reason = %d, want Timeout", withoutCommitment.AbortReason)
	}
}
