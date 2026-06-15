package tasks

import (
	"context"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/relay"
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

func generateHeartbeatTask(client models.Client) (*models.InferenceTask, error) {
	heartbeatTaskConfig, err := selectHeartbeatTaskConfig(config.GetConfig().Task.HeartbeatTasks.Tasks)
	if err != nil {
		return nil, err
	}

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

func selectHeartbeatTaskConfig(heartbeatTaskConfigs []config.HeartbeatTaskConfig) (config.HeartbeatTaskConfig, error) {
	eligibleConfigs := make([]config.HeartbeatTaskConfig, 0, len(heartbeatTaskConfigs))
	weights := make([]float64, 0, len(heartbeatTaskConfigs))

	for _, heartbeatTaskConfig := range heartbeatTaskConfigs {
		if heartbeatTaskConfig.Ratio <= 0 {
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
	switch strings.ToLower(heartbeatTaskConfig.Type) {
	case "sd":
		taskArgs, err := buildSDHeartbeatTaskArgs(heartbeatTaskConfig.Model)
		return taskArgs, models.TaskTypeSD, err
	case "llm":
		taskArgs, err := buildLLMHeartbeatTaskArgs(heartbeatTaskConfig.Model)
		return taskArgs, models.TaskTypeLLM, err
	default:
		return "", 0, fmt.Errorf("unsupported heartbeat task type %q", heartbeatTaskConfig.Type)
	}
}

func buildSDHeartbeatTaskArgs(model string) (string, error) {
	seed := rand.Intn(100000000)
	baseModel := map[string]interface{}{
		"name":    model,
		"variant": "fp16",
	}

	var taskArgs map[string]interface{}
	if model == "crynux-network/sdxl-turbo" {
		taskArgs = map[string]interface{}{
			"base_model":      baseModel,
			"prompt":          "Self-portrait oil painting,a beautiful cyborg with golden hair,8k",
			"negative_prompt": "",
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
		taskArgs = map[string]interface{}{
			"base_model":      baseModel,
			"prompt":          "best quality, ultra high res, photorealistic++++, 1girl, off-shoulder sweater, smiling, faded ash gray messy bun hair+, border light, depth of field, looking at viewer, closeup",
			"negative_prompt": "paintings, sketches, worst quality+++++, low quality+++++, normal quality+++++, lowres, normal quality, monochrome++, grayscale++, skin spots, acnes, skin blemishes, age spot, glans",
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

func buildLLMHeartbeatTaskArgs(model string) (string, error) {
	taskArgs := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "I want to create an AI agent. Any suggestions?",
			},
		},
		"tools": nil,
		"generation_config": map[string]interface{}{
			"max_new_tokens":     250,
			"do_sample":          true,
			"temperature":        0.8,
			"repetition_penalty": 1.1,
		},
		"seed":  rand.Intn(100000000),
		"dtype": "bfloat16",
	}

	taskArgsBytes, err := json.Marshal(taskArgs)
	if err != nil {
		return "", err
	}
	return string(taskArgsBytes), nil
}

func getPendingHeartbeatTasksCount(ctx context.Context, client models.Client) (uint64, error) {
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	task := &models.InferenceTask{
		Client: client,
	}
	var count int64
	if err := config.GetDB().WithContext(dbCtx).Model(&task).Where(&task).Where("(status = ? OR status = ?)", models.InferenceTaskPending, models.InferenceTaskStarted).Count(&count).Error; err != nil {
		return 0, err
	}
	return uint64(count), nil
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

			tasks := make([]*models.InferenceTask, 0, batchSize)
			cnt, err := getPendingHeartbeatTasksCount(ctx, client)
			if err != nil {
				log.Errorf("HeartbeatTask: cannot get pending heartbeat tasks count %v", err)
				time.Sleep(2 * time.Second)
				continue
			}
			log.Infof("HeartbeatTask: pending heartbeat tasks count: %d", cnt)
			if cnt > appConfig.Task.HeartbeatTasks.PendingTasksLimit {
				time.Sleep(2 * time.Second)
				continue
			}
			queuedTasks, err := relay.GetQueuedTasks(ctx)
			if err != nil {
				log.Errorf("HeartbeatTask: cannot get queued tasks count %v", err)
				time.Sleep(2 * time.Second)
				continue
			}
			log.Infof("HeartbeatTask: queued task count %d", queuedTasks)
			if uint64(queuedTasks) > appConfig.Task.HeartbeatTasks.PendingTasksLimit {
				time.Sleep(2 * time.Second)
				continue
			}

			generationFailed := false
			for i := 0; i < batchSize; i++ {
				task, err := generateHeartbeatTask(client)
				if err != nil {
					log.Errorf("HeartbeatTask: cannot generate heartbeat task: %v", err)
					generationFailed = true
					break
				}
				tasks = append(tasks, task)
			}
			if generationFailed {
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
