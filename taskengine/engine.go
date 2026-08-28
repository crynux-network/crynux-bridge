package taskengine

import (
	"context"
	"crynux_bridge/models"
	"crynux_bridge/tasktrace"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Engine struct {
	db                   *gorm.DB
	config               Config
	createCapacity       chan struct{}
	statusCapacity       chan struct{}
	validationCapacity   chan struct{}
	cancellationCapacity chan struct{}
	resultCapacity       chan struct{}
	startOnce            sync.Once
}

var (
	defaultMu     sync.RWMutex
	defaultEngine *Engine
)

func New(db *gorm.DB, cfg Config) (*Engine, error) {
	if db == nil {
		return nil, errors.New("task engine database must not be nil")
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid task engine config: %w", err)
	}
	return &Engine{
		db:                   db,
		config:               cfg,
		createCapacity:       make(chan struct{}, cfg.CreateWorkers),
		statusCapacity:       make(chan struct{}, cfg.StatusWorkers),
		validationCapacity:   make(chan struct{}, cfg.ValidationWorkers),
		cancellationCapacity: make(chan struct{}, cfg.CancellationWorkers),
		resultCapacity:       make(chan struct{}, cfg.ResultWorkers),
	}, nil
}

func SetDefault(engine *Engine) {
	defaultMu.Lock()
	defaultEngine = engine
	defaultMu.Unlock()
}

func Default() (*Engine, error) {
	defaultMu.RLock()
	engine := defaultEngine
	defaultMu.RUnlock()
	if engine == nil {
		return nil, errors.New("task engine is not initialized")
	}
	return engine, nil
}

func (e *Engine) Start(ctx context.Context) {
	e.startOnce.Do(func() {
		go e.run(ctx)
	})
}

func (e *Engine) CreateTask(
	ctx context.Context,
	clientID string,
	apiKey *models.ClientAPIKey,
	submission Submission,
) (*models.ClientTask, error) {
	repeatNum := e.config.DefaultRepeatNum
	if submission.RepeatNum != nil {
		repeatNum = *submission.RepeatNum
	}
	if repeatNum <= 0 {
		return nil, errors.New("repeat_num must be greater than zero")
	}
	encoded, err := json.Marshal(submission)
	if err != nil {
		return nil, fmt.Errorf("encode task submission: %w", err)
	}
	var clientTask models.ClientTask
	err = e.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if apiKey != nil {
			result := tx.Model(&models.ClientAPIKey{}).
				Where("id = ?", apiKey.ID).
				Where("use_limit <= 0 OR used_count < use_limit").
				Updates(map[string]any{
					"used_count":   gorm.Expr("used_count + ?", 1),
					"last_used_at": time.Now(),
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return models.ErrAPIKeyQuotaExceeded
			}
		}
		client := models.Client{ClientId: clientID}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "client_id"}},
			DoNothing: true,
		}).Create(&client).Error; err != nil {
			return err
		}
		if err := tx.Where("client_id = ?", clientID).First(&client).Error; err != nil {
			return err
		}
		clientTask = models.ClientTask{
			ClientID:           client.ID,
			Submission:         string(encoded),
			SubmissionTaskType: submission.TaskType,
			EffectiveRepeatNum: repeatNum,
			RepeatExpanded:     false,
			NextActionAt:       time.Now(),
		}
		if len(submission.TaskModelIDs) > 0 {
			clientTask.SubmissionModelID = submission.TaskModelIDs[0]
		}
		return tx.Create(&clientTask).Error
	})
	if err != nil {
		return nil, err
	}
	return &clientTask, nil
}

func (e *Engine) Status(ctx context.Context, clientID string, clientTaskID uint) (*models.ClientTask, error) {
	var clientTask models.ClientTask
	err := e.db.WithContext(ctx).
		Model(&models.ClientTask{}).
		Joins("JOIN clients ON clients.id = client_tasks.client_id").
		Where("client_tasks.id = ? AND clients.client_id = ?", clientTaskID, clientID).
		First(&clientTask).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return &clientTask, nil
}

func (e *Engine) Result(ctx context.Context, clientID string, clientTaskID uint) (*Result, error) {
	clientTask, err := e.Status(ctx, clientID, clientTaskID)
	if err != nil {
		return nil, err
	}
	if clientTask.Status == models.ClientTaskStatusRunning {
		return nil, ErrTaskRunning
	}
	if clientTask.Status != models.ClientTaskStatusSuccess {
		return nil, ErrTaskFailed
	}
	var task models.InferenceTask
	if err := e.db.WithContext(ctx).
		Where("client_task_id = ? AND status = ?", clientTaskID, models.InferenceTaskResultDownloaded).
		Order("updated_at ASC, id ASC").
		First(&task).Error; err != nil {
		return nil, err
	}
	return &Result{
		TaskID:           task.ID,
		TaskIDValue:      task.TaskID,
		TaskIDCommitment: task.TaskIDCommitment,
		TaskType:         task.TaskType,
		TaskSize:         task.TaskSize,
		CreatedAt:        task.CreatedAt,
		Directory:        e.config.ResultDirectory,
	}, nil
}

func (e *Engine) CountPending(
	ctx context.Context,
	clientID string,
	taskType models.ChainTaskType,
	modelID string,
) (uint64, error) {
	var inferenceCount int64
	err := e.db.WithContext(ctx).
		Model(&models.InferenceTask{}).
		Joins("JOIN clients ON clients.id = inference_tasks.client_id").
		Where("clients.client_id = ?", clientID).
		Where("inference_tasks.task_type = ?", taskType).
		Where("inference_tasks.task_model_ids = ?", modelID).
		Where("inference_tasks.status NOT IN ?", []models.TaskStatus{
			models.InferenceTaskEndAborted,
			models.InferenceTaskEndGroupRefund,
			models.InferenceTaskEndInvalidated,
			models.InferenceTaskEndSuccess,
			models.InferenceTaskResultDownloaded,
		}).
		Count(&inferenceCount).Error
	if err != nil {
		return 0, err
	}
	var client models.Client
	if err := e.db.WithContext(ctx).Where("client_id = ?", clientID).First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uint64(inferenceCount), nil
		}
		return 0, err
	}
	var unexpandedCount int64
	if err := e.db.WithContext(ctx).
		Model(&models.ClientTask{}).
		Where("client_id = ? AND repeat_expanded = ? AND submission_task_type = ? AND submission_model_id = ?",
			client.ID, false, taskType, modelID).
		Count(&unexpandedCount).Error; err != nil {
		return 0, err
	}
	return uint64(inferenceCount + unexpandedCount), nil
}

func (e *Engine) CountSubmittedSince(ctx context.Context, clientID string, since time.Time) (uint64, error) {
	var count int64
	if err := e.db.WithContext(ctx).
		Model(&models.ClientTask{}).
		Joins("JOIN clients ON clients.id = client_tasks.client_id").
		Where("clients.client_id = ? AND client_tasks.created_at >= ?", clientID, since).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return uint64(count), nil
}

func (e *Engine) RegisterTraceTasks(ctx context.Context, clientTaskID uint) error {
	var tasks []models.InferenceTask
	if err := e.db.WithContext(ctx).
		Where("client_task_id = ? AND task_type IN ?", clientTaskID, []models.ChainTaskType{
			models.TaskTypeSD,
			models.TaskTypeLLM,
		}).
		Order("id ASC").
		Find(&tasks).Error; err != nil {
		return err
	}
	primaries := make([]models.InferenceTask, 0, len(tasks))
	validationMembers := make([]models.InferenceTask, 0)
	firstByTaskID := make(map[string]uint)
	for i := range tasks {
		if firstByTaskID[tasks[i].TaskID] == 0 || tasks[i].ID < firstByTaskID[tasks[i].TaskID] {
			firstByTaskID[tasks[i].TaskID] = tasks[i].ID
		}
	}
	for i := range tasks {
		if firstByTaskID[tasks[i].TaskID] == tasks[i].ID {
			primaries = append(primaries, tasks[i])
		} else {
			validationMembers = append(validationMembers, tasks[i])
		}
	}
	tasktrace.RegisterClientTasks(clientTaskID, primaries, "primary")
	tasktrace.RegisterClientTasks(clientTaskID, validationMembers, "validation")
	return nil
}
