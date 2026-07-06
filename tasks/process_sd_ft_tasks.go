package tasks

import (
	"archive/zip"
	"context"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/tasktrace"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func ProcessSDFTTasks(ctx context.Context) {
	type result struct {
		ID uint `json:"id"`
	}

	lastID := uint(0)
	limit := 100

	for {
		tasks, err := func(ctx context.Context) ([]*models.ClientTask, error) {
			var results []result

			dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			err := config.GetDB().WithContext(dbCtx).Table("client_tasks").
				Select("distinct client_tasks.id").
				Joins("LEFT JOIN inference_tasks ON inference_tasks.client_task_id = client_tasks.id").
				Where("client_tasks.status = ?", models.ClientTaskStatusRunning).
				Where("inference_tasks.task_type = ?", models.TaskTypeSDFTLora).
				Where("client_tasks.id > ?", lastID).
				Order("client_tasks.id ASC").
				Limit(limit).
				Scan(&results).
				Error
			if err != nil {
				return nil, err
			}

			var tasks []*models.ClientTask
			var ids []uint
			for _, result := range results {
				ids = append(ids, result.ID)
			}

			if len(ids) > 0 {
				dbCtx1, cancel1 := context.WithTimeout(ctx, 10*time.Second)
				defer cancel1()
				err := config.GetDB().WithContext(dbCtx1).Model(&models.ClientTask{}).
					Where("id IN (?)", ids).
					Order("id ASC").
					Find(&tasks).
					Error
				if err != nil {
					return nil, err
				}
			}

			return tasks, nil
		}(ctx)
		if err != nil {
			log.Errorf("GetSDFTTasks: cannot get tasks: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if len(tasks) == 0 {
			time.Sleep(5 * time.Second)
			continue
		}

		for _, task := range tasks {
			go func(ctx context.Context, task *models.ClientTask) {
				var inferenceTaskID uint
				for task.Status == models.ClientTaskStatusRunning {
					inferenceTask, err := getRunningSDFTInferenceTask(ctx, task.ID)
					if err != nil {
						log.Errorf("ProcessSDFTTasks: cannot get running inference task of client task %d: %v", task.ID, err)
						return
					}
					if inferenceTask == nil {
						log.Errorf("ProcessSDFTTasks: no running inference task of client task %d", task.ID)
						return
					}
					if inferenceTaskID == inferenceTask.ID {
						log.Errorf("ProcessSDFTTasks: get the same inference task of client task %d", task.ID)
					}
					inferenceTaskID = inferenceTask.ID
					func() {
						duration := time.Duration(inferenceTask.Timeout) * time.Second
						ctx, cancel := context.WithTimeout(ctx, duration)
						defer cancel()
						err = processSDFTTaskWithRetry(ctx, task, inferenceTask)
						if err != nil {
							log.Errorf("ProcessSDFTTasks: cannot process task %d: %v", task.ID, err)
						}
					}()
				}
			}(ctx, task)
		}

		lastID = tasks[len(tasks)-1].ID
	}
}

func getRunningSDFTInferenceTask(ctx context.Context, clientTaskID uint) (*models.InferenceTask, error) {
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var inferenceTask models.InferenceTask
	err := config.GetDB().WithContext(dbCtx).Transaction(func(tx *gorm.DB) error {
		type result struct {
			ID uint `json:"id"`
		}
		r := result{}
		err := tx.Table("inference_tasks").
			Select("max(id) as id").
			Where("client_task_id = ?", clientTaskID).
			Where("task_type = ?", models.TaskTypeSDFTLora).
			First(&r).Error
		if err != nil {
			return err
		}
		if r.ID == 0 {
			return nil
		}
		return tx.Model(&models.InferenceTask{}).Where("id = ?", r.ID).First(&inferenceTask).Error
	})
	if err != nil {
		return nil, err
	}
	return &inferenceTask, nil
}

func processSDFTTaskWithRetry(ctx context.Context, clientTask *models.ClientTask, inferenceTask *models.InferenceTask) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := processSDFTTask(ctx, clientTask, inferenceTask)
			if err != nil {
				time.Sleep(3 * time.Second)
			} else {
				return nil
			}
		}
	}
}

func processSDFTTask(ctx context.Context, clientTask *models.ClientTask, inferenceTask *models.InferenceTask) error {
	log.Infof("processSDFTTask: process client task %d inference task %d", clientTask.ID, inferenceTask.ID)
	taskGroup, err := models.WaitTaskGroup(ctx, config.GetDB(), inferenceTask)
	if err != nil {
		return err
	}

	resultDownloadedTask, err := models.WaitResultTask(ctx, config.GetDB(), taskGroup)

	if err == models.ErrTaskEndWithoutResult {
		return processFailedSDFTTask(ctx, clientTask, inferenceTask)
	}
	if err != nil {
		return err
	}

	if resultDownloadedTask != nil {
		return processResultDownloadedSDFTTask(ctx, clientTask, resultDownloadedTask)
	}

	return nil
}

func processResultDownloadedSDFTTask(ctx context.Context, clientTask *models.ClientTask, task *models.InferenceTask) error {
	appConfig := config.GetConfig()

	resultFilePath := filepath.Join(appConfig.DataDir.InferenceTasks, task.TaskIDCommitment, "result.zip")
	if _, err := os.Stat(resultFilePath); !os.IsNotExist(err) {
		log.Infof("processSDFTTasks: client task %d inference task %d result file already exists", clientTask.ID, task.ID)

		if clientTask.Status != models.ClientTaskStatusSuccess {
			clientTask.Status = models.ClientTaskStatusSuccess
			if err := clientTask.Update(ctx, config.GetDB(), clientTask); err != nil {
				return err
			}
		}
		return nil
	}

	checkpointFilePath := filepath.Join(appConfig.DataDir.InferenceTasks, task.TaskIDCommitment, "checkpoint.zip")
	if _, err := os.Stat(checkpointFilePath); os.IsNotExist(err) {
		log.Errorf("processSDFTTasks: checkpoint file of task %s not found", task.TaskIDCommitment)
		return errors.New("checkpoint file not found")
	}

	// unzip checkpoint file and check if FINISH file exists
	finished, err := func() (bool, error) {
		zipFile, err := zip.OpenReader(checkpointFilePath)
		if err != nil {
			log.Errorf("processSDFTTasks: cannot open checkpoint file of task %s: %v", task.TaskIDCommitment, err)
			return false, err
		}
		defer zipFile.Close()

		for _, file := range zipFile.File {
			if file.Name == "FINISH" {
				return true, nil
			}
		}
		return false, nil
	}()

	if err != nil {
		log.Errorf("processSDFTTasks: cannot check checkpoint file of task %s: %v", task.TaskIDCommitment, err)
		return err
	}

	if finished {
		log.Infof("processSDFTTasks: client task %d inference task %d is finished", clientTask.ID, task.ID)
		// rename the checkpoint file to result.zip
		err = os.Rename(checkpointFilePath, filepath.Join(appConfig.DataDir.InferenceTasks, task.TaskIDCommitment, "result.zip"))
		if err != nil {
			log.Errorf("processSDFTTasks: cannot rename checkpoint file of task %s: %v", task.TaskIDCommitment, err)
			return err
		}
		// update client task status
		clientTask.Status = models.ClientTaskStatusSuccess
		if err := clientTask.Update(ctx, config.GetDB(), clientTask); err != nil {
			return err
		}
		tasktrace.RecordEvent(task, "finetune_result_ready", nil)
		return nil
	} else {
		// sd ft task is not finished, create a new task with the same client task id and task args, except the checkpoint file
		newTaskArgs, err := models.ChangeSDFTTaskArgsCheckpoint(task.TaskArgs, checkpointFilePath)
		if err != nil {
			log.Errorf("processSDFTTasks: cannot change task args of task %s: %v", task.TaskIDCommitment, err)
			return err
		}
		taskIDBytes := make([]byte, 32)
		rand.Read(taskIDBytes)
		newTaskID := hexutil.Encode(taskIDBytes)

		newTask := &models.InferenceTask{
			ClientID:        task.ClientID,
			ClientTaskID:    task.ClientTaskID,
			TaskArgs:        newTaskArgs,
			TaskType:        task.TaskType,
			TaskModelIDs:    task.TaskModelIDs,
			TaskVersion:     task.TaskVersion,
			TaskFee:         task.TaskFee,
			MinVram:         task.MinVram,
			RequiredGPU:     task.RequiredGPU,
			RequiredGPUVram: task.RequiredGPUVram,
			TaskSize:        task.TaskSize,
			TaskID:          newTaskID,
			Timeout:         task.Timeout,
		}

		err = newTask.Save(ctx, config.GetDB())
		if err != nil {
			log.Errorf("processSDFTTasks: cannot save new task %s: %v", newTaskID, err)
			return err
		}
		tasktrace.RegisterTask(task, newTask, "retry")
		tasktrace.RecordEvent(task, "finetune_retry_task_created", map[string]any{
			"new_task_local_id": newTask.ID,
			"new_task_id":       newTask.TaskID,
		})

		return nil
	}
}

func processFailedSDFTTask(ctx context.Context, clientTask *models.ClientTask, task *models.InferenceTask) error {
	clientTask.FailedCount += 1
	if clientTask.FailedCount > 3 {
		clientTask.Status = models.ClientTaskStatusFailed
	}
	if err := clientTask.Update(ctx, config.GetDB(), clientTask); err != nil {
		return err
	}
	if clientTask.Status == models.ClientTaskStatusFailed {
		return nil
	}

	taskIDBytes := make([]byte, 32)
	rand.Read(taskIDBytes)
	newTaskID := hexutil.Encode(taskIDBytes)

	newTask := &models.InferenceTask{
		ClientID:        task.ClientID,
		ClientTaskID:    task.ClientTaskID,
		TaskArgs:        task.TaskArgs,
		TaskType:        task.TaskType,
		TaskModelIDs:    task.TaskModelIDs,
		TaskVersion:     task.TaskVersion,
		TaskFee:         task.TaskFee,
		MinVram:         task.MinVram,
		RequiredGPU:     task.RequiredGPU,
		RequiredGPUVram: task.RequiredGPUVram,
		TaskSize:        task.TaskSize,
		TaskID:          newTaskID,
		Timeout:         task.Timeout,
	}

	err := newTask.Save(ctx, config.GetDB())
	if err != nil {
		return err
	}
	tasktrace.RegisterTask(task, newTask, "retry")
	tasktrace.RecordEvent(task, "finetune_retry_task_created", map[string]any{
		"new_task_local_id": newTask.ID,
		"new_task_id":       newTask.TaskID,
	})

	return nil
}
