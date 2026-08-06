package inference_tasks

import (
	"crynux_bridge/config"
	"crynux_bridge/models"
	"testing"
)

func TestResolveTaskTimeoutNormalTasks(t *testing.T) {
	appConfig := &config.AppConfig{}
	for _, taskType := range []models.ChainTaskType{models.TaskTypeSD, models.TaskTypeLLM} {
		timeout, err := resolveTaskTimeout(taskType, nil, appConfig)
		if err != nil {
			t.Fatalf("task type %d: %v", taskType, err)
		}
		if timeout != 0 {
			t.Fatalf("task type %d timeout = %d, want 0", taskType, timeout)
		}
		value := uint64(10)
		if _, err := resolveTaskTimeout(taskType, &value, appConfig); err == nil {
			t.Fatalf("task type %d accepted a direct timeout", taskType)
		}
	}
}

func TestResolveTaskTimeoutSDFT(t *testing.T) {
	appConfig := &config.AppConfig{}
	appConfig.Task.SDFinetuneTimeout = 120

	timeout, err := resolveTaskTimeout(models.TaskTypeSDFTLora, nil, appConfig)
	if err != nil {
		t.Fatal(err)
	}
	if timeout != 120*60 {
		t.Fatalf("default SDFT timeout = %d, want %d", timeout, 120*60)
	}

	requested := uint64(321)
	timeout, err = resolveTaskTimeout(models.TaskTypeSDFTLora, &requested, appConfig)
	if err != nil {
		t.Fatal(err)
	}
	if timeout != requested {
		t.Fatalf("requested SDFT timeout = %d, want %d", timeout, requested)
	}

	zero := uint64(0)
	if _, err := resolveTaskTimeout(models.TaskTypeSDFTLora, &zero, appConfig); err == nil {
		t.Fatal("explicit zero SDFT timeout was accepted")
	}

	appConfig.Task.SDFinetuneTimeout = 0
	if _, err := resolveTaskTimeout(models.TaskTypeSDFTLora, nil, appConfig); err == nil {
		t.Fatal("zero default SDFT timeout was accepted")
	}
}
