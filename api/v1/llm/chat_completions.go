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

type ChatCompletionsRequest struct {
	structs.ChatCompletionsRequest
	Authorization string  `header:"Authorization" validate:"required" description:"API key"`
	Timeout       *uint64 `json:"timeout,omitempty" description:"Task timeout" validate:"omitempty"`
	VramLimit     *uint64 `path:"vram_limit" description:"Override minimum GPU VRAM in GB from URL path"`
}

// build TaskInput from ChatCompletionsRequest, create task, wait for task to finish, get task result, then return ChatCompletionsResponse
func ChatCompletions(c *gin.Context, in *ChatCompletionsRequest) (res *structs.ChatCompletionsResponse, err error) {
	ctx := c.Request.Context()
	db := config.GetDB()

	/* 1. Build TaskInput from ChatCompletionsRequest */
	requestStart := time.Now()
	in.SetDefaultValues() // set default values for some fields
	logRequestPayload := map[string]any{
		"request":    in.ChatCompletionsRequest,
		"timeout":    in.Timeout,
		"vram_limit": in.VramLimit,
	}
	var logResponsePayload any
	taskIDCommitment := ""
	defer func() {
		logOpenAICompatibleExchange("chat_completions", in.Authorization, taskIDCommitment, logRequestPayload, logResponsePayload, err, time.Since(requestStart).Seconds())
	}()
	toolCallRequestHasTools := len(in.Tools) > 0
	toolCallMatched := false
	var toolCallLogResponsePayload any
	defer func() {
		if toolCallRequestHasTools {
			logOpenAICompatibleToolCallExchange("chat_completions", in.Authorization, taskIDCommitment, logRequestPayload, toolCallLogResponsePayload, toolCallMatched, err, time.Since(requestStart).Seconds())
		}
	}()

	// validate request (apiKey)
	apiKey, err := tools.ValidateAuthorization(ctx, db, in.Authorization)
	if err != nil {
		return nil, mapLLMAuthorizationError(err)
	}

	allowed, waitTime, err := ratelimit.APIRateLimiter.CheckRateLimit(ctx, apiKey.ClientID, apiKey.RateLimit, time.Minute)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}
	if !allowed {
		return nil, newRateLimitExceededError(waitTime)
	}

	messages := make([]models.Message, len(in.Messages))
	for i, m := range in.Messages {
		convertedMessage, err := utils.CCReqMessageToMessage(m)
		if err != nil {
			return nil, response.NewValidationErrorResponse("messages", fmt.Sprintf("messages[%d].content: %v", i, err))
		}
		messages[i] = convertedMessage
	}

	generationConfig := &models.GPTGenerationConfig{
		DoSample:           false,
		Temperature:        0,
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
		Tools:            in.Tools,
		GenerationConfig: generationConfig,
		TemplateArgs:     buildLLMTemplateArgs(in.Model, in.Tools),
		Seed:             in.Seed,
		DType:            dtype,
		// QuantizeBits:     structs.QuantizeBits8,
	}
	taskArgsStr, err := json.Marshal(taskArgs)
	if err != nil {
		err := errors.New("failed to marshal taskArgs")
		return nil, response.NewExceptionResponse(err)
	}

	taskType := models.TaskTypeLLM
	minVram := resolveMinVram(in.MinVram, in.VramLimit)

	task := &inference_tasks.TaskInput{
		ClientID:        apiKey.ClientID,
		TaskArgs:        string(taskArgsStr),
		TaskType:        &taskType,
		TaskVersion:     nil,
		MinVram:         &minVram,
		RequiredGPU:     "",
		RequiredGPUVram: 0,
		RepeatNum:       nil,
		TaskFee:         nil,
		Timeout:         in.Timeout,
	}

	/* 2. Create task, wait until task finish and get task result. Implemented by function ProcessGPTTask */
	gptTaskResponse, resultDownloadedTask, err := inference_tasks.ProcessGPTTask(ctx, db, task)
	if err != nil {
		return nil, mapLLMTaskProcessingError(err)
	}
	taskIDCommitment = resultDownloadedTask.TaskIDCommitment
	logResponsePayload = map[string]any{
		"task": map[string]any{
			"task_id_commitment": resultDownloadedTask.TaskIDCommitment,
			"status":             resultDownloadedTask.Status,
			"task_type":          resultDownloadedTask.TaskType,
		},
		"result": gptTaskResponse,
	}

	/* 3. Wrap GPTTaskResponse into ChatCompletionsResponse and return */
	choices := make([]structs.CCResChoice, len(gptTaskResponse.Choices))
	for i, choice := range gptTaskResponse.Choices {

		choiceMessageContent := utils.MessageContentToString(choice.Message.Content)
		cleanContent, parsedToolCalls := normalizeAssistantContent(choiceMessageContent)
		choice.Message.Content = cleanContent

		if len(parsedToolCalls) > 0 {
			toolCallMatched = true
			choice.FinishReason = models.FinishReasonToolCalls
			choice.Message.ToolCalls = make([]structs.ToolCall, len(parsedToolCalls))
			for toolIdx, parsedToolCall := range parsedToolCalls {
				choice.Message.ToolCalls[toolIdx] = structs.ToolCall{
					Id:   fmt.Sprintf("call_%s_choice%d_tool%d", resultDownloadedTask.TaskIDCommitment, i, toolIdx),
					Type: "function",
					Function: structs.FunctionCall{
						Name:      parsedToolCall.Name,
						Arguments: parsedToolCall.Arguments,
					},
				}
			}
		} else if choice.FinishReason == "" {
			choice.FinishReason = models.FinishReasonStop
		}

		choices[i] = utils.ResponseChoiceToCCResChoice(choice)
	}
	ccResponse := &structs.ChatCompletionsResponse{
		Id:      resultDownloadedTask.TaskIDCommitment,
		Created: resultDownloadedTask.CreatedAt.Unix(),
		Model:   gptTaskResponse.Model,
		Choices: choices,
		Usage:   utils.UsageToCCResUsage(gptTaskResponse.Usage),
		// Object:  "text",
		// ServiceTier: "",
	}
	toolCallLogResponsePayload = ccResponse

	if err := apiKey.Use(ctx, db); err != nil {
		return nil, response.NewExceptionResponse(err)
	}

	if in.Stream {
		includeUsage := in.StreamOptions != nil && in.StreamOptions.IncludeUsage
		if err := streamChatCompletionsResponse(c, ccResponse, includeUsage); err != nil {
			return nil, response.NewExceptionResponse(err)
		}
		return nil, nil
	}

	return ccResponse, nil
}
