package config

import (
	"testing"
	"time"
)

func validTaskEngineConfig() TaskEngineConfig {
	return TaskEngineConfig{
		ScanInterval:                 time.Second,
		StatusPollInterval:           time.Second,
		ExecutionPollAdvance:         0,
		ExecutionOverrunPollInterval: time.Second,
		RetryInterval:                time.Second,
		OperationTimeout:             time.Second,
		ExpansionBatchSize:           1,
		OperationResultBatchSize:     1,
		DueTaskBatchSize:             1,
		CreateBatchSize:              1,
		StatusBatchSize:              1,
		ValidationBatchSize:          1,
		CancellationBatchSize:        1,
		CreateWorkers:                1,
		StatusWorkers:                1,
		ValidationWorkers:            1,
		CancellationWorkers:          1,
		ResultWorkers:                1,
	}
}

func TestValidateTaskEngineConfigRequiresEveryBound(t *testing.T) {
	if err := validateTaskEngineConfig(validTaskEngineConfig()); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*TaskEngineConfig)
	}{
		{"scan interval", func(c *TaskEngineConfig) { c.ScanInterval = 0 }},
		{"retry interval", func(c *TaskEngineConfig) { c.RetryInterval = 0 }},
		{"create batch", func(c *TaskEngineConfig) { c.CreateBatchSize = 0 }},
		{"status workers", func(c *TaskEngineConfig) { c.StatusWorkers = 0 }},
		{"result workers", func(c *TaskEngineConfig) { c.ResultWorkers = 0 }},
		{"negative poll advance", func(c *TaskEngineConfig) { c.ExecutionPollAdvance = -time.Second }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validTaskEngineConfig()
			tt.mutate(&config)
			if err := validateTaskEngineConfig(config); err == nil {
				t.Fatal("invalid config was accepted")
			}
		})
	}
}
