package tasks

import (
	"context"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/relay"
	crand "crypto/rand"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat/sampleuv"
	"gorm.io/gorm"
)

func generateRandomTask(client models.Client, pendingLargeVramLLMTasksCount uint64) *models.InferenceTask {
	appConfig := config.GetConfig()
	pendingLargeVramLLMTasksLimit := appConfig.Task.PendingLargeVramLLMTasksLimit

	clientTask := models.ClientTask{Client: client}

	var taskArgs string
	var minVram uint64 = 0
	var taskType models.ChainTaskType
	var taskFee uint64
	var requiredGPU string
	var requiredGPUVram uint64

	taskVersionSampler := sampleuv.NewWeighted(appConfig.Task.AutoTaskVersionRatio, nil)
	taskVersionIdx, _ := taskVersionSampler.Take()
	taskVersion := appConfig.Task.TaskVersions[taskVersionIdx]

	taskTypeSampler := sampleuv.NewWeighted(appConfig.Task.AutoTaskTypeRatio, nil)
	taskTypeIdx, _ := taskTypeSampler.Take()

	switch taskTypeIdx {
	case 0:
		prompt := "Self-portrait oil painting,a beautiful cyborg with golden hair,8k"
		seed := rand.Intn(100000000)
		taskArgs = fmt.Sprintf(`{"base_model":{"name":"crynux-network/sdxl-turbo", "variant": "fp16"},"prompt":"%s","negative_prompt":"","scheduler":{"method":"EulerAncestralDiscreteScheduler","args":{"timestep_spacing":"trailing"}},"task_config":{"num_images":1,"seed":%d,"steps":1,"cfg":0,"safety_checker":false}}`, prompt, seed)
		minVram = 14
		taskType = models.TaskTypeSD
		taskFee = appConfig.Task.SDXLTaskFee
	case 1:
		prompt := "best quality, ultra high res, photorealistic++++, 1girl, off-shoulder sweater, smiling, faded ash gray messy bun hair+, border light, depth of field, looking at viewer, closeup"
		negativePrompt := "paintings, sketches, worst quality+++++, low quality+++++, normal quality+++++, lowres, normal quality, monochrome++, grayscale++, skin spots, acnes, skin blemishes, age spot, glans"
		seed := rand.Intn(100000000)
		taskArgs = fmt.Sprintf(`{"base_model":{"name":"crynux-network/stable-diffusion-v1-5", "variant": "fp16"},"prompt":"%s","negative_prompt":"%s","task_config":{"num_images":1,"seed":%d,"steps":25,"cfg":0,"safety_checker":false}}`, prompt, negativePrompt, seed)
		minVram = 4
		taskType = models.TaskTypeSD
		taskFee = appConfig.Task.SDTaskFee
	default:
		taskType = models.TaskTypeLLM
		seed := rand.Intn(100000000)
		if pendingLargeVramLLMTasksCount < pendingLargeVramLLMTasksLimit {
			r := rand.Float64()
			if r < 0.5 {
				minVram = 32
				taskArgs = fmt.Sprintf(`{"model":"Qwen/Qwen2.5-14B","messages":[{"role":"user","content":"I want to create an AI agent. Any suggestions?"}],"tools":null,"generation_config":{"max_new_tokens":250,"do_sample":true,"temperature":0.8,"repetition_penalty":1.1},"seed":%d,"dtype":"bfloat16"}`, seed)
				taskFee = appConfig.Task.LLMTaskFee * 4
			} else {
				minVram = 60
				taskArgs = fmt.Sprintf(`{"model":"Qwen/Qwen2.5-32B","messages":[{"role":"user","content":"I want to create an AI agent. Any suggestions?"}],"tools":null,"generation_config":{"max_new_tokens":250,"do_sample":true,"temperature":0.8,"repetition_penalty":1.1},"seed":%d,"dtype":"bfloat16"}`, seed)
				taskFee = appConfig.Task.LLMTaskFee * 8
			}
		} else {
			minVram = 24
			taskArgs = fmt.Sprintf(`{"model":"Qwen/Qwen2.5-7B","messages":[{"role":"user","content":"I want to create an AI agent. Any suggestions?"}],"tools":null,"generation_config":{"max_new_tokens":250,"do_sample":true,"temperature":0.8,"repetition_penalty":1.1},"seed":%d,"dtype":"bfloat16"}`, seed)
			taskFee = appConfig.Task.LLMTaskFee
		}
	}
	taskModelIDs, _ := models.GetTaskConfigModelIDs(taskArgs, taskType)

	taskIDBytes := make([]byte, 32)
	crand.Read(taskIDBytes)
	taskID := hexutil.Encode(taskIDBytes)

	task := &models.InferenceTask{
		Client:          client,
		ClientTask:      clientTask,
		TaskArgs:        taskArgs,
		TaskType:        taskType,
		TaskModelIDs:    taskModelIDs,
		TaskVersion:     taskVersion,
		MinVram:         minVram,
		RequiredGPU:     requiredGPU,
		RequiredGPUVram: requiredGPUVram,
		TaskFee:         taskFee,
		TaskSize:        1,
		TaskID:          taskID,
	}
	return task
}

func getPendingAutoTasksCount(ctx context.Context, client models.Client) (uint64, error) {
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

func getPendingLargeVramLLMTasksCount(ctx context.Context, client models.Client) (uint64, error) {
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	task := &models.InferenceTask{
		Client:   client,
		TaskType: models.TaskTypeLLM,
	}
	var count int64
	if err := config.GetDB().WithContext(dbCtx).Model(&task).Where(&task).Where("(status = ? OR status = ?)", models.InferenceTaskPending, models.InferenceTaskStarted).Where("min_vram > ?", 24).Count(&count).Error; err != nil {
		return 0, err
	}
	return uint64(count), nil
}

func autoCreateTasks(ctx context.Context) error {
	appConfig := config.GetConfig()

	clientID := "auto-task"
	client := models.Client{ClientId: clientID}

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
		log.Errorf("AutoTask: create client failed: %v", err)
		return err
	}

	for {
		batchSize := int(appConfig.Task.AutoTasksBatchSize)
		if batchSize > 0 {
			tasks := make([]*models.InferenceTask, batchSize)
			cnt, err := getPendingAutoTasksCount(ctx, client)
			if err != nil {
				log.Errorf("AutoTask: cannot get pending auto tasks count %v", err)
				time.Sleep(2 * time.Second)
				continue
			}
			log.Infof("AutoTask: pending auto tasks count: %d", cnt)
			if cnt > appConfig.Task.PendingAutoTasksLimit {
				time.Sleep(2 * time.Second)
				continue
			}
			queuedTasks, err := relay.GetQueuedTasks(ctx)
			if err != nil {
				log.Errorf("AutoTask: cannot get queued tasks count %v", err)
				time.Sleep(2 * time.Second)
				continue
			}
			log.Infof("AutoTask: queued task count %d", queuedTasks)
			if uint64(queuedTasks) > appConfig.Task.PendingAutoTasksLimit {
				time.Sleep(2 * time.Second)
				continue
			}
			pendingLargeVramLLMTasksCount, err := getPendingLargeVramLLMTasksCount(ctx, client)
			if err != nil {
				log.Errorf("AutoTask: cannot get pending large vram llm tasks count %v", err)
				time.Sleep(2 * time.Second)
				continue
			}

			for i := 0; i < batchSize; i++ {
				task := generateRandomTask(client, pendingLargeVramLLMTasksCount)
				tasks[i] = task
				if task.TaskType == models.TaskTypeLLM && task.MinVram > 24 {
					pendingLargeVramLLMTasksCount += 1
				}
			}
			if err := models.SaveTasks(ctx, config.GetDB(), tasks); err != nil {
				log.Errorf("AutoTask: cannot save auto tasks: %v", err)
				return err
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func AutoCreateTasks(ctx context.Context) {
	ctx1, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		for {
			err := autoCreateTasks(ctx1)
			if err != nil {
				log.Errorf("AutoTask: auto create tasks error: %v", err)
				time.Sleep(5 * time.Second)
			}
		}
	}()
	<-ctx1.Done()
	err := ctx1.Err()
	log.Errorf("AutoTask: timeout %v, finish", err)
}
