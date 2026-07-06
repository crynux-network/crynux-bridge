package tasktrace

import (
	"crynux_bridge/models"
	"errors"
	"testing"
	"time"
)

func TestStoreKeepsMostRecentTraces(t *testing.T) {
	ResetForTest()

	taskType := models.TaskTypeLLM
	for i := 0; i < 3; i++ {
		task := models.InferenceTask{
			RootModel: models.RootModel{ID: uint(i + 1), CreatedAt: time.Unix(int64(i), 0)},
			TaskType:  taskType,
			TaskID:    "0xtask",
		}
		task.TaskIDCommitment = string(rune('a' + i))
		StartTrace(StartTraceInput{
			Source:      SourceOpenAICompletions,
			Endpoint:    "/v1/llm/completions",
			ClientID:    "client",
			TaskType:    &taskType,
			RequestTime: time.Unix(int64(i), 0),
			Tasks:       []models.InferenceTask{task},
		}, 2)
	}

	if traces := ListTraces(10, false); len(traces) != 2 {
		t.Fatalf("expected 2 retained traces, got %d", len(traces))
	}
	if _, ok := GetTrace("a"); ok {
		t.Fatal("expected oldest trace to be evicted")
	}
}

func TestStoreIndexesRelatedTasksAndRecordsEvents(t *testing.T) {
	ResetForTest()

	taskType := models.TaskTypeLLM
	primaryTask := models.InferenceTask{
		RootModel:        models.RootModel{ID: 1, CreatedAt: time.Unix(1, 0)},
		ClientTaskID:     10,
		TaskType:         taskType,
		Status:           models.InferenceTaskCreated,
		TaskID:           "0xshared",
		TaskIDCommitment: "0xprimary",
	}
	primary := StartTrace(StartTraceInput{
		Source:      SourceOpenAIChatCompletions,
		Endpoint:    "/v1/llm/chat/completions",
		ClientID:    "client",
		TaskType:    &taskType,
		Request:     map[string]any{"model": "test"},
		RequestTime: time.Unix(1, 0),
		Tasks:       []models.InferenceTask{primaryTask},
	}, 10)

	validationTask := models.InferenceTask{
		RootModel:        models.RootModel{ID: 2, CreatedAt: time.Unix(2, 0)},
		ClientTaskID:     10,
		TaskType:         taskType,
		Status:           models.InferenceTaskPending,
		TaskID:           "0xshared",
		TaskIDCommitment: "0xvalidation",
		SamplingSeed:     "0xseed",
	}
	RegisterTasks(&primaryTask, []models.InferenceTask{validationTask}, "validation")
	validationTask.Status = models.InferenceTaskResultDownloaded
	RecordEvent(&validationTask, "result_downloaded", map[string]any{"file_count": 1})
	FinishTrace(primary, map[string]any{"id": "0xvalidation"}, errors.New("sample error"), &validationTask)

	trace, ok := GetTrace("0xvalidation")
	if !ok {
		t.Fatal("expected trace lookup by validation task commitment")
	}
	if trace.PrimaryTaskIDCommitment != "0xprimary" {
		t.Fatalf("unexpected primary commitment %s", trace.PrimaryTaskIDCommitment)
	}
	if trace.FinalTaskIDCommitment != "0xvalidation" {
		t.Fatalf("unexpected final commitment %s", trace.FinalTaskIDCommitment)
	}
	if trace.Error != "sample error" {
		t.Fatalf("unexpected trace error %q", trace.Error)
	}
	if len(trace.ValidationGroups) != 1 {
		t.Fatalf("expected one validation group, got %d", len(trace.ValidationGroups))
	}
	if len(trace.ParallelTasks) != 2 {
		t.Fatalf("expected two tasks, got %d", len(trace.ParallelTasks))
	}
	if len(trace.ParallelTasks[1].Events) != 1 {
		t.Fatalf("expected one event on validation task, got %d", len(trace.ParallelTasks[1].Events))
	}
}
