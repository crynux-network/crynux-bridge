package tasks

import (
	"context"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/relay"
	"crynux_bridge/tasktrace"
	"errors"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

func cancelTask(ctx context.Context, task *models.InferenceTask) error {
	log.Infof("CancelTasks: start to cancel task %d", task.ID)
	taskIDCommitment := task.TaskIDCommitment
	newTask := &models.InferenceTask{}
	// If task is not uploaded yet, cancel it directly
	if len(task.TaskIDCommitment) == 0 {
		tasktrace.RecordEvent(task, "task_cancel_started", nil)
		newTask.Status = models.InferenceTaskEndAborted
		newTask.AbortReason = models.TaskAbortTimeout
		if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
			log.Errorf("CancelTasks: cannot save task %d status: %v", task.ID, err)
			return err
		}
		task.Status = newTask.Status
		tasktrace.RecordEvent(task, "task_cancelled", map[string]any{
			"abort_reason": newTask.AbortReason,
		})
		log.Infof("CancelTasks: task %d canceled successfully", task.ID)
		return nil
	}

	chainTask, err := relay.GetTaskByCommitment(ctx, taskIDCommitment)
	if err != nil {
		var relayErr relay.RelayError
		if errors.As(err, &relayErr) && strings.Contains(relayErr.ErrorMessage, "Task not found") {
			log.Infof("CancelTasks: task %d not found", task.ID)
			tasktrace.RecordEvent(task, "task_cancel_started", map[string]any{
				"reason": "relay_task_not_found",
			})
			newTask.Status = models.InferenceTaskEndAborted
			newTask.AbortReason = models.TaskAbortTimeout
			if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
				log.Errorf("CancelTasks: cannot save task %d status: %v", task.ID, err)
				return err
			}
			task.Status = newTask.Status
			tasktrace.RecordEvent(task, "task_cancelled", map[string]any{
				"abort_reason": newTask.AbortReason,
			})
			log.Infof("CancelTasks: task %d canceled successfully", task.ID)
			return nil
		}

		log.Errorf("CancelTasks: cannot get task %d : %v", task.ID, err)
		return err
	}

	// If task is already finished, we don't need to cancel it
	// Otherwise, cancel it
	chainTaskStatus := models.ChainTaskStatus(chainTask.Status)
	if chainTaskStatus == models.ChainTaskEndSuccess || chainTaskStatus == models.ChainTaskEndGroupSuccess {
		newTask.Status = models.InferenceTaskEndSuccess
	} else if chainTaskStatus == models.ChainTaskEndGroupRefund {
		newTask.Status = models.InferenceTaskEndGroupRefund
	} else if chainTaskStatus == models.ChainTaskEndInvalidated {
		newTask.Status = models.InferenceTaskEndInvalidated
	} else if chainTaskStatus == models.ChainTaskEndAborted {
		newTask.Status = models.InferenceTaskEndAborted
		newTask.AbortReason = models.TaskAbortReason(chainTask.AbortReason)
	} else {
		tasktrace.RecordEvent(task, "task_cancel_started", nil)
		if err := relay.CancelTask(ctx, task, models.TaskAbortTimeout); err != nil {
			log.Errorf("CancelTasks: cannot cancel task %d : %v", task.ID, err)
			return err
		}
		newTask.Status = models.InferenceTaskEndAborted
		newTask.AbortReason = models.TaskAbortTimeout
	}
	if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
		log.Errorf("CancelTasks: cannot save task %d status: %v", task.ID, err)
		return err
	}
	task.Status = newTask.Status
	tasktrace.RecordEvent(task, "task_cancelled", map[string]any{
		"abort_reason": newTask.AbortReason,
		"chain_status": chainTaskStatus,
	})
	log.Infof("CancelTasks: task %d canceled successfully", task.ID)
	return nil
}

func getTasksNeedCancel(ctx context.Context) ([]models.InferenceTask, error) {
	limit := 100
	offset := 0

	allTasks := []models.InferenceTask{}
	for {
		tasks, err := func(ctx context.Context, offset, limit int) ([]models.InferenceTask, error) {
			var tasks []models.InferenceTask

			dbCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			err := config.GetDB().WithContext(dbCtx).Model(&models.InferenceTask{}).
				Where("status = ?", models.InferenceTaskNeedCancel).
				Order("id ASC").
				Limit(limit).
				Offset(offset).
				Find(&tasks).
				Error
			if err != nil {
				return nil, err
			}
			return tasks, nil
		}(ctx, offset, limit)
		if err != nil {
			return nil, err
		}
		allTasks = append(allTasks, tasks...)
		if len(tasks) < limit {
			break
		}
		offset += limit
	}

	return allTasks, nil
}

func CancelTasks(ctx context.Context) {
	for {
		tasks, err := getTasksNeedCancel(ctx)
		if err != nil {
			log.Errorf("CancelTasks: cannot get tasks need to cancel: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		log.Infof("CancelTasks: %d tasks need to cancel", len(tasks))

		for _, task := range tasks {
			if err := cancelTask(ctx, &task); err != nil {
				log.Errorf("CancelTasks: cannot cancel task %d due to %v", task.ID, err)
				continue
			}
		}

		time.Sleep(time.Minute)
	}
}
