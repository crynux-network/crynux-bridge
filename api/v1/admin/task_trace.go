package admin

import (
	"crynux_bridge/api/v1/response"
	"crynux_bridge/tasktrace"

	"github.com/gin-gonic/gin"
)

type ListTaskTracesRequest struct {
	Limit int `query:"limit" json:"limit" validate:"omitempty"`
}

type GetTaskTraceRequest struct {
	TaskID string `path:"task_id" json:"task_id" validate:"required"`
}

type ListTaskTracesResponse struct {
	response.Response
	Data []tasktrace.Trace `json:"data"`
}

type GetTaskTraceResponse struct {
	response.Response
	Data *tasktrace.Trace `json:"data"`
}

func ListTaskTraces(c *gin.Context, in *ListTaskTracesRequest) (*ListTaskTracesResponse, error) {
	return &ListTaskTracesResponse{
		Data: tasktrace.ListTraces(in.Limit, false),
	}, nil
}

func ListOpenAILLMTaskTraces(c *gin.Context, in *ListTaskTracesRequest) (*ListTaskTracesResponse, error) {
	return &ListTaskTracesResponse{
		Data: tasktrace.ListTraces(in.Limit, true),
	}, nil
}

func GetTaskTrace(c *gin.Context, in *GetTaskTraceRequest) (*GetTaskTraceResponse, error) {
	trace, ok := tasktrace.GetTrace(in.TaskID)
	if !ok {
		return nil, response.NewValidationErrorResponse("task_id", "task trace not found")
	}
	return &GetTaskTraceResponse{
		Data: &trace,
	}, nil
}

func GetOpenAILLMTaskTrace(c *gin.Context, in *GetTaskTraceRequest) (*GetTaskTraceResponse, error) {
	trace, ok := tasktrace.GetTrace(in.TaskID)
	if !ok {
		return nil, response.NewValidationErrorResponse("task_id", "task trace not found")
	}
	if trace.Source != tasktrace.SourceOpenAIChatCompletions && trace.Source != tasktrace.SourceOpenAICompletions {
		return nil, response.NewValidationErrorResponse("task_id", "OpenAI LLM task trace not found")
	}
	return &GetTaskTraceResponse{
		Data: &trace,
	}, nil
}
