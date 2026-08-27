package inference_tasks

import (
	"crynux_bridge/api/v1/response"
	"crynux_bridge/config"
	"crynux_bridge/models"

	"github.com/gin-gonic/gin"
)

type GetTaskInput struct {
	ClientTaskID  uint   `path:"client_task_id" json:"client_task_id" description:"Client task id" validate:"required"`
	Authorization string `header:"Authorization" validate:"required" description:"API key"`
}

type GetTaskResponse struct {
	response.Response
	Data *models.InferenceTask `json:"data"`
}

func GetTaskById(c *gin.Context, in *GetTaskInput) (*GetTaskResponse, error) {
	ctx := c.Request.Context()
	db := config.GetDB()

	clientTask, err := loadRawClientTask(ctx, db, in.Authorization, in.ClientTaskID)
	if err != nil {
		return nil, err
	}

	var task *models.InferenceTask
	if clientTask.InferenceTasks[0].TaskType == models.TaskTypeSDFTLora {
		task = selectSDFTRawInferenceTask(clientTask.InferenceTasks)
	} else {
		task = selectRawInferenceTask(clientTask.InferenceTasks)
	}

	return &GetTaskResponse{
		Data: task,
	}, nil
}
