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
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var toolCallRegex = regexp.MustCompile(`<tool_call>\s*({[\s\S]*?})\s*</tool_call>`)

type parsedLlmToolCallArgs struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"` // Keep arguments as raw JSON to convert back to string easily
}

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
	in.SetDefaultValues() // set default values for some fields
	logRequestPayload := map[string]any{
		"request":    in.ChatCompletionsRequest,
		"timeout":    in.Timeout,
		"vram_limit": in.VramLimit,
	}
	var logResponsePayload any
	defer func() {
		logOpenAICompatibleExchange("chat_completions", in.Authorization, logRequestPayload, logResponsePayload, err)
	}()

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

	messages := make([]models.Message, len(in.Messages))
	for i, m := range in.Messages {
		convertedMessage, err := utils.CCReqMessageToMessage(m)
		if err != nil {
			return nil, response.NewValidationErrorResponse("messages", fmt.Sprintf("messages[%d].content: %v", i, err))
		}
		messages[i] = convertedMessage
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
		Tools:            in.Tools,
		GenerationConfig: generationConfig,
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
		fmt.Println("choice.Message.Content", choiceMessageContent)

		matches := toolCallRegex.FindStringSubmatch(choiceMessageContent)
		if len(matches) > 1 {
			potentialJsonString := matches[1]
			var parsedArgs parsedLlmToolCallArgs
			if err := json.Unmarshal([]byte(potentialJsonString), &parsedArgs); err == nil {
				// Successfully parsed the LLM's tool call structure
				choice.Message.Content = "" // Clear content for tool calls
				choice.FinishReason = models.FinishReasonToolCalls

				var finalArgumentsString string
				var tempStr string
				// Attempt to unmarshal Arguments as a JSON string first.
				// This handles cases where arguments are double-encoded as a string.
				if err := json.Unmarshal(parsedArgs.Arguments, &tempStr); err == nil {
					// If successful, parsedArgs.Arguments was a JSON string,
					// and tempStr is its unescaped content.
					finalArgumentsString = tempStr
				} else {
					// If not a JSON string (e.g., it's a JSON object/array directly),
					// use its direct string representation.
					finalArgumentsString = string(parsedArgs.Arguments)
				}

				toolCallInstance := structs.ToolCall{
					Id:   fmt.Sprintf("call_%s_choice%d_tool0", resultDownloadedTask.TaskIDCommitment, i),
					Type: "function",
					Function: structs.FunctionCall{
						Name:      parsedArgs.Name,
						Arguments: finalArgumentsString,
					},
				}
				choice.Message.ToolCalls = []structs.ToolCall{toolCallInstance}
			} else {
				// JSON parsing failed, treat as regular text. Original content is already there.
				// Set a default finish reason if not already set by LLM response processing for non-tool-call cases
				if choice.FinishReason == "" {
					choice.FinishReason = models.FinishReasonStop
				}
			}
		} else {
			// No tool call tag found, ensure default finish reason
			if choice.FinishReason == "" {
				choice.FinishReason = models.FinishReasonStop
			}
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

	if err := apiKey.Use(ctx, db); err != nil {
		return nil, response.NewExceptionResponse(err)
	}

	return ccResponse, nil
}
