package inference_tasks

import (
	"errors"

	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/taskengine"

	"github.com/gin-gonic/gin"
)

type AbortTaskInput struct {
	ClientTaskID  uint   `path:"client_task_id" json:"client_task_id" description:"Client task id" validate:"required"`
	Authorization string `header:"Authorization" validate:"required" description:"API key"`
}

type AbortTaskResponse struct {
	response.Response
	Data *models.ClientTask `json:"data"`
}

func AbortTask(c *gin.Context, in *AbortTaskInput) (*AbortTaskResponse, error) {
	ctx := c.Request.Context()
	db := config.GetDB()

	apiKey, err := tools.ValidateReadAuthorization(ctx, db, in.Authorization)
	if err != nil {
		return nil, err
	}
	engine, err := taskengine.Default()
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}
	clientTask, err := engine.AbortClientTask(ctx, apiKey.ClientID, in.ClientTaskID)
	if err != nil {
		return nil, mapAbortError(err)
	}
	return &AbortTaskResponse{Data: clientTask}, nil
}

func mapAbortError(err error) error {
	switch {
	case errors.Is(err, taskengine.ErrTaskNotFound):
		return response.NewValidationErrorResponse("client_task_id", "Client task not found")
	case errors.Is(err, taskengine.ErrAbortNotCancellable):
		return response.NewStatusErrorResponse(409, "not_cancellable")
	case errors.Is(err, taskengine.ErrAbortTemporary):
		return response.NewStatusErrorResponse(503, "temporary_error")
	default:
		return response.NewExceptionResponse(err)
	}
}
