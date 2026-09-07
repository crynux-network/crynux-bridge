package inference_tasks

import (
	"crynux_bridge/api/ratelimit"
	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/tasktrace"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

const maxRawTaskBatchSize = 100

type BatchCreateTaskInput struct {
	Tasks         []TaskInput `json:"tasks" validate:"required"`
	Authorization string      `header:"Authorization" validate:"required" description:"API key"`
}

type BatchCreateTaskItemResult struct {
	Index      int                `json:"index"`
	ClientTask *models.ClientTask `json:"client_task,omitempty"`
	Error      string             `json:"error,omitempty"`
}

type BatchCreateTaskResponse struct {
	response.Response
	Data []BatchCreateTaskItemResult `json:"data"`
}

func BatchCreateTask(c *gin.Context, in *BatchCreateTaskInput) (*BatchCreateTaskResponse, error) {
	ctx := c.Request.Context()
	requestStart := time.Now()
	db := config.GetDB()

	if len(in.Tasks) == 0 {
		return nil, response.NewValidationErrorResponse("tasks", "tasks must not be empty")
	}
	if len(in.Tasks) > maxRawTaskBatchSize {
		return nil, response.NewValidationErrorResponse("tasks", fmt.Sprintf("tasks must contain at most %d items", maxRawTaskBatchSize))
	}

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

	results := make([]BatchCreateTaskItemResult, 0, len(in.Tasks))
	for i := range in.Tasks {
		item := &in.Tasks[i]
		if err := validateRawTaskInput(item); err != nil {
			results = append(results, BatchCreateTaskItemResult{
				Index: i,
				Error: validationErrorMessage(err),
			})
			continue
		}

		taskResponse, err := createRawTaskAndConsumeQuota(ctx, db, apiKey, item)
		if err != nil {
			results = append(results, BatchCreateTaskItemResult{
				Index: i,
				Error: validationErrorMessage(err),
			})
			continue
		}

		clientTask := taskResponse.Data
		traceKey := tasktrace.StartTrace(tasktrace.StartTraceInput{
			Source:       tasktrace.SourceDirectInferenceTask,
			Endpoint:     "/v1/inference_tasks/batch",
			ClientID:     apiKey.ClientID,
			Model:        traceClientTaskModel(clientTask),
			TaskType:     item.TaskType,
			Request:      item,
			RequestTime:  requestStart,
			ClientTaskID: clientTask.ID,
			Tasks:        clientTask.InferenceTasks,
		}, config.GetConfig().Admin.TaskTraceMaxTasks)
		tasktrace.FinishTrace(traceKey, taskResponse, nil, nil)

		results = append(results, BatchCreateTaskItemResult{
			Index:      i,
			ClientTask: clientTask,
		})
	}

	return &BatchCreateTaskResponse{Data: results}, nil
}

func validationErrorMessage(err error) string {
	if ve, ok := err.(*response.ValidationErrorResponse); ok {
		return fmt.Sprintf("%s: %s", ve.GetFieldName(), ve.GetFieldMessage())
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}
