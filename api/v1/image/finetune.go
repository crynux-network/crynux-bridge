package image

import (
	"crynux_bridge/api/ratelimit"
	"crynux_bridge/api/v1/inference_tasks"
	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type SDFinetuneLoraRequest struct {
	SDFinetuneLoraTaskParams
	Authorization string  `header:"Authorization" validate:"required" description:"API key"`
	Timeout       *uint64 `json:"timeout,omitempty" description:"Task timeout" validate:"omitempty"`
}

type SDFinetuneLoraTaskResponse struct {
	response.Response
	Data *models.ClientTask `json:"data"`
}

func CreateSDFinetuneLoraTask(c *gin.Context, in *SDFinetuneLoraRequest) (*SDFinetuneLoraTaskResponse, error) {
	ctx := c.Request.Context()
	db := config.GetDB()

	// validate request (apiKey)
	apiKey, err := tools.ValidateAuthorization(ctx, db, in.Authorization)
	if err != nil {
		return nil, err
	}

	allowed, waitTime, err := ratelimit.APIRateLimiter.CheckRateLimit(ctx, apiKey.ClientID, apiKey.RateLimit, time.Minute)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}
	if !allowed {
		return nil, response.NewValidationErrorResponse("rate_limit", fmt.Sprintf("rate limit exceeded, please wait %.2f seconds", waitTime))
	}

	in.SetDefaultValues()

	taskArgs := &models.FinetuneLoraTaskArgs{
		Model: models.ModelArgs{
			Name:     in.ModelName,
			Variant:  in.ModelVariant,
			Revision: in.ModelRevision,
		},
		Dataset: models.DatasetArgs{
			Url:           in.DatasetUrl,
			Name:          in.DatasetName,
			ConfigName:    in.DatasetConfigName,
			ImageColumn:   in.DatasetImageColumn,
			CaptionColumn: in.DatasetCaptionColumn,
		},
		Validation: models.ValidationArgs{
			Prompt:    in.ValidationPrompt,
			NumImages: in.ValidationNumImages,
		},
		TrainArgs: models.TrainArgs{
			LearningRate:              in.LearningRate,
			BatchSize:                 in.BatchSize,
			GradientAccumulationSteps: in.GradientAccumulationSteps,
			PredictionType:            in.PredictionType,
			MaxGradNorm:               in.MaxGradNorm,
			NumTrainEpochs:            in.NumTrainEpochs,
			NumTrainSteps:             in.NumTrainSteps,
			MaxTrainEpochs:            in.MaxTrainEpochs,
			MaxTrainSteps:             in.MaxTrainSteps,
			ScaleLR:                   in.ScaleLR,
			Resolution:                in.Resolution,
			NoiseOffset:               in.NoiseOffset,
			SNRGamma:                  in.SNRGamma,
			LRScheduler: models.LRSchedulerArgs{
				LRScheduler:   in.LRScheduler,
				LRWarmupSteps: in.LRWarmupSteps,
			},
			AdamArgs: models.AdamOptimizerArgs{
				Beta1:       in.AdamBeta1,
				Beta2:       in.AdamBeta2,
				WeightDecay: in.AdamWeightDecay,
				Epsilon:     in.AdamEpsilon,
			},
		},
		Lora: models.LoraArgs{
			Rank:            in.Rank,
			InitLoraWeights: in.InitLoraWeights,
			TargetModules:   in.TargetModules,
		},
		Transforms: models.TransformArgs{
			CenterCrop: in.CenterCrop,
			RandomFlip: in.RandomFlip,
		},
		MixedPrecision: in.MixedPrecision,
		Seed:           in.Seed,
	}

	if c.ContentType() == "multipart/form-data" {
		form, err := c.MultipartForm()
		if err != nil {
			return nil, response.NewExceptionResponse(err)
		}

		if files, ok := form.File["checkpoint"]; ok {
			if len(files) != 1 {
				return nil, response.NewValidationErrorResponse("checkpoint", "More than one checkpoint file uploaded")
			}

			checkpoint := files[0]
			appConfig := config.GetConfig()

			uuid := uuid.New().String()
			checkpointFilename := filepath.Join(appConfig.DataDir.InferenceTasks, fmt.Sprintf("%s_checkpoint.zip", uuid))
			if err = c.SaveUploadedFile(checkpoint, checkpointFilename); err != nil {
				return nil, response.NewExceptionResponse(err)
			}
			taskArgs.Checkpoint = checkpointFilename
		}
	}

	taskArgsStr, err := json.Marshal(taskArgs)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}
	log.Infof("finetune taskArgs: %s", string(taskArgsStr))

	taskType := models.TaskTypeSDFTLora
	minVram := uint64(24)
	repeatNum := 1
	task := &inference_tasks.TaskInput{
		ClientID:  apiKey.ClientID,
		TaskArgs:  string(taskArgsStr),
		TaskType:  &taskType,
		MinVram:   &minVram,
		RepeatNum: &repeatNum,
		Timeout:   in.Timeout,
	}

	taskResponse, err := inference_tasks.DoCreateTask(ctx, task)
	if err != nil {
		return nil, err
	}

	return &SDFinetuneLoraTaskResponse{
		Data: taskResponse.Data,
	}, nil
}

type GetSDFinetuneLoraTaskRequest struct {
	ID uint `path:"id" json:"id" description:"Task id" validate:"required"`
}

type GetSDFinetuneLoraTaskResult struct {
	ID     uint                    `json:"id"`
	Status models.ClientTaskStatus `json:"status"`
}

type GetSDFinetuneLoraTaskResponse struct {
	response.Response
	Data *GetSDFinetuneLoraTaskResult `json:"data"`
}

func GetSDFinetuneLoraTaskStatus(c *gin.Context, in *GetSDFinetuneLoraTaskRequest) (*GetSDFinetuneLoraTaskResponse, error) {
	ctx := c.Request.Context()
	db := config.GetDB()

	clientTask, err := models.GetClientTaskByID(ctx, db, in.ID)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}

	return &GetSDFinetuneLoraTaskResponse{
		Data: &GetSDFinetuneLoraTaskResult{
			ID:     clientTask.ID,
			Status: clientTask.Status,
		},
	}, nil
}

type DownloadSDFinetuneLoraTaskRequest struct {
	ID            uint   `path:"id" json:"id" description:"Task id" validate:"required"`
	Authorization string `header:"Authorization" validate:"required" description:"API key"`
}

func DownloadSDFinetuneLoraTaskResult(c *gin.Context, in *DownloadSDFinetuneLoraTaskRequest) error {
	ctx := c.Request.Context()
	db := config.GetDB()

	// validate request (apiKey)
	apiKey, err := tools.ValidateAuthorization(ctx, db, in.Authorization)
	if err != nil {
		return err
	}
	client, err := tools.GetClient(ctx, db, apiKey.ClientID)
	if err != nil {
		return response.NewExceptionResponse(err)
	}

	task, err := models.GetSDFTTaskFinalTask(ctx, db, in.ID)
	if err != nil {
		return response.NewExceptionResponse(err)
	}
	if task == nil {
		return response.NewValidationErrorResponse("id", "Task not found")
	}

	if client.ID != task.ClientID {
		return response.NewValidationErrorResponse("api_key", "invalid api key")
	}

	appConfig := config.GetConfig()

	taskResultFilePath := filepath.Join(appConfig.DataDir.InferenceTasks, task.TaskIDCommitment, "result.zip")
	if _, err := os.Stat(taskResultFilePath); os.IsNotExist(err) {
		return response.NewValidationErrorResponse("id", "Task result not found")
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", "attachment; filename=result.zip")
	c.Header("Content-Type", "application/zip")
	c.File(taskResultFilePath)

	return nil
}
