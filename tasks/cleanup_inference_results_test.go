package tasks

import (
	"context"
	"crynux_bridge/models"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCleanupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	if err := db.AutoMigrate(&models.Client{}, &models.ClientTask{}, &models.InferenceTask{}); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}
	return db
}

func forceUpdatedAt(t *testing.T, db *gorm.DB, model any, id uint, updatedAt time.Time) {
	t.Helper()
	if err := db.Model(model).Where("id = ?", id).UpdateColumn("updated_at", updatedAt).Error; err != nil {
		t.Fatalf("failed to force updated_at: %v", err)
	}
}

func TestCleanupExpiredInferenceResultsDeletesOldRowsAndDirs(t *testing.T) {
	db := setupCleanupTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	client := models.Client{ClientId: "cleanup-client"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	oldClientTask := models.ClientTask{ClientID: client.ID, Status: models.ClientTaskStatusSuccess}
	recentClientTask := models.ClientTask{ClientID: client.ID, Status: models.ClientTaskStatusSuccess}
	if err := db.Create(&oldClientTask).Error; err != nil {
		t.Fatalf("failed to create old client task: %v", err)
	}
	if err := db.Create(&recentClientTask).Error; err != nil {
		t.Fatalf("failed to create recent client task: %v", err)
	}

	oldCommitment := "0xoldcommitment"
	recentCommitment := "0xrecentcommitment"
	oldTask := models.InferenceTask{
		ClientID:         client.ID,
		ClientTaskID:     oldClientTask.ID,
		TaskIDCommitment: oldCommitment,
		Status:           models.InferenceTaskPending,
	}
	recentTask := models.InferenceTask{
		ClientID:         client.ID,
		ClientTaskID:     recentClientTask.ID,
		TaskIDCommitment: recentCommitment,
		Status:           models.InferenceTaskResultDownloaded,
	}
	if err := db.Create(&oldTask).Error; err != nil {
		t.Fatalf("failed to create old inference task: %v", err)
	}
	if err := db.Create(&recentTask).Error; err != nil {
		t.Fatalf("failed to create recent inference task: %v", err)
	}

	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	forceUpdatedAt(t, db, &models.InferenceTask{}, oldTask.ID, oldTime)
	forceUpdatedAt(t, db, &models.ClientTask{}, oldClientTask.ID, oldTime)

	oldDir := filepath.Join(dir, oldCommitment)
	recentDir := filepath.Join(dir, recentCommitment)
	orphanDir := filepath.Join(dir, "0xorphan")
	stagingZip := filepath.Join(dir, "abc_checkpoint.zip")
	for _, path := range []string{oldDir, recentDir, orphanDir} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatalf("failed to create dir %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(path, "0.json"), []byte("{}"), 0600); err != nil {
			t.Fatalf("failed to write file in %s: %v", path, err)
		}
	}
	if err := os.WriteFile(stagingZip, []byte("zip"), 0600); err != nil {
		t.Fatalf("failed to write staging zip: %v", err)
	}

	oldModTime := time.Now().Add(-10 * 24 * time.Hour)
	for _, path := range []string{oldDir, orphanDir, stagingZip} {
		if err := os.Chtimes(path, oldModTime, oldModTime); err != nil {
			t.Fatalf("failed to chtimes %s: %v", path, err)
		}
	}

	if err := cleanupExpiredInferenceResults(ctx, db, dir, 7); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	var oldTaskCount int64
	if err := db.Unscoped().Model(&models.InferenceTask{}).Where("id = ?", oldTask.ID).Count(&oldTaskCount).Error; err != nil {
		t.Fatalf("failed to count old inference task: %v", err)
	}
	if oldTaskCount != 0 {
		t.Fatalf("expected old inference task deleted, count=%d", oldTaskCount)
	}

	var recentTaskCount int64
	if err := db.Unscoped().Model(&models.InferenceTask{}).Where("id = ?", recentTask.ID).Count(&recentTaskCount).Error; err != nil {
		t.Fatalf("failed to count recent inference task: %v", err)
	}
	if recentTaskCount != 1 {
		t.Fatalf("expected recent inference task kept, count=%d", recentTaskCount)
	}

	var oldClientTaskCount int64
	if err := db.Unscoped().Model(&models.ClientTask{}).Where("id = ?", oldClientTask.ID).Count(&oldClientTaskCount).Error; err != nil {
		t.Fatalf("failed to count old client task: %v", err)
	}
	if oldClientTaskCount != 0 {
		t.Fatalf("expected old client task deleted, count=%d", oldClientTaskCount)
	}

	var recentClientTaskCount int64
	if err := db.Unscoped().Model(&models.ClientTask{}).Where("id = ?", recentClientTask.ID).Count(&recentClientTaskCount).Error; err != nil {
		t.Fatalf("failed to count recent client task: %v", err)
	}
	if recentClientTaskCount != 1 {
		t.Fatalf("expected recent client task kept, count=%d", recentClientTaskCount)
	}

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("expected old result dir removed, err=%v", err)
	}
	if _, err := os.Stat(recentDir); err != nil {
		t.Fatalf("expected recent result dir kept: %v", err)
	}
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Fatalf("expected orphan dir removed, err=%v", err)
	}
	if _, err := os.Stat(stagingZip); !os.IsNotExist(err) {
		t.Fatalf("expected staging zip removed, err=%v", err)
	}

	var clientCount int64
	if err := db.Model(&models.Client{}).Where("id = ?", client.ID).Count(&clientCount).Error; err != nil {
		t.Fatalf("failed to count client: %v", err)
	}
	if clientCount != 1 {
		t.Fatalf("expected client kept, count=%d", clientCount)
	}
}

func TestCleanupExpiredInferenceResultsDisabled(t *testing.T) {
	db := setupCleanupTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	client := models.Client{ClientId: "cleanup-disabled"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	clientTask := models.ClientTask{ClientID: client.ID, Status: models.ClientTaskStatusFailed}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatalf("failed to create client task: %v", err)
	}
	task := models.InferenceTask{
		ClientID:         client.ID,
		ClientTaskID:     clientTask.ID,
		TaskIDCommitment: "0xkeep",
		Status:           models.InferenceTaskEndAborted,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("failed to create inference task: %v", err)
	}
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	forceUpdatedAt(t, db, &models.InferenceTask{}, task.ID, oldTime)
	forceUpdatedAt(t, db, &models.ClientTask{}, clientTask.ID, oldTime)

	taskDir := filepath.Join(dir, task.TaskIDCommitment)
	if err := os.MkdirAll(taskDir, 0700); err != nil {
		t.Fatalf("failed to create task dir: %v", err)
	}
	if err := os.Chtimes(taskDir, oldTime, oldTime); err != nil {
		t.Fatalf("failed to chtimes task dir: %v", err)
	}

	if err := cleanupExpiredInferenceResults(ctx, db, dir, 0); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	var taskCount int64
	if err := db.Unscoped().Model(&models.InferenceTask{}).Where("id = ?", task.ID).Count(&taskCount).Error; err != nil {
		t.Fatalf("failed to count inference task: %v", err)
	}
	if taskCount != 1 {
		t.Fatalf("expected inference task kept when retention disabled, count=%d", taskCount)
	}
	if _, err := os.Stat(taskDir); err != nil {
		t.Fatalf("expected result dir kept when retention disabled: %v", err)
	}
}
