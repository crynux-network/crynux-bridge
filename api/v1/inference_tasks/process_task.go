package inference_tasks

import (
	"context"
	"crynux_bridge/api/v1/response"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"gorm.io/gorm"
)

const defaultLLMTaskWaitTimeout = 3 * time.Minute

func ProcessGPTTask(ctx context.Context, db *gorm.DB, in *TaskInput) (*models.GPTTaskResponse, *models.InferenceTask, error) {
	waitTimeout := defaultLLMTaskWaitTimeout
	if in != nil && in.Timeout != nil && *in.Timeout > 0 {
		waitTimeout = time.Duration(*in.Timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()
	/* 1. Create GPT task by function CreateTask */
	taskResponse, err := DoCreateTask(ctx, in)
	if err != nil {
		return nil, nil, err
	}

	/* 2. Get tasks, wait until they are finished and the taks result is downloaded  */
	tasks := taskResponse.Data.InferenceTasks
	if len(tasks) == 0 {
		err := errors.New("no task created")
		return nil, nil, response.NewExceptionResponse(err)
	}
	taskGroups, err := models.WaitAllTaskGroup(ctx, db, tasks)
	if err != nil {
		if timeoutErr := mapTaskTimeoutError(err, waitTimeout); timeoutErr != nil {
			return nil, nil, timeoutErr
		}
		return nil, nil, response.NewExceptionResponse(err)
	}
	resultDownloadedTask, err := models.WaitResultTask(ctx, db, taskGroups)
	if err != nil {
		if timeoutErr := mapTaskTimeoutError(err, waitTimeout); timeoutErr != nil {
			return nil, nil, timeoutErr
		}
		return nil, nil, response.NewExceptionResponse(err)
	}

	/* 3. Read task result and return */
	results, err := readGPTTaskResults(resultDownloadedTask)
	if err != nil {
		return nil, nil, response.NewExceptionResponse(err)
	}

	// without stream, the response is a single object, i.e. results[0]
	gptTaskResponse := results[0]

	return &gptTaskResponse, resultDownloadedTask, nil
}

func ProcessSDTask(ctx context.Context, db *gorm.DB, in *TaskInput) ([]string, *models.InferenceTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	/* 1. Create SD task by function CreateTask */
	taskResponse, err := DoCreateTask(ctx, in)
	if err != nil {
		return nil, nil, err
	}

	/* 2. Get tasks, wait until they are finished and the taks result is downloaded  */
	tasks := taskResponse.Data.InferenceTasks
	if len(tasks) == 0 {
		err := errors.New("no task created")
		return nil, nil, response.NewExceptionResponse(err)
	}
	taskGroups, err := models.WaitAllTaskGroup(ctx, db, tasks)
	if err != nil {
		if timeoutErr := mapTaskTimeoutError(err, 3*time.Minute); timeoutErr != nil {
			return nil, nil, timeoutErr
		}
		return nil, nil, response.NewExceptionResponse(err)
	}
	resultDownloadedTask, err := models.WaitResultTask(ctx, db, taskGroups)
	if err != nil {
		if timeoutErr := mapTaskTimeoutError(err, 3*time.Minute); timeoutErr != nil {
			return nil, nil, timeoutErr
		}
		return nil, nil, response.NewExceptionResponse(err)
	}

	/* 3. Read task result and return */
	results, err := readSDTaskResults(resultDownloadedTask)
	if err != nil {
		return nil, nil, response.NewExceptionResponse(err)
	}

	return results, resultDownloadedTask, nil
}

func ProcessSDFTLoraTask(ctx context.Context, db *gorm.DB, in *TaskInput) (string, *models.InferenceTask, error) {
	/* 1. Create SD task by function CreateTask */
	taskResponse, err := DoCreateTask(ctx, in)
	if err != nil {
		return "", nil, err
	}

	/* 2. Get tasks, wait until they are finished and the taks result is downloaded  */
	tasks := taskResponse.Data.InferenceTasks
	if len(tasks) == 0 {
		err := errors.New("no task created")
		return "", nil, response.NewExceptionResponse(err)
	}
	taskGroups, err := models.WaitAllTaskGroup(ctx, db, tasks)
	if err != nil {
		if timeoutErr := mapTaskTimeoutError(err, 0); timeoutErr != nil {
			return "", nil, timeoutErr
		}
		return "", nil, response.NewExceptionResponse(err)
	}
	resultDownloadedTask, err := models.WaitResultTask(ctx, db, taskGroups)
	if err != nil {
		if timeoutErr := mapTaskTimeoutError(err, 0); timeoutErr != nil {
			return "", nil, timeoutErr
		}
		return "", nil, response.NewExceptionResponse(err)
	}

	/* 3. Read task result and return */
	results, err := readSDFTLoraTaskResults(resultDownloadedTask)
	if err != nil {
		return "", nil, response.NewExceptionResponse(err)
	}

	return results, resultDownloadedTask, nil
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

func readSDFTLoraTaskResults(task *models.InferenceTask) (string, error) {
	if task.TaskType != models.TaskTypeSDFTLora {
		err := errors.New("unsupported task type")
		return "", err
	}

	appConfig := config.GetConfig()
	taskFolder := path.Join(appConfig.DataDir.InferenceTasks, task.TaskIDCommitment)
	filename := path.Join(taskFolder, "checkpoint.zip")

	return filename, nil
}

func mapTaskTimeoutError(err error, waitTimeout time.Duration) error {
	if errors.Is(err, models.ErrTaskTimeout) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		if waitTimeout > 0 {
			return response.NewExceptionResponse(fmt.Errorf("task timeout: task did not finish within %s", waitTimeout.String()))
		}
		return response.NewExceptionResponse(errors.New("task timeout: task did not finish before the request deadline"))
	}
	return nil
}
