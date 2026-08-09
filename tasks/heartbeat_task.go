package tasks

import (
	"context"
	"crynux_bridge/api/v1/llm/structs"
	"crynux_bridge/config"
	"crynux_bridge/llmtask"
	"crynux_bridge/models"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat/sampleuv"
	"gorm.io/gorm"
)

func generateHeartbeatTask(client models.Client, heartbeatTaskConfig config.HeartbeatTaskConfig) (*models.InferenceTask, error) {
	taskArgs, taskType, err := buildHeartbeatTaskArgs(heartbeatTaskConfig)
	if err != nil {
		return nil, err
	}
	taskModelIDs, _ := models.GetTaskConfigModelIDs(taskArgs, taskType)
	taskFee, err := config.CNXToGWei(heartbeatTaskConfig.FeeCNX)
	if err != nil {
		return nil, err
	}

	taskIDBytes := make([]byte, 32)
	crand.Read(taskIDBytes)
	taskID := hexutil.Encode(taskIDBytes)

	task := &models.InferenceTask{
		Client:       client,
		ClientTask:   models.ClientTask{Client: client},
		TaskArgs:     taskArgs,
		TaskType:     taskType,
		TaskModelIDs: taskModelIDs,
		TaskVersion:  heartbeatTaskConfig.TaskVersion,
		MinVram:      heartbeatTaskConfig.MinVram,
		TaskFee:      taskFee,
		TaskSize:     1,
		TaskID:       taskID,
	}
	return task, nil
}

func heartbeatTypeModelKey(taskType string, model string) string {
	return strings.ToLower(taskType) + "|" + model
}

func heartbeatTaskTypeAndModelID(heartbeatTaskConfig config.HeartbeatTaskConfig) (models.ChainTaskType, string, error) {
	switch strings.ToLower(heartbeatTaskConfig.Type) {
	case "sd":
		taskArgs, err := buildSDHeartbeatTaskArgs(heartbeatTaskConfig.Model, config.HeartbeatPromptConfig{}, true)
		if err != nil {
			return 0, "", err
		}
		modelIDs, err := models.GetTaskConfigModelIDs(taskArgs, models.TaskTypeSD)
		if err != nil {
			return 0, "", err
		}
		if len(modelIDs) == 0 {
			return 0, "", errors.New("sd heartbeat task has no model ids")
		}
		return models.TaskTypeSD, modelIDs[0], nil
	case "llm":
		return models.TaskTypeLLM, "base:" + heartbeatTaskConfig.Model, nil
	default:
		return 0, "", fmt.Errorf("unsupported heartbeat task type %q", heartbeatTaskConfig.Type)
	}
}

func selectHeartbeatTaskConfig(
	heartbeatTaskConfigs []config.HeartbeatTaskConfig,
	pendingCounts map[string]uint64,
	batchIncrements map[string]uint64,
) (config.HeartbeatTaskConfig, error) {
	eligibleConfigs := make([]config.HeartbeatTaskConfig, 0, len(heartbeatTaskConfigs))
	weights := make([]float64, 0, len(heartbeatTaskConfigs))

	for _, heartbeatTaskConfig := range heartbeatTaskConfigs {
		if heartbeatTaskConfig.Ratio <= 0 {
			continue
		}
		key := heartbeatTypeModelKey(heartbeatTaskConfig.Type, heartbeatTaskConfig.Model)
		count := pendingCounts[key] + batchIncrements[key]
		if count > heartbeatTaskConfig.MaxPendingTasks {
			continue
		}
		eligibleConfigs = append(eligibleConfigs, heartbeatTaskConfig)
		weights = append(weights, heartbeatTaskConfig.Ratio)
	}

	if len(eligibleConfigs) == 0 {
		return config.HeartbeatTaskConfig{}, errors.New("no eligible heartbeat task config")
	}

	sampler := sampleuv.NewWeighted(weights, nil)
	idx, ok := sampler.Take()
	if !ok {
		return config.HeartbeatTaskConfig{}, errors.New("cannot sample heartbeat task config")
	}
	return eligibleConfigs[idx], nil
}

func buildHeartbeatTaskArgs(heartbeatTaskConfig config.HeartbeatTaskConfig) (string, models.ChainTaskType, error) {
	prompt, useDefault, err := selectHeartbeatPrompt(heartbeatTaskConfig.Prompts)
	if err != nil {
		return "", 0, err
	}

	switch strings.ToLower(heartbeatTaskConfig.Type) {
	case "sd":
		taskArgs, err := buildSDHeartbeatTaskArgs(heartbeatTaskConfig.Model, prompt, useDefault)
		return taskArgs, models.TaskTypeSD, err
	case "llm":
		taskArgs, err := buildLLMHeartbeatTaskArgs(heartbeatTaskConfig, prompt, useDefault)
		return taskArgs, models.TaskTypeLLM, err
	default:
		return "", 0, fmt.Errorf("unsupported heartbeat task type %q", heartbeatTaskConfig.Type)
	}
}

func selectHeartbeatPrompt(prompts []config.HeartbeatPromptConfig) (config.HeartbeatPromptConfig, bool, error) {
	if len(prompts) == 0 {
		return config.HeartbeatPromptConfig{}, true, nil
	}
	return prompts[rand.Intn(len(prompts))], false, nil
}

func buildSDHeartbeatTaskArgs(model string, prompt config.HeartbeatPromptConfig, useDefault bool) (string, error) {
	seed := rand.Intn(100000000)
	baseModel := map[string]interface{}{
		"name":    model,
		"variant": "fp16",
	}

	var taskArgs map[string]interface{}
	if model == "crynux-network/sdxl-turbo" {
		promptText := "Self-portrait oil painting,a beautiful cyborg with golden hair,8k"
		negativePrompt := ""
		if !useDefault {
			promptText = prompt.Text
			negativePrompt = prompt.NegativePrompt
		}
		taskArgs = map[string]interface{}{
			"base_model":      baseModel,
			"prompt":          promptText,
			"negative_prompt": negativePrompt,
			"scheduler": map[string]interface{}{
				"method": "EulerAncestralDiscreteScheduler",
				"args": map[string]interface{}{
					"timestep_spacing": "trailing",
				},
			},
			"task_config": map[string]interface{}{
				"num_images":     1,
				"seed":           seed,
				"steps":          1,
				"cfg":            0,
				"safety_checker": false,
			},
		}
	} else {
		promptText := "best quality, ultra high res, photorealistic++++, 1girl, off-shoulder sweater, smiling, faded ash gray messy bun hair+, border light, depth of field, looking at viewer, closeup"
		negativePrompt := "paintings, sketches, worst quality+++++, low quality+++++, normal quality+++++, lowres, normal quality, monochrome++, grayscale++, skin spots, acnes, skin blemishes, age spot, glans"
		if !useDefault {
			promptText = prompt.Text
			negativePrompt = prompt.NegativePrompt
		}
		taskArgs = map[string]interface{}{
			"base_model":      baseModel,
			"prompt":          promptText,
			"negative_prompt": negativePrompt,
			"task_config": map[string]interface{}{
				"num_images":     1,
				"seed":           seed,
				"steps":          25,
				"cfg":            0,
				"safety_checker": false,
			},
		}
	}

	taskArgsBytes, err := json.Marshal(taskArgs)
	if err != nil {
		return "", err
	}
	return string(taskArgsBytes), nil
}

func buildLLMHeartbeatTaskArgs(
	heartbeatTaskConfig config.HeartbeatTaskConfig,
	prompt config.HeartbeatPromptConfig,
	useDefault bool,
) (string, error) {
	messages, err := buildLLMHeartbeatMessages(prompt, useDefault)
	if err != nil {
		return "", err
	}
	adaptedMessages, err := llmtask.BuildTemplateToolCallMessages(heartbeatTaskConfig.Model, messages)
	if err != nil {
		return "", err
	}

	taskArgs := map[string]interface{}{
		"model":    heartbeatTaskConfig.Model,
		"messages": adaptedMessages,
		"generation_config": map[string]interface{}{
			"max_new_tokens":     heartbeatTaskConfig.MaxNewTokens,
			"do_sample":          false,
			"temperature":        0,
			"repetition_penalty": 1.1,
		},
		"seed":  rand.Intn(100000000),
		"dtype": "bfloat16",
	}
	if len(heartbeatTaskConfig.Tools) > 0 {
		taskArgs["tools"] = heartbeatTaskConfig.Tools
	}

	taskArgsBytes, err := json.Marshal(taskArgs)
	if err != nil {
		return "", err
	}
	return string(taskArgsBytes), nil
}

func buildLLMHeartbeatMessages(prompt config.HeartbeatPromptConfig, useDefault bool) ([]models.Message, error) {
	if useDefault {
		return []models.Message{{
			Role:    models.LLMRoleUser,
			Content: "I want to create an AI agent. Any suggestions?",
		}}, nil
	}
	if len(prompt.Messages) > 0 {
		return convertHeartbeatMessages(prompt.Messages)
	}
	content, err := buildLLMHeartbeatMessageContent(prompt)
	if err != nil {
		return nil, err
	}
	return []models.Message{{
		Role:    models.LLMRoleUser,
		Content: content,
	}}, nil
}

func convertHeartbeatMessages(messages []config.HeartbeatMessageConfig) ([]models.Message, error) {
	converted := make([]models.Message, 0, len(messages))
	for i, message := range messages {
		role, err := parseHeartbeatMessageRole(message.Role)
		if err != nil {
			return nil, fmt.Errorf("messages[%d]: %w", i, err)
		}
		convertedMessage := models.Message{
			Role:       role,
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
		}
		if len(message.ToolCalls) > 0 {
			toolCalls := make([]structs.ToolCall, 0, len(message.ToolCalls))
			for _, toolCall := range message.ToolCalls {
				toolCalls = append(toolCalls, structs.ToolCall{
					Id:   toolCall.ID,
					Type: toolCall.Type,
					Function: structs.FunctionCall{
						Name:      toolCall.Function.Name,
						Arguments: toolCall.Function.Arguments,
					},
				})
			}
			convertedMessage.ToolCalls = toolCalls
		}
		converted = append(converted, convertedMessage)
	}
	return converted, nil
}

func parseHeartbeatMessageRole(role string) (models.LLMRole, error) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "system":
		return models.LLMRoleSystem, nil
	case "user":
		return models.LLMRoleUser, nil
	case "assistant":
		return models.LLMRoleAssistant, nil
	case "tool":
		return models.LLMRoleTool, nil
	default:
		return "", fmt.Errorf("unsupported role %q", role)
	}
}

func buildLLMHeartbeatMessageContent(prompt config.HeartbeatPromptConfig) (any, error) {
	if len(prompt.Content) > 0 {
		blocks := make([]models.MessageContentBlock, 0, len(prompt.Content))
		for _, block := range prompt.Content {
			switch block.Type {
			case "text":
				blocks = append(blocks, models.MessageContentBlock{
					Type: "text",
					Text: block.Text,
				})
			case "image":
				blocks = append(blocks, models.MessageContentBlock{
					Type:   "image",
					Base64: block.Base64,
				})
			default:
				return nil, fmt.Errorf("unsupported content block type %q", block.Type)
			}
		}
		return blocks, nil
	}
	if strings.TrimSpace(prompt.Text) == "" {
		return nil, errors.New("heartbeat prompt text is empty")
	}
	return prompt.Text, nil
}

func getPendingHeartbeatTasksCount(
	ctx context.Context,
	client models.Client,
	taskType models.ChainTaskType,
	modelID string,
) (uint64, error) {
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	var count int64
	err := config.GetDB().WithContext(dbCtx).
		Model(&models.InferenceTask{}).
		Where("client_id = ?", client.ID).
		Where("task_type = ?", taskType).
		Where("task_model_ids = ?", modelID).
		Where("status NOT IN ?", []models.TaskStatus{
			models.InferenceTaskEndAborted,
			models.InferenceTaskEndGroupRefund,
			models.InferenceTaskEndInvalidated,
			models.InferenceTaskEndSuccess,
			models.InferenceTaskResultDownloaded,
		}).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return uint64(count), nil
}

func getPendingHeartbeatTasksCounts(
	ctx context.Context,
	client models.Client,
	heartbeatTaskConfigs []config.HeartbeatTaskConfig,
) (map[string]uint64, error) {
	counts := make(map[string]uint64)
	for _, heartbeatTaskConfig := range heartbeatTaskConfigs {
		if heartbeatTaskConfig.Ratio <= 0 {
			continue
		}
		key := heartbeatTypeModelKey(heartbeatTaskConfig.Type, heartbeatTaskConfig.Model)
		if _, ok := counts[key]; ok {
			continue
		}
		taskType, modelID, err := heartbeatTaskTypeAndModelID(heartbeatTaskConfig)
		if err != nil {
			return nil, err
		}
		count, err := getPendingHeartbeatTasksCount(ctx, client, taskType, modelID)
		if err != nil {
			return nil, err
		}
		counts[key] = count
	}
	return counts, nil
}

func heartbeatCreateTasks(ctx context.Context) error {
	appConfig := config.GetConfig()

	clientID := "heartbeat-task"
	client := models.Client{ClientId: clientID}
	currentHour := time.Now().Truncate(time.Hour)
	tasksCreatedInHour := uint64(0)

	if err := func() error {
		dbCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		err := config.GetDB().WithContext(dbCtx).Model(&client).Where(&client).First(&client).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return config.GetDB().WithContext(dbCtx).Create(&client).Error
			}
			return err
		}
		return nil
	}(); err != nil {
		log.Errorf("HeartbeatTask: create client failed: %v", err)
		return err
	}

	for {
		batchSize := int(appConfig.Task.HeartbeatTasks.BatchSize)
		if batchSize > 0 {
			now := time.Now()
			hour := now.Truncate(time.Hour)
			if hour.After(currentHour) {
				currentHour = hour
				tasksCreatedInHour = 0
			}

			maxTasksPerHour := appConfig.Task.HeartbeatTasks.MaxTasksPerHour
			if maxTasksPerHour > 0 {
				if tasksCreatedInHour >= maxTasksPerHour {
					log.Infof("HeartbeatTask: max tasks per hour reached: %d", maxTasksPerHour)
					time.Sleep(2 * time.Second)
					continue
				}

				remainingTasks := maxTasksPerHour - tasksCreatedInHour
				if uint64(batchSize) > remainingTasks {
					batchSize = int(remainingTasks)
				}
			}

			pendingCounts, err := getPendingHeartbeatTasksCounts(ctx, client, appConfig.Task.HeartbeatTasks.Tasks)
			if err != nil {
				log.Errorf("HeartbeatTask: cannot get pending heartbeat tasks counts %v", err)
				time.Sleep(2 * time.Second)
				continue
			}
			log.Infof("HeartbeatTask: in-flight heartbeat tasks counts: %v", pendingCounts)

			tasks := make([]*models.InferenceTask, 0, batchSize)
			batchIncrements := make(map[string]uint64)
			generationFailed := false
			for i := 0; i < batchSize; i++ {
				heartbeatTaskConfig, err := selectHeartbeatTaskConfig(
					appConfig.Task.HeartbeatTasks.Tasks,
					pendingCounts,
					batchIncrements,
				)
				if err != nil {
					break
				}
				task, err := generateHeartbeatTask(client, heartbeatTaskConfig)
				if err != nil {
					log.Errorf("HeartbeatTask: cannot generate heartbeat task: %v", err)
					generationFailed = true
					break
				}
				tasks = append(tasks, task)
				key := heartbeatTypeModelKey(heartbeatTaskConfig.Type, heartbeatTaskConfig.Model)
				batchIncrements[key]++
			}
			if generationFailed {
				time.Sleep(2 * time.Second)
				continue
			}
			if len(tasks) == 0 {
				time.Sleep(2 * time.Second)
				continue
			}
			if err := models.SaveTasks(ctx, config.GetDB(), tasks); err != nil {
				log.Errorf("HeartbeatTask: cannot save heartbeat tasks: %v", err)
				return err
			}
			tasksCreatedInHour += uint64(len(tasks))
		}
		time.Sleep(2 * time.Second)
	}
}

func HeartbeatCreateTasks(ctx context.Context) {
	ctx1, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		for {
			err := heartbeatCreateTasks(ctx1)
			if err != nil {
				log.Errorf("HeartbeatTask: heartbeat create tasks error: %v", err)
				time.Sleep(5 * time.Second)
			}
		}
	}()
	<-ctx1.Done()
	err := ctx1.Err()
	log.Errorf("HeartbeatTask: timeout %v, finish", err)
}
