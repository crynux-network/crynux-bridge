package inference_tasks

import (
	"context"
	"crynux_bridge/api/v1/response"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/taskengine"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"gorm.io/gorm"
)

type ClientTaskCreatedCallback func(*models.ClientTask)

func ProcessGPTTask(ctx context.Context, db *gorm.DB, clientID string, in *TaskInput, onTaskCreated ...ClientTaskCreatedCallback) (*models.GPTTaskResponse, *models.InferenceTask, error) {
	taskResponse, err := DoCreateTask(ctx, clientID, in)
	if err != nil {
		return nil, nil, err
	}
	engine, err := taskengine.Default()
	if err != nil {
		return nil, nil, response.NewExceptionResponse(err)
	}
	for _, callback := range onTaskCreated {
		if callback != nil {
			callback(taskResponse.Data)
		}
	}
	if err := engine.RegisterTraceTasks(ctx, taskResponse.Data.ID); err != nil {
		return nil, nil, response.NewExceptionResponse(err)
	}
	pollInterval := config.GetConfig().Task.TaskStatusPollInterval
	for {
		status, err := engine.Status(ctx, clientID, taskResponse.Data.ID)
		if err != nil {
			return nil, nil, response.NewExceptionResponse(err)
		}
		if status.Status == models.ClientTaskStatusFailed {
			return nil, nil, response.NewExceptionResponse(taskengine.ErrTaskFailed)
		}
		if status.Status == models.ClientTaskStatusSuccess {
			break
		}
		select {
		case <-ctx.Done():
			return nil, nil, mapTaskTimeoutError(ctx.Err())
		case <-time.After(pollInterval):
		}
	}
	result, err := engine.Result(ctx, clientID, taskResponse.Data.ID)
	if err != nil {
		return nil, nil, response.NewExceptionResponse(err)
	}
	resultDownloadedTask := &models.InferenceTask{
		RootModel: models.RootModel{
			ID:        result.TaskID,
			CreatedAt: result.CreatedAt,
		},
		TaskID:           result.TaskIDValue,
		TaskIDCommitment: result.TaskIDCommitment,
		TaskType:         result.TaskType,
		TaskSize:         result.TaskSize,
		Status:           models.InferenceTaskResultDownloaded,
	}
	results, err := readGPTTaskResults(resultDownloadedTask)
	if err != nil {
		return nil, resultDownloadedTask, response.NewExceptionResponse(err)
	}
	gptTaskResponse := results[0]
	return &gptTaskResponse, resultDownloadedTask, nil
}

func readGPTTaskResults(task *models.InferenceTask) ([]models.GPTTaskResponse, error) {
	if task.TaskType != models.TaskTypeLLM {
		err := errors.New("unsupported task type")
		return nil, err
	}

	appConfig := config.GetConfig()
	taskFolder := path.Join(appConfig.DataDir.InferenceTasks, task.TaskIDCommitment)
	ext := "json"

	// 1. create a results array
	results := make([]models.GPTTaskResponse, task.TaskSize)
	var wg sync.WaitGroup
	errCh := make(chan error, int(task.TaskSize))

	// 2. read all result files in parallel
	for i := 0; i < int(task.TaskSize); i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			filename := path.Join(taskFolder, fmt.Sprintf("%d.%s", index, ext))
			data, err := os.ReadFile(filename)
			if err != nil {
				errCh <- fmt.Errorf("readGPTTaskResults: failed to read file %s, error: %w", filename, err)
				return
			}

			var result models.GPTTaskResponse

			if err := json.Unmarshal(data, &result); err != nil {
				errCh <- fmt.Errorf("failed to unmarshal json file %s, error: %w", filename, err)
				return
			}

			// 3. save result to results array, according to the index
			results[index] = result
		}(i)
	}

	// 4. wait for all goroutines to finish
	go func() { wg.Wait(); close(errCh) }()
	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

func readSDTaskResults(task *models.InferenceTask) ([]string, error) {
	if task.TaskType != models.TaskTypeSD {
		err := errors.New("unsupported task type")
		return nil, err
	}

	appConfig := config.GetConfig()
	taskFolder := path.Join(appConfig.DataDir.InferenceTasks, task.TaskIDCommitment)
	ext := "png"

	results := make([]string, task.TaskSize)
	for i := 0; i < int(task.TaskSize); i++ {
		filename := path.Join(taskFolder, fmt.Sprintf("%d.%s", i, ext))
		results[i] = filename
	}

	return results, nil
}

func mapTaskTimeoutError(err error) error {
	if errors.Is(err, models.ErrTaskTimeout) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return response.NewExceptionResponse(fmt.Errorf("task wait interrupted: %w", err))
	}
	return nil
}
