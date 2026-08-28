package inference_tasks

import (
	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/taskengine"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

type GetAuthenticatedTaskImageInput struct {
	ClientTaskID  uint    `path:"client_task_id" description:"Client task id" validate:"required"`
	Index         *uint64 `path:"index" description:"Result index" validate:"required"`
	Authorization string  `header:"Authorization" validate:"required" description:"API key"`
}

type GetLLMResultInput struct {
	ClientTaskID  uint   `path:"client_task_id" description:"Client task id" validate:"required"`
	Authorization string `header:"Authorization" validate:"required" description:"API key"`
}

func GetAuthenticatedTaskImage(c *gin.Context, in *GetAuthenticatedTaskImageInput) error {
	return getTaskResult(c, in.ClientTaskID, in.Authorization, *in.Index, models.TaskTypeSD)
}

func GetLLMResult(c *gin.Context, in *GetLLMResultInput) error {
	return getTaskResult(c, in.ClientTaskID, in.Authorization, 0, models.TaskTypeLLM)
}

func getTaskResult(c *gin.Context, clientTaskID uint, authorization string, index uint64, expectedTaskType models.ChainTaskType) error {
	ctx := c.Request.Context()
	db := config.GetDB()

	_, err := loadRawClientTask(ctx, db, authorization, clientTaskID)
	if err != nil {
		return err
	}
	engine, err := taskengine.Default()
	if err != nil {
		return response.NewExceptionResponse(err)
	}
	apiKey, err := tools.ValidateReadAuthorization(ctx, db, authorization)
	if err != nil {
		return err
	}
	result, err := engine.Result(ctx, apiKey.ClientID, clientTaskID)
	if errors.Is(err, taskengine.ErrTaskRunning) || errors.Is(err, taskengine.ErrTaskFailed) {
		return response.NewValidationErrorResponse("client_task_id", "Client task was not successful")
	}
	if err != nil {
		return response.NewExceptionResponse(err)
	}
	if result.TaskType != expectedTaskType {
		return response.NewValidationErrorResponse("client_task_id", "Task result type does not match the endpoint")
	}
	task := &models.InferenceTask{
		TaskIDCommitment: result.TaskIDCommitment,
		TaskType:         result.TaskType,
		TaskSize:         result.TaskSize,
	}
	return serveAuthenticatedTaskResultFile(c, task, index)
}

func serveAuthenticatedTaskResultFile(c *gin.Context, task *models.InferenceTask, index uint64) error {
	extension := "png"
	contentType := "image/png"
	if task.TaskType == models.TaskTypeLLM {
		extension = "json"
		contentType = "application/json"
	}
	filename := fmt.Sprintf("%d.%s", index, extension)
	resultFile := filepath.Join(
		config.GetConfig().DataDir.InferenceTasks,
		task.TaskIDCommitment,
		filename,
	)

	if _, err := os.Stat(resultFile); err != nil {
		return classifyResultFileError(task.TaskType, err)
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", contentType)
	c.File(resultFile)
	return nil
}

func classifyResultFileError(taskType models.ChainTaskType, err error) error {
	if os.IsNotExist(err) {
		field := "index"
		if taskType == models.TaskTypeLLM {
			field = "client_task_id"
		}
		return response.NewValidationErrorResponse(field, "Result not found")
	}
	return response.NewExceptionResponse(err)
}
