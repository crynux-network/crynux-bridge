package tasks

import (
	"context"
	"crypto/rand"
	"crynux_bridge/config"
	"crynux_bridge/llmtask"
	"crynux_bridge/models"
	"crynux_bridge/taskengine"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	mathrand "math/rand"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat/sampleuv"
)

const heartbeatRandHexLen = 16

func generateHeartbeatTask(heartbeatTaskConfig config.HeartbeatTaskConfig) (taskengine.Submission, error) {
	taskArgs, taskType, err := buildHeartbeatTaskArgs(heartbeatTaskConfig)
	if err != nil {
		return taskengine.Submission{}, err
	}
	taskModelIDs, _ := models.GetTaskConfigModelIDs(taskArgs, taskType)
	taskFee, err := config.CNXToWei(heartbeatTaskConfig.FeeCNX)
	if err != nil {
		return taskengine.Submission{}, err
	}
	repeatNum := 1
	taskVersion, err := models.ParseTaskArgsVersion(taskArgs)
	if err != nil {
		return taskengine.Submission{}, err
	}
	return taskengine.Submission{
		TaskArgs:     taskArgs,
		TaskType:     taskType,
		TaskModelIDs: taskModelIDs,
		TaskVersion:  taskVersion,
		MinVram:      heartbeatTaskConfig.MinVram,
		TaskFee:      taskFee,
		TaskSize:     1,
		RepeatNum:    &repeatNum,
	}, nil
}

func heartbeatTypeModelKey(taskType string, model string) string {
	return strings.ToLower(taskType) + "|" + model
}

func heartbeatTaskTypeAndModelID(heartbeatTaskConfig config.HeartbeatTaskConfig) (models.ChainTaskType, string, error) {
	switch strings.ToLower(heartbeatTaskConfig.Type) {
	case "sd":
		taskArgs, err := buildSDHeartbeatTaskArgs(heartbeatTaskConfig, config.HeartbeatPromptConfig{}, true)
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
		taskArgs, err := buildSDHeartbeatTaskArgs(heartbeatTaskConfig, prompt, useDefault)
		return taskArgs, models.TaskTypeSD, err
	case "llm":
		prompt, useDefault, err = applyHeartbeatRandPlaceholder(prompt, useDefault)
		if err != nil {
			return "", 0, err
		}
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
	return prompts[mathrand.Intn(len(prompts))], false, nil
}

func newHeartbeatRandValue() (string, error) {
	raw := make([]byte, heartbeatRandHexLen/2)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate heartbeat rand: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func applyHeartbeatRandPlaceholder(
	prompt config.HeartbeatPromptConfig,
	useDefault bool,
) (config.HeartbeatPromptConfig, bool, error) {
	value, err := newHeartbeatRandValue()
	if err != nil {
		return config.HeartbeatPromptConfig{}, false, err
	}
	if useDefault {
		text := strings.ReplaceAll(defaultLLMHeartbeatPromptText(), config.HeartbeatRandPlaceholder, value)
		if !strings.Contains(text, value) {
			return config.HeartbeatPromptConfig{}, false, fmt.Errorf(
				"llm heartbeat prompt must contain %s", config.HeartbeatRandPlaceholder,
			)
		}
		return config.HeartbeatPromptConfig{Text: text}, false, nil
	}

	copied, err := cloneHeartbeatPrompt(prompt)
	if err != nil {
		return config.HeartbeatPromptConfig{}, false, err
	}
	replaced := replaceHeartbeatRandInPrompt(&copied, value)
	if !replaced || !heartbeatPromptContainsValue(copied, value) {
		return config.HeartbeatPromptConfig{}, false, fmt.Errorf(
			"llm heartbeat prompt must contain %s", config.HeartbeatRandPlaceholder,
		)
	}
	return copied, false, nil
}

func defaultLLMHeartbeatPromptText() string {
	return "Ticket " + config.HeartbeatRandPlaceholder +
		" needs a one-sentence triage note that repeats the ticket number and says whether the 27B pool should stay drained."
}

func heartbeatPromptContainsValue(prompt config.HeartbeatPromptConfig, value string) bool {
	if strings.Contains(prompt.Text, value) {
		return true
	}
	for _, block := range prompt.Content {
		if block.Type == "text" && strings.Contains(block.Text, value) {
			return true
		}
	}
	for _, message := range prompt.Messages {
		if heartbeatMessageContentContainsValue(message.Content, value) {
			return true
		}
	}
	return false
}

func heartbeatMessageContentContainsValue(content any, value string) bool {
	switch typed := content.(type) {
	case string:
		return strings.Contains(typed, value)
	case []any:
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if block["type"] != "text" {
				continue
			}
			text, ok := block["text"].(string)
			if ok && strings.Contains(text, value) {
				return true
			}
		}
	}
	return false
}

func cloneHeartbeatPrompt(prompt config.HeartbeatPromptConfig) (config.HeartbeatPromptConfig, error) {
	raw, err := json.Marshal(prompt)
	if err != nil {
		return config.HeartbeatPromptConfig{}, err
	}
	var copied config.HeartbeatPromptConfig
	if err := json.Unmarshal(raw, &copied); err != nil {
		return config.HeartbeatPromptConfig{}, err
	}
	return copied, nil
}

func replaceHeartbeatRandInPrompt(prompt *config.HeartbeatPromptConfig, value string) bool {
	replaced := false
	placeholder := config.HeartbeatRandPlaceholder
	if strings.Contains(prompt.Text, placeholder) {
		prompt.Text = strings.ReplaceAll(prompt.Text, placeholder, value)
		replaced = true
	}
	for i := range prompt.Content {
		if prompt.Content[i].Type == "text" && strings.Contains(prompt.Content[i].Text, placeholder) {
			prompt.Content[i].Text = strings.ReplaceAll(prompt.Content[i].Text, placeholder, value)
			replaced = true
		}
	}
	for i := range prompt.Messages {
		if replaceHeartbeatRandInMessageContent(&prompt.Messages[i].Content, value) {
			replaced = true
		}
	}
	return replaced
}

func replaceHeartbeatRandInMessageContent(content *any, value string) bool {
	placeholder := config.HeartbeatRandPlaceholder
	switch typed := (*content).(type) {
	case string:
		if !strings.Contains(typed, placeholder) {
			return false
		}
		*content = strings.ReplaceAll(typed, placeholder, value)
		return true
	case []any:
		replaced := false
		for i, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if block["type"] != "text" {
				continue
			}
			text, ok := block["text"].(string)
			if !ok || !strings.Contains(text, placeholder) {
				continue
			}
			block["text"] = strings.ReplaceAll(text, placeholder, value)
			typed[i] = block
			replaced = true
		}
		if replaced {
			*content = typed
		}
		return replaced
	default:
		return false
	}
}

const (
	heartbeatSDImageSize    = 512
	defaultSDXLTurboPrompt  = "Self-portrait oil painting,a beautiful cyborg with golden hair,8k"
	defaultSDPrompt         = "best quality, ultra high res, photorealistic++++, 1girl, off-shoulder sweater, smiling, faded ash gray messy bun hair+, border light, depth of field, looking at viewer, closeup"
	defaultSDNegativePrompt = "paintings, sketches, worst quality+++++, low quality+++++, normal quality+++++, lowres, normal quality, monochrome++, grayscale++, skin spots, acnes, skin blemishes, age spot, glans"
)

func buildSDHeartbeatTaskArgs(heartbeatTaskConfig config.HeartbeatTaskConfig, prompt config.HeartbeatPromptConfig, useDefault bool) (string, error) {
	promptText, negativePrompt := defaultSDPrompt, defaultSDNegativePrompt
	if heartbeatTaskConfig.Model == "crynux-network/sdxl-turbo" {
		promptText, negativePrompt = defaultSDXLTurboPrompt, ""
	}
	if !useDefault {
		promptText = prompt.Text
		negativePrompt = prompt.NegativePrompt
	}

	taskArgs := map[string]interface{}{
		"version":         heartbeatTaskConfig.TaskVersion,
		"base_model":      map[string]interface{}{"name": heartbeatTaskConfig.Model},
		"prompt":          promptText,
		"negative_prompt": negativePrompt,
		"task_config": map[string]interface{}{
			"image_width":           heartbeatSDImageSize,
			"image_height":          heartbeatSDImageSize,
			"num_images_per_prompt": 1,
			"seed":                  mathrand.Intn(100000000),
			"steps":                 heartbeatTaskConfig.Steps,
			"guidance_scale":        0,
		},
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
		"version":  heartbeatTaskConfig.TaskVersion,
		"model":    heartbeatTaskConfig.Model,
		"messages": adaptedMessages,
		"generation_config": map[string]interface{}{
			"max_new_tokens":     heartbeatTaskConfig.MaxNewTokens,
			"do_sample":          false,
			"temperature":        0,
			"repetition_penalty": 1.1,
		},
		"seed":  mathrand.Intn(100000000),
		"dtype": "bfloat16",
	}
	if len(heartbeatTaskConfig.Tools) > 0 {
		taskArgs["tools"] = heartbeatTaskConfig.Tools
	}
	if heartbeatTaskConfig.ToolChoice != nil {
		taskArgs["tool_choice"] = heartbeatTaskConfig.ToolChoice
	}
	if len(heartbeatTaskConfig.ResponseFormat) > 0 {
		taskArgs["response_format"] = heartbeatTaskConfig.ResponseFormat
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
			Content: defaultLLMHeartbeatPromptText(),
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
			toolCalls := make([]models.ToolCall, 0, len(message.ToolCalls))
			for _, toolCall := range message.ToolCalls {
				toolCalls = append(toolCalls, models.ToolCall{
					Id:   toolCall.ID,
					Type: toolCall.Type,
					Function: models.FunctionCall{
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
	clientID string,
	taskType models.ChainTaskType,
	modelID string,
) (uint64, error) {
	engine, err := taskengine.Default()
	if err != nil {
		return 0, err
	}
	return engine.CountPending(ctx, clientID, taskType, modelID)
}

func getPendingHeartbeatTasksCounts(
	ctx context.Context,
	clientID string,
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
		count, err := getPendingHeartbeatTasksCount(ctx, clientID, taskType, modelID)
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
	engine, err := taskengine.Default()
	if err != nil {
		return err
	}
	currentHour := time.Now().Truncate(time.Hour)
	tasksCreatedInHour, err := engine.CountSubmittedSince(ctx, clientID, currentHour)
	if err != nil {
		return err
	}

	for {
		batchSize := int(appConfig.Task.HeartbeatTasks.BatchSize)
		if batchSize > 0 {
			now := time.Now()
			hour := now.Truncate(time.Hour)
			if hour.After(currentHour) {
				currentHour = hour
				tasksCreatedInHour, err = engine.CountSubmittedSince(ctx, clientID, currentHour)
				if err != nil {
					return err
				}
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

			pendingCounts, err := getPendingHeartbeatTasksCounts(ctx, clientID, appConfig.Task.HeartbeatTasks.Tasks)
			if err != nil {
				log.Errorf("HeartbeatTask: cannot get pending heartbeat tasks counts %v", err)
				time.Sleep(2 * time.Second)
				continue
			}
			log.Infof("HeartbeatTask: in-flight heartbeat tasks counts: %v", pendingCounts)

			submissions := make([]taskengine.Submission, 0, batchSize)
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
				submission, err := generateHeartbeatTask(heartbeatTaskConfig)
				if err != nil {
					log.Errorf("HeartbeatTask: cannot generate heartbeat task: %v", err)
					generationFailed = true
					break
				}
				submissions = append(submissions, submission)
				key := heartbeatTypeModelKey(heartbeatTaskConfig.Type, heartbeatTaskConfig.Model)
				batchIncrements[key]++
			}
			if generationFailed {
				time.Sleep(2 * time.Second)
				continue
			}
			if len(submissions) == 0 {
				time.Sleep(2 * time.Second)
				continue
			}
			for _, submission := range submissions {
				if _, err := engine.CreateTask(ctx, clientID, nil, submission); err != nil {
					log.Errorf("HeartbeatTask: cannot create heartbeat task: %v", err)
					return err
				}
				tasksCreatedInHour++
			}
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
