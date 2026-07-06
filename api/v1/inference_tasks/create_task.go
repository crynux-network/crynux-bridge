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
	ClientID        string                `json:"client_id" description:"Client id" validate:"required"`
	TaskArgs        string                `json:"task_args" description:"Task args" validate:"required"`
	TaskType        *models.ChainTaskType `json:"task_type" description:"Task type. 0 - SD task, 1 - LLM task, 2 - SD Finetune task" validate:"required"`
	TaskVersion     *string               `json:"task_version,omitempty" description:"Task version. Default is task.default_task_version" validate:"omitempty"`
	MinVram         *uint64               `json:"min_vram,omitempty" description:"Task minimal vram requirement" validate:"omitempty"`
	RequiredGPU     string                `json:"required_gpu,omitempty" description:"Task required GPU name" validate:"omitempty"`
	RequiredGPUVram uint64                `json:"required_gpu_vram,omitempty" description:"Task required GPU Vram" validate:"omitempty"`
	RepeatNum       *int                  `json:"repeat_num,omitempty" description:"Task repeat number" validate:"omitempty"`
	TaskFee         *uint64               `json:"task_fee,omitempty" description:"Task fee" validate:"omitempty"`
	Timeout         *uint64               `json:"timeout,omitempty" description:"Task timeout" validate:"omitempty"`
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

func getDefaultTaskFeeGWei(taskType models.ChainTaskType, taskArgs string, appConfig *config.AppConfig) (uint64, error) {
	var feeCNX float64
	switch taskType {
	case models.TaskTypeSD:
		baseModel, err := models.GetSDTaskConfigBaseModel(taskArgs)
		if err != nil {
			return 0, err
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
		return 0, fmt.Errorf("unsupported task type %d", taskType)
	}
	return config.CNXToGWei(feeCNX)
}

func getTaskFee(baseTaskFee, cap uint64) uint64 {
	return baseTaskFee * cap
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
	var baseTaskFee uint64
	if in.TaskFee != nil {
		baseTaskFee = *in.TaskFee
	} else {
		baseTaskFee, err = getDefaultTaskFeeGWei(taskType, in.TaskArgs, appConfig)
		if err != nil {
			return nil, response.NewExceptionResponse(err)
		}
	}
	taskFee := getTaskFee(baseTaskFee, taskSize)

	repeatNum := appConfig.Task.RepeatNum
	if in.RepeatNum != nil {
		repeatNum = *in.RepeatNum
	}

	modelIDs, err := models.GetTaskConfigModelIDs(in.TaskArgs, taskType)
	if err != nil {
		return nil, response.NewValidationErrorResponse("task_args", fmt.Sprintf("invalid task_args: %v", err))
	}

	var timeout uint64
	if in.Timeout != nil {
		timeout = *in.Timeout
	} else if taskType == models.TaskTypeSDFTLora {
		timeout = appConfig.Task.SDFinetuneTimeout * 60
	} else {
		timeout = appConfig.Task.DefaultTimeout * 60
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

func DoCreateTask(ctx context.Context, in *TaskInput) (*TaskResponse, error) {
	appConfig := config.GetConfig()
	db := config.GetDB()

	// get Client
	client, err := tools.GetClient(ctx, db, in.ClientID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, response.NewExceptionResponse(err)
		}
	}

	// create ClientTask for client
	clientTask, err := tools.CreateClientTask(ctx, db, client)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}

	// build interface tasks
	tasks, err := buildTasks(in, client, clientTask, appConfig)
	if err != nil {
		return nil, err
	}

	// save tasks to local db
	err = models.SaveTasks(ctx, config.GetDB(), tasks)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}
	clientTask.InferenceTasks = make([]models.InferenceTask, len(tasks))
	for i, t := range tasks {
		clientTask.InferenceTasks[i] = *t
	}

	return &TaskResponse{Data: clientTask}, nil
}

func CreateTask(c *gin.Context, in *TaskInput) (*TaskResponse, error) {
	ctx := c.Request.Context()
	requestStart := time.Now()

	// check rate limit
	allowed, waitTime, err := ratelimit.APIRateLimiter.CheckRateLimit(ctx, in.ClientID, 20, time.Minute)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}
	if !allowed {
		return nil, response.NewValidationErrorResponse("rate_limit", fmt.Sprintf("rate limit exceeded, please wait %.2f seconds", waitTime))
	}

	taskResponse, err := DoCreateTask(ctx, in)
	if err != nil {
		return nil, err
	}
	tasks := taskResponse.Data.InferenceTasks
	primaryTaskIDCommitment := tasktrace.StartTrace(tasktrace.StartTraceInput{
		Source:      tasktrace.SourceDirectInferenceTask,
		Endpoint:    "/v1/inference_tasks",
		ClientID:    in.ClientID,
		Model:       traceModelIDs(tasks),
		TaskType:    in.TaskType,
		Request:     in,
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
