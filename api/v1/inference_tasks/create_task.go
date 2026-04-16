package inference_tasks

import (
	"context"
	"crynux_bridge/api/ratelimit"
	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crypto/rand"
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
	TaskVersion     *string               `json:"task_version,omitempty" description:"Task version. Default is 2.5.0" validate:"omitempty"`
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
		case "crynux-ai/stable-diffusion-v1-5", "crynux-network/stable-diffusion-v1-5":
			return 8, nil
		case "crynux-ai/sdxl-turbo", "crynux-network/sdxl-turbo", "crynux-ai/stable-diffusion-xl-base-1.0":
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

func getTaskFee(taskType models.ChainTaskType, baseTaskFee, cap uint64) uint64 {
	if taskType == models.TaskTypeSD {
		return baseTaskFee * cap
	} else {
		return baseTaskFee * cap
	}
}

func buildTasks(in *TaskInput, client *models.Client, clientTask *models.ClientTask, appConfig *config.AppConfig) ([]*models.InferenceTask, error) {
	taskType := *in.TaskType

	var taskVersion = appConfig.Task.TaskVersions[0]
	if in.TaskVersion != nil {
		taskVersion = *in.TaskVersion
	}

	if taskType == models.TaskTypeSD || taskType == models.TaskTypeSDFTLora {
		if taskVersion == "2.5.0" {
			in.TaskArgs = strings.ReplaceAll(in.TaskArgs, "crynux-network/", "crynux-ai/")
		} else {
			in.TaskArgs = strings.ReplaceAll(in.TaskArgs, "crynux-ai/", "crynux-network/")
		}
	}

	result, err := models.ValidateTaskArgsJsonStr(in.TaskArgs, taskType)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}

	if result != nil {
		return nil, response.NewValidationErrorResponse("task_args", result.Error())
	}

	if taskType == models.TaskTypeLLM {
		if err := models.ValidateGPTTaskArgsContentJSON(in.TaskArgs); err != nil {
			return nil, response.NewValidationErrorResponse("task_args", err.Error())
		}
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
		baseTaskFee = 5000000000
	}
	taskFee := getTaskFee(taskType, baseTaskFee, taskSize) // unit: GWei

	repeatNum := appConfig.Task.RepeatNum
	if in.RepeatNum != nil {
		repeatNum = *in.RepeatNum
	}

	modelIDs, err := models.GetTaskConfigModelIDs(in.TaskArgs, taskType)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
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

		task := &models.InferenceTask{
			Client:          *client,
			ClientTask:      *clientTask,
			TaskArgs:        in.TaskArgs,
			TaskType:        taskType,
			TaskModelIDs:    modelIDs,
			TaskVersion:     taskVersion,
			TaskFee:         taskFee,
			MinVram:         minVram,
			RequiredGPU:     in.RequiredGPU,
			RequiredGPUVram: in.RequiredGPUVram,
			TaskSize:        taskSize,
			TaskID:          taskID,
			Timeout:         timeout,
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
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

	// check rate limit
	allowed, waitTime, err := ratelimit.APIRateLimiter.CheckRateLimit(ctx, in.ClientID, 20, time.Minute)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}
	if !allowed {
		return nil, response.NewValidationErrorResponse("rate_limit", fmt.Sprintf("rate limit exceeded, please wait %.2f seconds", waitTime))
	}

	return DoCreateTask(ctx, in)
}
