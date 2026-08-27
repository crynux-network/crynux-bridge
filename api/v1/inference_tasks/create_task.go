package inference_tasks

import (
	"context"
	"crynux_bridge/api/ratelimit"
	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/tasktrace"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TaskInput struct {
	TaskArgs        string                `json:"task_args" description:"Task args" validate:"required"`
	TaskType        *models.ChainTaskType `json:"task_type" description:"Task type. 0 - SD task, 1 - LLM task, 2 - SD Finetune task" validate:"required"`
	TaskVersion     *string               `json:"task_version,omitempty" description:"Task version. Default is task.default_task_version" validate:"omitempty"`
	MinVram         *uint64               `json:"min_vram,omitempty" description:"Task minimal vram requirement" validate:"omitempty"`
	RequiredGPU     string                `json:"required_gpu,omitempty" description:"Task required GPU name" validate:"omitempty"`
	RequiredGPUVram uint64                `json:"required_gpu_vram,omitempty" description:"Task required GPU Vram" validate:"omitempty"`
	RepeatNum       *int                  `json:"repeat_num,omitempty" description:"Task repeat number" validate:"omitempty"`
	TaskFee         *string               `json:"task_fee" description:"Final task fee in Wei as a decimal string" validate:"required"`
	Timeout         *uint64               `json:"timeout,omitempty" description:"Task timeout" validate:"omitempty"`
}

type CreateTaskInput struct {
	TaskInput
	Authorization string `header:"Authorization" validate:"required" description:"API key"`
}

type TaskResponse struct {
	response.Response
	Data *models.ClientTask `json:"data"`
}

func getDefaultMinVram(taskType models.ChainTaskType, taskArgs string) (uint64, error) {
	if taskType == models.TaskTypeSD {
		baseModel, err := models.GetSDTaskConfigBaseModel(taskArgs)
		if err != nil {
			return 0, err
		}
		switch baseModel {
		case "crynux-network/stable-diffusion-v1-5":
			return 8, nil
		case "crynux-network/sdxl-turbo":
			return 14, nil
		default:
			return 10, nil
		}
	} else {
		return 24, nil
	}
}

func getTaskSize(taskType models.ChainTaskType, taskArgs string) (uint64, error) {
	if taskType == models.TaskTypeSD {
		num, err := models.GetTaskConfigNumImages(taskArgs)
		if err != nil {
			return 0, err
		}
		return uint64(num), nil
	} else {
		return 1, nil
	}
}

func getDefaultTaskFeeCNX(taskType models.ChainTaskType, taskArgs string, appConfig *config.AppConfig) (string, error) {
	var feeCNX string
	switch taskType {
	case models.TaskTypeSD:
		baseModel, err := models.GetSDTaskConfigBaseModel(taskArgs)
		if err != nil {
			return "", err
		}
		if baseModel == "crynux-network/sdxl-turbo" {
			feeCNX = appConfig.Task.DefaultSDXLTaskFeeCNX
		} else {
			feeCNX = appConfig.Task.DefaultSDTaskFeeCNX
		}
	case models.TaskTypeLLM:
		feeCNX = appConfig.Task.DefaultLLMTaskFeeCNX
	case models.TaskTypeSDFTLora:
		feeCNX = appConfig.Task.DefaultSDFinetuneTaskFeeCNX
	default:
		return "", fmt.Errorf("unsupported task type %d", taskType)
	}
	return feeCNX, nil
}

func resolveTaskFee(
	requested *string,
	taskType models.ChainTaskType,
	taskArgs string,
	taskSize uint64,
	appConfig *config.AppConfig,
) (string, error) {
	if requested != nil {
		return config.ValidateWei(*requested)
	}
	feeCNX, err := getDefaultTaskFeeCNX(taskType, taskArgs, appConfig)
	if err != nil {
		return "", err
	}
	baseTaskFeeWei, err := config.CNXToWei(feeCNX)
	if err != nil {
		return "", err
	}
	return config.MultiplyWei(baseTaskFeeWei, taskSize)
}

func resolveTaskTimeout(taskType models.ChainTaskType, requested *uint64, appConfig *config.AppConfig) (uint64, error) {
	if taskType == models.TaskTypeSDFTLora {
		if requested != nil {
			if *requested == 0 {
				return 0, response.NewValidationErrorResponse("timeout", "timeout must be greater than 0 for SD finetune tasks")
			}
			return *requested, nil
		}
		timeout := appConfig.Task.SDFinetuneTimeout * 60
		if timeout == 0 {
			return 0, response.NewValidationErrorResponse("timeout", "timeout must be greater than 0 for SD finetune tasks")
		}
		return timeout, nil
	}
	if requested != nil {
		return 0, response.NewValidationErrorResponse("timeout", "timeout is only supported for SD finetune tasks")
	}
	return 0, nil
}

func buildTasks(in *TaskInput, client *models.Client, clientTask *models.ClientTask, appConfig *config.AppConfig) ([]*models.InferenceTask, error) {
	taskType := *in.TaskType

	var taskVersion = appConfig.Task.DefaultTaskVersion
	if in.TaskVersion != nil {
		taskVersion = *in.TaskVersion
	}

	result, err := models.ValidateTaskArgsJsonStr(in.TaskArgs, taskType)
	if err != nil {
		if isTaskArgsJSONError(err) {
			return nil, response.NewValidationErrorResponse("task_args", fmt.Sprintf("task_args must be valid JSON: %v", err))
		}
		return nil, response.NewExceptionResponse(err)
	}

	if result != nil {
		return nil, response.NewValidationErrorResponse("task_args", fmt.Sprintf("invalid task_args: %s", result.Error()))
	}

	var minVram uint64

	if in.MinVram == nil {
		// task args has been validated, so there should be no error
		minVram, _ = getDefaultMinVram(taskType, in.TaskArgs)
	} else {
		minVram = *in.MinVram
	}

	// task args has been validated, so there should be no error
	taskSize, _ := getTaskSize(taskType, in.TaskArgs)
	taskFee, err := resolveTaskFee(in.TaskFee, taskType, in.TaskArgs, taskSize, appConfig)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}

	repeatNum := appConfig.Task.RepeatNum
	if in.RepeatNum != nil {
		repeatNum = *in.RepeatNum
	}

	modelIDs, err := models.GetTaskConfigModelIDs(in.TaskArgs, taskType)
	if err != nil {
		return nil, response.NewValidationErrorResponse("task_args", fmt.Sprintf("invalid task_args: %v", err))
	}

	timeout, err := resolveTaskTimeout(taskType, in.Timeout, appConfig)
	if err != nil {
		return nil, err
	}

	tasks := make([]*models.InferenceTask, 0)
	for i := 0; i < repeatNum; i++ {
		taskIDBytes := make([]byte, 32)
		rand.Read(taskIDBytes)
		taskID := hexutil.Encode(taskIDBytes)
		nonce, taskIDCommitment := models.GenerateTaskIDCommitment(taskID)

		task := &models.InferenceTask{
			Client:           *client,
			ClientTask:       *clientTask,
			TaskArgs:         in.TaskArgs,
			TaskType:         taskType,
			TaskModelIDs:     modelIDs,
			TaskVersion:      taskVersion,
			TaskFee:          taskFee,
			MinVram:          minVram,
			RequiredGPU:      in.RequiredGPU,
			RequiredGPUVram:  in.RequiredGPUVram,
			TaskSize:         taskSize,
			TaskID:           taskID,
			TaskIDCommitment: taskIDCommitment,
			Nonce:            nonce,
			Timeout:          timeout,
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func isTaskArgsJSONError(err error) bool {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return true
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return true
	}

	return false
}

func DoCreateTask(ctx context.Context, clientID string, in *TaskInput) (*TaskResponse, error) {
	appConfig := config.GetConfig()
	db := config.GetDB()
	if err := validateTaskInput(in, appConfig); err != nil {
		return nil, err
	}

	client, err := tools.CreateClientIfNotExist(ctx, db, clientID)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}

	clientTask, err := createTaskRecords(ctx, db, client, in, appConfig)
	if err != nil {
		if _, ok := err.(response.ErrorResponseMessage); ok {
			return nil, err
		}
		if _, ok := err.(response.ExceptionResponseMessage); ok {
			return nil, err
		}
		return nil, response.NewExceptionResponse(err)
	}
	return &TaskResponse{Data: clientTask}, nil
}

func createTaskRecords(
	ctx context.Context,
	db *gorm.DB,
	client *models.Client,
	in *TaskInput,
	appConfig *config.AppConfig,
) (*models.ClientTask, error) {
	return saveTaskRecords(ctx, db, client, func(clientTask *models.ClientTask) ([]*models.InferenceTask, error) {
		return buildTasks(in, client, clientTask, appConfig)
	})
}

func saveTaskRecords(
	ctx context.Context,
	db *gorm.DB,
	client *models.Client,
	build func(*models.ClientTask) ([]*models.InferenceTask, error),
) (*models.ClientTask, error) {
	clientTask, err := tools.CreateClientTask(ctx, db, client)
	if err != nil {
		return nil, err
	}
	tasks, err := build(clientTask)
	if err != nil {
		return nil, err
	}
	if err := models.SaveTasks(ctx, db, tasks); err != nil {
		return nil, err
	}
	return attachInferenceTasks(clientTask, tasks), nil
}

func attachInferenceTasks(clientTask *models.ClientTask, tasks []*models.InferenceTask) *models.ClientTask {
	clientTask.InferenceTasks = make([]models.InferenceTask, len(tasks))
	for i, t := range tasks {
		clientTask.InferenceTasks[i] = *t
	}
	return clientTask
}

func validateRawTaskInput(in *TaskInput) error {
	if in.TaskFee == nil {
		return response.NewValidationErrorResponse("task_fee", "task_fee is required")
	}
	if _, err := config.ValidateWei(*in.TaskFee); err != nil {
		return response.NewValidationErrorResponse("task_fee", err.Error())
	}
	return validateTaskInput(in, config.GetConfig())
}

func validateTaskInput(in *TaskInput, appConfig *config.AppConfig) error {
	if in.TaskType == nil || *in.TaskType > models.TaskTypeSDFTLora {
		return response.NewValidationErrorResponse("task_type", "task_type must be 0, 1, or 2")
	}
	repeatNum := appConfig.Task.RepeatNum
	if in.RepeatNum != nil {
		repeatNum = *in.RepeatNum
	}
	if repeatNum <= 0 {
		return response.NewValidationErrorResponse("repeat_num", "repeat_num must be greater than 0")
	}
	if result, err := models.ValidateTaskArgsJsonStr(in.TaskArgs, *in.TaskType); err != nil {
		if isTaskArgsJSONError(err) {
			return response.NewValidationErrorResponse("task_args", fmt.Sprintf("task_args must be valid JSON: %v", err))
		}
		return response.NewExceptionResponse(err)
	} else if result != nil {
		return response.NewValidationErrorResponse("task_args", fmt.Sprintf("invalid task_args: %s", result.Error()))
	}
	return nil
}

func createRawTaskAndConsumeQuota(
	ctx context.Context,
	db *gorm.DB,
	apiKey *models.ClientAPIKey,
	in *TaskInput,
) (*TaskResponse, error) {
	var clientTask *models.ClientTask
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := apiKey.UseWithinLimit(ctx, tx); err != nil {
			return err
		}
		client, err := tools.CreateClientIfNotExist(ctx, tx, apiKey.ClientID)
		if err != nil {
			return err
		}
		clientTask, err = createTaskRecords(ctx, tx, client, in, config.GetConfig())
		return err
	})
	if err != nil {
		if errors.Is(err, models.ErrAPIKeyQuotaExceeded) {
			return nil, response.NewValidationErrorResponse("Authorization", "API key quota exceeded")
		}
		if _, ok := err.(response.ErrorResponseMessage); ok {
			return nil, err
		}
		if _, ok := err.(response.ExceptionResponseMessage); ok {
			return nil, err
		}
		return nil, response.NewExceptionResponse(err)
	}
	return &TaskResponse{Data: clientTask}, nil
}

func CreateTask(c *gin.Context, in *CreateTaskInput) (*TaskResponse, error) {
	ctx := c.Request.Context()
	requestStart := time.Now()
	db := config.GetDB()

	apiKey, err := tools.ValidateAuthorization(ctx, db, in.Authorization)
	if err != nil {
		return nil, err
	}
	if err := validateRawTaskInput(&in.TaskInput); err != nil {
		return nil, err
	}

	allowed, waitTime, err := ratelimit.APIRateLimiter.CheckRateLimit(ctx, apiKey.ClientID, apiKey.RateLimit, time.Minute)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}
	if !allowed {
		return nil, response.NewValidationErrorResponse("rate_limit", fmt.Sprintf("rate limit exceeded, please wait %.2f seconds", waitTime))
	}

	taskResponse, err := createRawTaskAndConsumeQuota(ctx, db, apiKey, &in.TaskInput)
	if err != nil {
		return nil, err
	}
	tasks := taskResponse.Data.InferenceTasks
	primaryTaskIDCommitment := tasktrace.StartTrace(tasktrace.StartTraceInput{
		Source:      tasktrace.SourceDirectInferenceTask,
		Endpoint:    "/v1/inference_tasks",
		ClientID:    apiKey.ClientID,
		Model:       traceModelIDs(tasks),
		TaskType:    in.TaskType,
		Request:     in.TaskInput,
		RequestTime: requestStart,
		Tasks:       tasks,
	}, config.GetConfig().Admin.TaskTraceMaxTasks)
	tasktrace.FinishTrace(primaryTaskIDCommitment, taskResponse, nil, nil)
	return taskResponse, nil
}

func traceModelIDs(tasks []models.InferenceTask) string {
	if len(tasks) == 0 {
		return ""
	}
	return strings.Join(tasks[0].TaskModelIDs, ",")
}
