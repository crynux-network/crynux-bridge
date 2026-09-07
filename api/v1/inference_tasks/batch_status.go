package inference_tasks

import (
	"errors"
	"fmt"

	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/taskengine"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type BatchGetTaskStatusInput struct {
	ClientTaskIDs []uint `json:"client_task_ids" validate:"required"`
	Authorization string `header:"Authorization" validate:"required" description:"API key"`
}

type BatchGetTaskStatusItemResult struct {
	ClientTaskID uint               `json:"client_task_id"`
	ClientTask   *models.ClientTask `json:"client_task,omitempty"`
	Error        string             `json:"error,omitempty"`
}

type BatchGetTaskStatusResponse struct {
	response.Response
	Data []BatchGetTaskStatusItemResult `json:"data"`
}

func BatchGetTaskStatus(c *gin.Context, in *BatchGetTaskStatusInput) (*BatchGetTaskStatusResponse, error) {
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

	dedupedIDs := dedupeUintIDs(in.ClientTaskIDs)
	resultsByID := make(map[uint]BatchGetTaskStatusItemResult, len(dedupedIDs))

	engine, engineErr := taskengine.Default()
	if engineErr != nil {
		client, clientErr := tools.GetClient(ctx, db, apiKey.ClientID)
		if clientErr != nil {
			return nil, response.NewExceptionResponse(clientErr)
		}
		for _, clientTaskID := range dedupedIDs {
			clientTask, loadErr := tools.GetClientTask(ctx, db, client.ID, clientTaskID)
			if loadErr != nil {
				if errors.Is(loadErr, gorm.ErrRecordNotFound) {
					resultsByID[clientTaskID] = BatchGetTaskStatusItemResult{
						ClientTaskID: clientTaskID,
						Error:        "Client task not found",
					}
					continue
				}
				return nil, response.NewExceptionResponse(loadErr)
			}
			copied := *clientTask
			resultsByID[clientTaskID] = BatchGetTaskStatusItemResult{
				ClientTaskID: clientTaskID,
				ClientTask:   &copied,
			}
		}
	} else {
		clientTasks, statusErr := engine.StatusBatch(ctx, apiKey.ClientID, dedupedIDs)
		if statusErr != nil {
			return nil, response.NewExceptionResponse(statusErr)
		}
		found := make(map[uint]struct{}, len(clientTasks))
		for i := range clientTasks {
			task := clientTasks[i]
			found[task.ID] = struct{}{}
			copied := task
			resultsByID[task.ID] = BatchGetTaskStatusItemResult{
				ClientTaskID: task.ID,
				ClientTask:   &copied,
			}
		}
		for _, clientTaskID := range dedupedIDs {
			if _, ok := found[clientTaskID]; ok {
				continue
			}
			resultsByID[clientTaskID] = BatchGetTaskStatusItemResult{
				ClientTaskID: clientTaskID,
				Error:        "Client task not found",
			}
		}
	}

	results := make([]BatchGetTaskStatusItemResult, 0, len(in.ClientTaskIDs))
	for _, clientTaskID := range in.ClientTaskIDs {
		results = append(results, resultsByID[clientTaskID])
	}
	return &BatchGetTaskStatusResponse{Data: results}, nil
}

func dedupeUintIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
