package taskengine

import (
	"crynux_bridge/models"
	"errors"
	"fmt"
	"time"
)

type Submission struct {
	TaskArgs        string               `json:"task_args"`
	TaskType        models.ChainTaskType `json:"task_type"`
	TaskModelIDs    []string             `json:"task_model_ids"`
	TaskVersion     string               `json:"task_version"`
	TaskFee         string               `json:"task_fee"`
	MinVram         uint64               `json:"min_vram"`
	RequiredGPU     string               `json:"required_gpu"`
	RequiredGPUVram uint64               `json:"required_gpu_vram"`
	TaskSize        uint64               `json:"task_size"`
	Timeout         uint64               `json:"timeout"`
	RepeatNum       *int                 `json:"repeat_num,omitempty"`
}

type Config struct {
	DefaultRepeatNum             int
	ScanInterval                 time.Duration
	StatusPollInterval           time.Duration
	ExecutionPollAdvance         time.Duration
	ExecutionOverrunPollInterval time.Duration
	RetryInterval                time.Duration
	OperationTimeout             time.Duration
	ExpansionBatchSize           int
	OperationResultBatchSize     int
	DueTaskBatchSize             int
	CreateBatchSize              int
	StatusBatchSize              int
	ValidationBatchSize          int
	CancellationBatchSize        int
	CreateWorkers                int
	StatusWorkers                int
	ValidationWorkers            int
	CancellationWorkers          int
	ResultWorkers                int
	ResultDirectory              string
	PrivateKey                   string
}

func (c Config) validate() error {
	if c.DefaultRepeatNum <= 0 {
		return errors.New("default repeat number must be greater than zero")
	}
	positiveDurations := []struct {
		name  string
		value time.Duration
	}{
		{"scan interval", c.ScanInterval},
		{"status poll interval", c.StatusPollInterval},
		{"execution overrun poll interval", c.ExecutionOverrunPollInterval},
		{"retry interval", c.RetryInterval},
		{"operation timeout", c.OperationTimeout},
	}
	for _, field := range positiveDurations {
		if field.value <= 0 {
			return fmt.Errorf("%s must be greater than zero", field.name)
		}
	}
	if c.ExecutionPollAdvance < 0 {
		return errors.New("execution poll advance must not be negative")
	}
	positiveIntegers := []struct {
		name  string
		value int
	}{
		{"expansion batch size", c.ExpansionBatchSize},
		{"operation result batch size", c.OperationResultBatchSize},
		{"due task batch size", c.DueTaskBatchSize},
		{"create batch size", c.CreateBatchSize},
		{"status batch size", c.StatusBatchSize},
		{"validation batch size", c.ValidationBatchSize},
		{"cancellation batch size", c.CancellationBatchSize},
		{"create workers", c.CreateWorkers},
		{"status workers", c.StatusWorkers},
		{"validation workers", c.ValidationWorkers},
		{"cancellation workers", c.CancellationWorkers},
		{"result workers", c.ResultWorkers},
	}
	for _, field := range positiveIntegers {
		if field.value <= 0 {
			return fmt.Errorf("%s must be greater than zero", field.name)
		}
	}
	if c.ResultDirectory == "" {
		return errors.New("result directory must not be empty")
	}
	if c.PrivateKey == "" {
		return errors.New("private key must not be empty")
	}
	return nil
}

var (
	ErrTaskNotFound = errors.New("client task not found")
	ErrTaskRunning  = errors.New("client task is still running")
	ErrTaskFailed   = errors.New("client task failed")
)

type Result struct {
	TaskID           uint
	TaskIDValue      string
	TaskIDCommitment string
	TaskType         models.ChainTaskType
	TaskSize         uint64
	CreatedAt        time.Time
	Directory        string
}
