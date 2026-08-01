package tasks

import (
	"context"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const cleanupInferenceResultsBatchSize = 500

func CleanupInferenceResults(ctx context.Context) {
	for {
		appConfig := config.GetConfig()
		retentionDays := appConfig.DataDir.InferenceTasksRetentionDays
		if retentionDays > 0 {
			if err := cleanupExpiredInferenceResults(ctx, config.GetDB(), appConfig.DataDir.InferenceTasks, retentionDays); err != nil {
				log.Errorf("CleanupInferenceResults: cleanup failed: %v", err)
			}
		}
		time.Sleep(time.Hour)
	}
}

func cleanupExpiredInferenceResults(ctx context.Context, db *gorm.DB, inferenceTasksDir string, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	deletedTasks, err := cleanupExpiredInferenceTasks(ctx, db, inferenceTasksDir, cutoff)
	if err != nil {
		return err
	}

	deletedClientTasks, err := cleanupExpiredClientTasks(ctx, db, cutoff)
	if err != nil {
		return err
	}

	deletedOrphans, err := cleanupOrphanInferenceResultEntries(ctx, db, inferenceTasksDir, cutoff)
	if err != nil {
		return err
	}

	log.Infof(
		"CleanupInferenceResults: deleted %d inference_tasks, %d client_tasks, %d orphan disk entries older than %s",
		deletedTasks,
		deletedClientTasks,
		deletedOrphans,
		cutoff.Format(time.RFC3339),
	)
	return nil
}

func cleanupExpiredInferenceTasks(ctx context.Context, db *gorm.DB, inferenceTasksDir string, cutoff time.Time) (int, error) {
	deleted := 0
	for {
		var tasks []models.InferenceTask
		dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := db.WithContext(dbCtx).
			Unscoped().
			Where("updated_at < ?", cutoff).
			Order("id ASC").
			Limit(cleanupInferenceResultsBatchSize).
			Find(&tasks).Error
		cancel()
		if err != nil {
			return deleted, err
		}
		if len(tasks) == 0 {
			return deleted, nil
		}

		ids := make([]uint, 0, len(tasks))
		for _, task := range tasks {
			if task.TaskIDCommitment != "" {
				taskFolder := filepath.Join(inferenceTasksDir, task.TaskIDCommitment)
				if err := os.RemoveAll(taskFolder); err != nil {
					log.Errorf("CleanupInferenceResults: cannot remove result dir %s: %v", taskFolder, err)
				}
			}
			ids = append(ids, task.ID)
		}

		dbCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
		err = db.WithContext(dbCtx).Unscoped().Where("id IN ?", ids).Delete(&models.InferenceTask{}).Error
		cancel()
		if err != nil {
			return deleted, err
		}
		deleted += len(ids)
		if len(tasks) < cleanupInferenceResultsBatchSize {
			return deleted, nil
		}
	}
}

func cleanupExpiredClientTasks(ctx context.Context, db *gorm.DB, cutoff time.Time) (int, error) {
	deleted := 0
	for {
		var clientTasks []models.ClientTask
		dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := db.WithContext(dbCtx).
			Unscoped().
			Where("updated_at < ?", cutoff).
			Where("NOT EXISTS (SELECT 1 FROM inference_tasks WHERE inference_tasks.client_task_id = client_tasks.id)").
			Order("id ASC").
			Limit(cleanupInferenceResultsBatchSize).
			Find(&clientTasks).Error
		cancel()
		if err != nil {
			return deleted, err
		}
		if len(clientTasks) == 0 {
			return deleted, nil
		}

		ids := make([]uint, 0, len(clientTasks))
		for _, clientTask := range clientTasks {
			ids = append(ids, clientTask.ID)
		}

		dbCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
		err = db.WithContext(dbCtx).Unscoped().Where("id IN ?", ids).Delete(&models.ClientTask{}).Error
		cancel()
		if err != nil {
			return deleted, err
		}
		deleted += len(ids)
		if len(clientTasks) < cleanupInferenceResultsBatchSize {
			return deleted, nil
		}
	}
}

func cleanupOrphanInferenceResultEntries(ctx context.Context, db *gorm.DB, inferenceTasksDir string, cutoff time.Time) (int, error) {
	entries, err := os.ReadDir(inferenceTasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	deleted := 0
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			log.Errorf("CleanupInferenceResults: cannot stat %s: %v", entry.Name(), err)
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}

		fullPath := filepath.Join(inferenceTasksDir, entry.Name())
		if entry.IsDir() {
			var count int64
			dbCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := db.WithContext(dbCtx).
				Unscoped().
				Model(&models.InferenceTask{}).
				Where("task_id_commitment = ?", entry.Name()).
				Count(&count).Error
			cancel()
			if err != nil {
				log.Errorf("CleanupInferenceResults: cannot check commitment %s: %v", entry.Name(), err)
				continue
			}
			if count > 0 {
				continue
			}
		}

		if err := os.RemoveAll(fullPath); err != nil {
			log.Errorf("CleanupInferenceResults: cannot remove orphan %s: %v", fullPath, err)
			continue
		}
		deleted++
	}
	return deleted, nil
}
