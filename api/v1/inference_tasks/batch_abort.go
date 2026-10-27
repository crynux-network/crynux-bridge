package inference_tasks

import (
	"fmt"

	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/taskengine"

	"github.com/gin-gonic/gin"
)

type BatchAbortTaskInput struct {
	ClientTaskIDs []uint `json:"client_task_ids" validate:"required"`
	Authorization string `header:"Authorization" validate:"required" description:"API key"`
}

type BatchAbortTaskItemResult struct {
	ClientTaskID uint               `json:"client_task_id"`
	ClientTask   *models.ClientTask `json:"client_task,omitempty"`
	Error        string             `json:"error,omitempty"`
}

type BatchAbortTaskResponse struct {
	response.Response
	Data []BatchAbortTaskItemResult `json:"data"`
}

func BatchAbortTask(c *gin.Context, in *BatchAbortTaskInput) (*BatchAbortTaskResponse, error) {
	ctx := c.Request.Context()
	db := config.GetDB()

	if len(in.ClientTaskIDs) == 0 {
		return nil, response.NewValidationErrorResponse("client_task_ids", "client_task_ids must not be empty")
	}
	if len(in.ClientTaskIDs) > maxRawTaskBatchSize {
		return nil, response.NewValidationErrorResponse(
			"client_task_ids",
			fmt.Sprintf("client_task_ids must contain at most %d items", maxRawTaskBatchSize),
		)
	}

	apiKey, err := tools.ValidateReadAuthorization(ctx, db, in.Authorization)
	if err != nil {
		return nil, err
	}
	engine, err := taskengine.Default()
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}

	engineResults, err := engine.AbortClientTaskBatch(ctx, apiKey.ClientID, in.ClientTaskIDs)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}
	results := make([]BatchAbortTaskItemResult, 0, len(engineResults))
	for _, item := range engineResults {
		results = append(results, BatchAbortTaskItemResult{
			ClientTaskID: item.ClientTaskID,
			ClientTask:   item.ClientTask,
			Error:        item.Error,
		})
	}
	return &BatchAbortTaskResponse{Data: results}, nil
}
