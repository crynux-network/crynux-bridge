package image

import (
	"bufio"
	"bytes"
	"crynux_bridge/api/ratelimit"
	"crynux_bridge/api/v1/inference_tasks"
	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/tasktrace"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type CreateImageRequest struct {
	Authorization     string  `header:"Authorization" validate:"required" description:"API key"`
	Prompt            string  `json:"prompt" validate:"required" description:"A text description of the desired image(s)"`
	Background        string  `json:"background,omitempty" description:"No use for now. For compatibility with Openai."`
	Model             string  `json:"model,omitempty" default:"crynux-network/sdxl-turbo" description:"The model to use for image generation. Default is 'crynux-network/sdxl-turbo'"`
	Moderation        string  `json:"moderation,omitempty" default:"auto" description:"No use for now. For compatibility with Openai."`
	N                 int     `json:"n,omitempty" default:"1" description:"The number of images to generate. Default is 1"`
	OutputCompression int     `json:"output_compression,omitempty" default:"100" description:"The compression level for the output image(s). Default is 100. No use for now."`
	OutputFormat      string  `json:"output_format,omitempty" default:"png" enum:"png,jpeg,webp" description:"The format of the output image(s). Only support 'png' for now."`
	Quality           string  `json:"quality,omitempty" default:"auto" enum:"auto,low,medium,high,hd,standard" description:"The quality of the output image(s). Default is 'auto'. No use for now."`
	ResponseFormat    string  `json:"response_format,omitempty" default:"b64_json" enum:"url,b64_json" description:"The format of the response. Only support 'b64_json' for now."`
	Size              string  `json:"size,omitempty" default:"512x512" enum:"256x256,512x512,1024x1024" description:"The size of the output image(s). Default is '512x512'"`
	Style             string  `json:"style,omitempty" enum:"vivid,natural" description:"No use for now. For compatibility with Openai."`
	User              string  `json:"user,omitempty" description:"No use for now. For compatibility with Openai."`
	Timeout           *uint64 `json:"timeout,omitempty" description:"Task timeout" validate:"omitempty"`
}

func (in *CreateImageRequest) SetDefaultValues() {
	if in.Background == "" {
		in.Background = "auto"
	}
	if in.Model == "" {
		in.Model = "crynux-network/sdxl-turbo"
	}
	if in.Moderation == "" {
		in.Moderation = "auto"
	}
	if in.N <= 0 {
		in.N = 1
	}
	if in.OutputCompression <= 0 {
		in.OutputCompression = 100
	}
	if in.OutputFormat == "" {
		in.OutputFormat = "png"
	}
	if in.ResponseFormat == "" {
		in.ResponseFormat = "b64_json"
	}
	if in.Size == "" {
		in.Size = "512x512"
	}
	if in.Style == "" {
		in.Style = "vivid"
	}
}

type CreateImageData struct {
	B64Json       string `json:"b64_json,omitempty"`
	Url           string `json:"url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type CreateImageInputTokensDetails struct {
	ImageTokens int `json:"image_tokens"`
	TextTokens  int `json:"text_tokens"`
}

type CreateImageUsage struct {
	InputTokens        int                           `json:"input_tokens"`
	OutputTokens       int                           `json:"output_tokens"`
	TotalTokens        int                           `json:"total_tokens"`
	InputTokensDetails CreateImageInputTokensDetails `json:"input_tokens_details"`
}

type CreateImageResponse struct {
	Created int64             `json:"created"`
	Data    []CreateImageData `json:"data"`
	Usage   CreateImageUsage  `json:"usage"`
}

var sizePattern = regexp.MustCompile(`^(\d+)(x|X)(\d+)$`)

func streamBase64Encode(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var buf bytes.Buffer
	encoder := base64.NewEncoder(base64.StdEncoding, &buf)
	defer encoder.Close()

	reader := bufio.NewReader(file)
	chunk := make([]byte, 4096)

	for {
		n, err := reader.Read(chunk)
		if err != nil && err.Error() != "EOF" {
			return "", err
		}

		if n == 0 {
			break
		}

		if _, err := encoder.Write(chunk[:n]); err != nil {
			return "", err
		}
	}

	return buf.String(), nil
}

func CreateImage(c *gin.Context, in *CreateImageRequest) (res *CreateImageResponse, err error) {
	ctx := c.Request.Context()
	db := config.GetDB()
	requestStart := time.Now()
	var tracePrimaryTaskIDCommitment string
	var resultDownloadedTask *models.InferenceTask
	defer func() {
		tasktrace.FinishTrace(tracePrimaryTaskIDCommitment, res, err, resultDownloadedTask)
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

	in.SetDefaultValues()

	if in.OutputFormat != "png" {
		return nil, response.NewValidationErrorResponse("output_format", "only support png out format now")
	}

	if in.ResponseFormat != "b64_json" {
		return nil, response.NewValidationErrorResponse("response_format", "only support b64_json out format now")
	}

	if !sizePattern.MatchString(in.Size) {
		return nil, response.NewValidationErrorResponse("size", "size must be in the format of 512x512")
	}

	matches := sizePattern.FindStringSubmatch(in.Size)
	width, _ := strconv.Atoi(matches[1])
	height, _ := strconv.Atoi(matches[3])

	var model models.SDModelArgs
	switch in.Model {
	case "stabilityai/sdxl-turbo":
		model.Name = "crynux-network/sdxl-turbo"
	case "ruwnayml/stable-diffusion-v1-5":
		model.Name = "crynux-network/stable-diffusion-v1-5"
	case "stabilityai/stable-diffusion-xl-base-1.0":
		model.Name = "crynux-network/stable-diffusion-xl-base-1.0"
	default:
		model.Name = in.Model
	}

	if model.Name == "crynux-network/sdxl-turbo" || model.Name == "crynux-network/stable-diffusion-v1-5" || model.Name == "crynux-network/stable-diffusion-xl-base-1.0" {
		model.Variant = "fp16"
	}

	taskConfig := models.SDTaskConfig{
		ImageWidth:    width,
		ImageHeight:   height,
		NumImages:     in.N,
		SafetyChecker: false,
		Steps:         25,
		Seed:          rand.Intn(100000000),
	}

	switch model.Name {
	case "crynux-network/sdxl-turbo":
		taskConfig.Steps = 1
		taskConfig.Cfg = 0
	case "crynux-network/stable-diffusion-v1-5":
		taskConfig.Cfg = 7
	}

	taskArgs := models.SDTaskArgs{
		BaseModel:  model,
		Prompt:     in.Prompt,
		TaskConfig: taskConfig,
	}
	if model.Name == "crynux-network/sdxl-turbo" {
		taskArgs.Scheduler = &models.EulerAncestralDiscrete{
			TimestepSpacing: "trailing",
		}
	}

	taskArgsStr, err := json.Marshal(taskArgs)
	if err != nil {
		err := fmt.Errorf("failed to marshal taskArgs: %w", err)
		return nil, response.NewExceptionResponse(err)
	}
	taskType := models.TaskTypeSD
	task := &inference_tasks.TaskInput{
		ClientID: apiKey.ClientID,
		TaskArgs: string(taskArgsStr),
		TaskType: &taskType,
	}

	requestPayload := *in
	requestPayload.Authorization = ""
	resultFiles, resultDownloadedTask, err := inference_tasks.ProcessSDTask(ctx, db, task, func(createdTasks []models.InferenceTask) {
		if len(createdTasks) == 0 {
			return
		}
		tracePrimaryTaskIDCommitment = tasktrace.StartTrace(tasktrace.StartTraceInput{
			Source:      tasktrace.SourceImageGeneration,
			Endpoint:    c.FullPath(),
			ClientID:    apiKey.ClientID,
			Model:       in.Model,
			TaskType:    &taskType,
			Request:     requestPayload,
			RequestTime: requestStart,
			Tasks:       createdTasks,
		}, config.GetConfig().Admin.TaskTraceMaxTasks)
	})
	if err != nil {
		return nil, err
	}

	b64results := make([]CreateImageData, len(resultFiles))
	var wg sync.WaitGroup

	for i, resultFile := range resultFiles {
		wg.Add(1)
		go func(i int, resultFile string) {
			defer wg.Done()
			b64result, err := streamBase64Encode(resultFile)
			if err != nil {
				return
			}
			b64results[i] = CreateImageData{
				B64Json: b64result,
			}
		}(i, resultFile)
	}

	wg.Wait()

	res = &CreateImageResponse{
		Created: time.Now().Unix(),
		Data:    b64results,
		Usage:   CreateImageUsage{},
	}
	return res, nil
}
