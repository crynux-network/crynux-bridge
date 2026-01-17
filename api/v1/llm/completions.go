package llm

import (
	"crynux_bridge/api/ratelimit"
	"crynux_bridge/api/v1/inference_tasks"
	"crynux_bridge/api/v1/llm/structs"
	"crynux_bridge/api/v1/llm/utils"
	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type CompletionsRequest struct {
	structs.CompletionsRequest
	Authorization string `header:"Authorization" validate:"required" description:"API key"`
	Timeout       *uint64 `json:"timeout,omitempty" description:"Task timeout" validate:"omitempty"`
}

// build TaskInput from CompletionsRequest, create task, wait for task to finish, get task result, then return CompletionsResponse
func Completions(c *gin.Context, in *CompletionsRequest) (*structs.CompletionsResponse, error) {
	ctx := c.Request.Context()
	db := config.GetDB()

	/* 1. Build TaskInput from CompletionsRequest */
	in.SetDefaultValues() // set default values for some fields

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

	messages := make([]models.Message, 1)
	messages[0] = models.Message{
		Role:    models.LLMRoleUser,
		Content: in.Prompt,
	}

	generationConfig := &models.GPTGenerationConfig{
		DoSample:           true,
		Temperature:        in.Temperature,
		NumReturnSequences: in.N,
	}
	if in.MaxTokens != nil {
		generationConfig.MaxNewTokens = *in.MaxTokens
	}
	if in.TopP != nil {
		generationConfig.TopP = *in.TopP
	}
	if in.TopK != nil {
		generationConfig.TopK = *in.TopK
	}
	if in.MinP != nil {
		generationConfig.MinP = *in.MinP
	}
	if in.RepetitionPenalty != nil {
		generationConfig.RepetitionPenalty = *in.RepetitionPenalty
	}
	if len(in.Stop) > 0 {
		generationConfig.StopStrings = in.Stop
	}

	var dtype models.DType = models.DTypeAuto
	if strings.HasPrefix(in.Model, "Qwen/Qwen2.5") {
		dtype = models.DTypeBFloat16
	}

	taskArgs := models.GPTTaskArgs{
		Model:            in.Model,
		Messages:         messages,
		GenerationConfig: generationConfig,
		Seed:             in.Seed,
		DType:            dtype,
		// Tools:            in.Tools,
		// QuantizeBits:     structs.QuantizeBits8,
	}
	taskArgsStr, err := json.Marshal(taskArgs)
	if err != nil {
		err := errors.New("failed to marshal taskArgs")
		return nil, response.NewExceptionResponse(err)
	}

	taskType := models.TaskTypeLLM
	minVram := uint64(24)
	if in.MinVram != nil {
		minVram = *in.MinVram
	}
	taskFee := uint64(6000000000)

	task := &inference_tasks.TaskInput{
		ClientID:        apiKey.ClientID,
		TaskArgs:        string(taskArgsStr),
		TaskType:        &taskType,
		TaskVersion:     nil,
		MinVram:         &minVram,
		RequiredGPU:     "",
		RequiredGPUVram: 0,
		RepeatNum:       nil,
		TaskFee:         &taskFee,
		Timeout:         in.Timeout,
	}

	/* 2. Create task, wait until task finish and get task result. Implemented by function ProcessGPTTask */
	gptTaskResponse, resultDownloadedTask, err := inference_tasks.ProcessGPTTask(ctx, db, task)
	if err != nil {
		return nil, err
	}

	/* 3. Wrap GPTTaskResponse into CompletionsResponse and return */
	choices := make([]structs.CResChoice, len(gptTaskResponse.Choices))
	for i, c := range gptTaskResponse.Choices {
		choice, err := utils.ResponseChoiceToCResChoice(c)
		if err != nil {
			return nil, response.NewExceptionResponse(err)
		}
		choices[i] = choice
	}
	ccResponse := &structs.CompletionsResponse{
		Id:      resultDownloadedTask.TaskIDCommitment,
		Created: resultDownloadedTask.CreatedAt.Unix(),
		Model:   gptTaskResponse.Model,
		Choices: choices,
		Usage:   utils.UsageToCResUsage(gptTaskResponse.Usage),
		// Object:  "text",
		// SystemFingerprint: resultDownloadedTask.SystemFingerprint,
	}

	if err := apiKey.Use(ctx, db); err != nil {
		return nil, response.NewExceptionResponse(err)
	}

	return ccResponse, nil
}
