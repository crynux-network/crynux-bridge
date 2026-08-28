package inference_tasks

import (
	"context"
	"errors"

	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/models"
	"crynux_bridge/taskengine"

	"gorm.io/gorm"
)

func loadRawClientTask(
	ctx context.Context,
	db *gorm.DB,
	authorization string,
	clientTaskID uint,
) (*models.ClientTask, error) {
	apiKey, err := tools.ValidateReadAuthorization(ctx, db, authorization)
	if err != nil {
		return nil, err
	}
	engine, err := taskengine.Default()
	if err != nil {
		client, clientErr := tools.GetClient(ctx, db, apiKey.ClientID)
		if clientErr != nil {
			return nil, rawClientTaskLoadError(clientErr)
		}
		clientTask, clientTaskErr := tools.GetClientTask(ctx, db, client.ID, clientTaskID)
		if clientTaskErr != nil {
			return nil, rawClientTaskLoadError(clientTaskErr)
		}
		return clientTask, nil
	}
	clientTask, err := engine.Status(ctx, apiKey.ClientID, clientTaskID)
	if err != nil {
		return nil, rawClientTaskLoadError(err)
	}
	return clientTask, nil
}

func rawClientTaskLoadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, taskengine.ErrTaskNotFound) {
		return response.NewValidationErrorResponse("client_task_id", "Client task not found")
	}
	return response.NewExceptionResponse(err)
}

func selectRawInferenceTask(tasks []models.InferenceTask) *models.InferenceTask {
	predicates := []func(models.InferenceTask) bool{
		func(task models.InferenceTask) bool {
			return task.Status == models.InferenceTaskResultDownloaded
		},
		func(task models.InferenceTask) bool {
			return !task.Finished()
		},
		func(task models.InferenceTask) bool {
			return task.Status == models.InferenceTaskEndAborted ||
				task.Status == models.InferenceTaskEndInvalidated
		},
		func(task models.InferenceTask) bool {
			return task.Status == models.InferenceTaskEndGroupRefund
		},
		func(models.InferenceTask) bool {
			return true
		},
	}
	for _, predicate := range predicates {
		if selected := selectEarliestTask(tasks, predicate); selected != nil {
			return selected
		}
	}
	return nil
}

func selectEarliestTask(
	tasks []models.InferenceTask,
	predicate func(models.InferenceTask) bool,
) *models.InferenceTask {
	var selected *models.InferenceTask
	for i := range tasks {
		task := &tasks[i]
		if !predicate(*task) {
			continue
		}
		if selected == nil ||
			task.UpdatedAt.Before(selected.UpdatedAt) ||
			task.UpdatedAt.Equal(selected.UpdatedAt) && task.ID < selected.ID {
			selected = task
		}
	}
	return selected
}

func selectSDFTRawInferenceTask(tasks []models.InferenceTask) *models.InferenceTask {
	if len(tasks) == 0 {
		return nil
	}
	selected := tasks[0]
	for _, task := range tasks[1:] {
		if task.Status == models.InferenceTaskResultDownloaded {
			if selected.Status != models.InferenceTaskResultDownloaded ||
				selected.UpdatedAt.After(task.UpdatedAt) {
				selected = task
			}
		} else if selected.Status == models.InferenceTaskEndAborted &&
			task.Status != models.InferenceTaskEndAborted {
			selected = task
		}
	}
	return &selected
}
