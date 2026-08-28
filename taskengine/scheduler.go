package taskengine

import (
	"context"
	"crynux_bridge/models"
	"crynux_bridge/tasktrace"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func (e *Engine) run(ctx context.Context) {
	if err := e.recover(ctx); err != nil {
		log.Errorf("TaskEngine: recovery failed: %v", err)
	}
	ticker := time.NewTicker(e.config.ScanInterval)
	defer ticker.Stop()
	for {
		if err := e.iterate(ctx); err != nil && ctx.Err() == nil {
			log.Errorf("TaskEngine: iteration failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (e *Engine) recover(ctx context.Context) error {
	now := time.Now()
	var runningOperations []models.InferenceTask
	if err := e.db.WithContext(ctx).
		Where("operation_status = ?", models.TaskOperationStatusRunning).
		Find(&runningOperations).Error; err != nil {
		return err
	}
	if err := e.db.WithContext(ctx).Model(&models.InferenceTask{}).
		Where("operation_status = ?", models.TaskOperationStatusRunning).
		Updates(map[string]any{
			"operation_status":  models.TaskOperationStatusNone,
			"operation_type":    models.TaskOperationNone,
			"operation_unknown": true,
			"next_action_at":    now,
		}).Error; err != nil {
		return err
	}
	for i := range runningOperations {
		if runningOperations[i].OperationType != models.TaskOperationValidate {
			continue
		}
		if err := e.db.WithContext(ctx).Model(&models.InferenceTask{}).
			Where("task_id = ?", runningOperations[i].TaskID).
			Updates(map[string]any{
				"relay_checked":  false,
				"next_action_at": now,
			}).Error; err != nil {
			return err
		}
	}
	var clientTasks []models.ClientTask
	if err := e.db.WithContext(ctx).
		Where("status = ?", models.ClientTaskStatusRunning).
		Order("id ASC").
		Find(&clientTasks).Error; err != nil {
		return err
	}
	for i := range clientTasks {
		if err := e.recalculateClientTask(ctx, e.db, clientTasks[i].ID); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) iterate(ctx context.Context) error {
	if err := e.expandDueClientTasks(ctx); err != nil {
		return err
	}
	if err := e.applyOperationResults(ctx); err != nil {
		return err
	}
	return e.dispatchDueTasks(ctx)
}

func (e *Engine) expandDueClientTasks(ctx context.Context) error {
	var clientTasks []models.ClientTask
	if err := e.db.WithContext(ctx).
		Where("repeat_expanded = ? AND next_action_at <= ?", false, time.Now()).
		Order("next_action_at ASC, id ASC").
		Limit(e.config.ExpansionBatchSize).
		Find(&clientTasks).Error; err != nil {
		return err
	}
	for i := range clientTasks {
		if err := e.expandClientTask(ctx, clientTasks[i]); err != nil {
			_ = e.db.WithContext(ctx).Model(&models.ClientTask{}).
				Where("id = ?", clientTasks[i].ID).
				Updates(map[string]any{
					"expansion_error": err.Error(),
					"next_action_at":  time.Now().Add(e.config.RetryInterval),
				}).Error
		}
	}
	return nil
}

func (e *Engine) expandClientTask(ctx context.Context, clientTask models.ClientTask) error {
	var submission Submission
	if err := json.Unmarshal([]byte(clientTask.Submission), &submission); err != nil {
		return fmt.Errorf("decode client task %d submission: %w", clientTask.ID, err)
	}
	var expandedTasks []models.InferenceTask
	err := e.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.ClientTask
		if err := tx.Where("id = ? AND repeat_expanded = ?", clientTask.ID, false).First(&current).Error; err != nil {
			return err
		}
		now := time.Now()
		tasks := make([]models.InferenceTask, 0, current.EffectiveRepeatNum)
		for i := 0; i < current.EffectiveRepeatNum; i++ {
			taskIDBytes := make([]byte, 32)
			if _, err := rand.Read(taskIDBytes); err != nil {
				return err
			}
			taskID := hexutil.Encode(taskIDBytes)
			nonce, commitment := models.GenerateTaskIDCommitment(taskID)
			tasks = append(tasks, models.InferenceTask{
				ClientID:         current.ClientID,
				ClientTaskID:     current.ID,
				TaskArgs:         submission.TaskArgs,
				TaskType:         submission.TaskType,
				TaskModelIDs:     models.StringArray(submission.TaskModelIDs),
				TaskVersion:      submission.TaskVersion,
				TaskFee:          submission.TaskFee,
				MinVram:          submission.MinVram,
				RequiredGPU:      submission.RequiredGPU,
				RequiredGPUVram:  submission.RequiredGPUVram,
				TaskSize:         submission.TaskSize,
				Timeout:          submission.Timeout,
				TaskID:           taskID,
				Nonce:            nonce,
				TaskIDCommitment: commitment,
				NextActionAt:     now,
			})
		}
		if err := tx.Create(&tasks).Error; err != nil {
			return err
		}
		expandedTasks = append(expandedTasks, tasks...)
		return tx.Model(&models.ClientTask{}).Where("id = ?", current.ID).Updates(map[string]any{
			"repeat_expanded": true,
			"submission":      "",
			"expansion_error": "",
			"next_action_at":  now,
		}).Error
	})
	if err != nil {
		return err
	}
	tasktrace.RegisterClientTasks(clientTask.ID, expandedTasks, "primary")
	return nil
}

func (e *Engine) recalculateClientTask(ctx context.Context, tx *gorm.DB, clientTaskID uint) error {
	var clientTask models.ClientTask
	if err := tx.WithContext(ctx).First(&clientTask, clientTaskID).Error; err != nil {
		return err
	}
	if clientTask.Status != models.ClientTaskStatusRunning || !clientTask.RepeatExpanded {
		return nil
	}
	var tasks []models.InferenceTask
	if err := tx.WithContext(ctx).
		Where("client_task_id = ? AND task_type IN ?", clientTaskID, []models.ChainTaskType{models.TaskTypeSD, models.TaskTypeLLM}).
		Find(&tasks).Error; err != nil {
		return err
	}
	if len(tasks) == 0 {
		return nil
	}
	allFinished := true
	success := false
	type groupState struct {
		allFinished bool
		success     bool
	}
	groups := make(map[string]groupState)
	for i := range tasks {
		group := groups[tasks[i].TaskID]
		if _, exists := groups[tasks[i].TaskID]; !exists {
			group.allFinished = true
		}
		if tasks[i].Status == models.InferenceTaskResultDownloaded {
			success = true
			group.success = true
		}
		if !tasks[i].Finished() {
			allFinished = false
			group.allFinished = false
		}
		groups[tasks[i].TaskID] = group
	}
	failedCount := 0
	for _, group := range groups {
		if group.allFinished && !group.success {
			failedCount++
		}
	}
	updates := map[string]any{"failed_count": failedCount}
	if success {
		updates["status"] = models.ClientTaskStatusSuccess
	} else if allFinished {
		updates["status"] = models.ClientTaskStatusFailed
	}
	return tx.WithContext(ctx).Model(&models.ClientTask{}).Where("id = ?", clientTaskID).Updates(updates).Error
}
