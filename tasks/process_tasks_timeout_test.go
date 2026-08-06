package tasks

import (
	"crynux_bridge/models"
	"crynux_bridge/relay"
	"errors"
	"testing"
)

func TestRelayCreateRejectionIsPermanent(t *testing.T) {
	rejected := relay.RelayError{StatusCode: 400, ErrorMessage: "Insufficient relay account balance"}
	if !isRelayRequestRejected(rejected) {
		t.Fatal("a 400 relay response must be treated as a permanent rejection")
	}
	if !isRelayRequestRejected(errors.Join(errors.New("wrapped"), rejected)) {
		t.Fatal("a wrapped 400 relay response must be treated as a permanent rejection")
	}
	serverError := relay.RelayError{StatusCode: 500, ErrorMessage: "internal error"}
	if isRelayRequestRejected(serverError) {
		t.Fatal("a 5xx relay response must be retried")
	}
	if isRelayRequestRejected(errors.New("connection refused")) {
		t.Fatal("a transport error must be retried")
	}
}

func TestRelayTaskNotFoundDetection(t *testing.T) {
	notFound := relay.RelayError{StatusCode: 400, ErrorMessage: `"Task not found"`}
	if !isRelayTaskNotFound(notFound) {
		t.Fatal("relay task-not-found response was not detected")
	}
	other := relay.RelayError{StatusCode: 400, ErrorMessage: "timeout must be greater than 0"}
	if isRelayTaskNotFound(other) {
		t.Fatal("other relay errors must not be treated as task-not-found")
	}
}

func TestCreatorValidationTimeoutStopsGroupValidation(t *testing.T) {
	group := []models.InferenceTask{
		{Status: models.InferenceTaskScoreReady},
		{
			Status:      models.InferenceTaskEndAborted,
			AbortReason: models.TaskAbortCreatorValidationTimeout,
		},
		{Status: models.InferenceTaskErrorReported},
	}
	if !hasCreatorValidationTimeout(group) {
		t.Fatal("creator validation timeout was not detected")
	}
	group[1].AbortReason = models.TaskAbortTimeout
	if hasCreatorValidationTimeout(group) {
		t.Fatal("non-validation timeout must not stop group validation")
	}
}

func TestOrdinaryAbortedGroupMemberRemainsEligibleForValidation(t *testing.T) {
	group := []models.InferenceTask{
		{Status: models.InferenceTaskScoreReady},
		{Status: models.InferenceTaskErrorReported},
		{
			Status:      models.InferenceTaskEndAborted,
			AbortReason: models.TaskAbortTimeout,
		},
	}
	if !allTasksEligibleForGroupValidation(group) {
		t.Fatal("ordinary aborted member must remain eligible for group validation")
	}
	group[2].AbortReason = models.TaskAbortCreatorValidationTimeout
	if !allTasksEligibleForGroupValidation(group) {
		t.Fatal("creator-validation timeout is filtered by its abort reason, not status eligibility")
	}
	if !hasCreatorValidationTimeout(group) {
		t.Fatal("creator-validation timeout must separately block group validation")
	}
}

func TestBuildSDFTValidationTasksPreservesTimeout(t *testing.T) {
	source := &models.InferenceTask{
		TaskType: models.TaskTypeSDFTLora,
		Timeout:  7200,
		TaskID:   "task",
	}
	tasks := buildValidationTasks(source, "gpu", 24, "seed", "proof", "number")
	if len(tasks) != 2 {
		t.Fatalf("validation task count = %d, want 2", len(tasks))
	}
	for i, task := range tasks {
		if task.Timeout != source.Timeout {
			t.Fatalf("validation task %d timeout = %d, want %d", i, task.Timeout, source.Timeout)
		}
	}
}

func TestNeedsRelayTaskData(t *testing.T) {
	unpersisted := &models.InferenceTask{
		Status: models.InferenceTaskScoreReady,
	}
	if !needsRelayTaskData(unpersisted) {
		t.Fatal("a ready task without persisted VRF data must still persist relay task data")
	}

	subTask := &models.InferenceTask{
		Status:       models.InferenceTaskCreated,
		SamplingSeed: "seed",
		VRFProof:     "proof",
		VRFNumber:    "number",
	}
	if !needsRelayTaskData(subTask) {
		t.Fatal("a validation sub-task without a persisted sequence must persist relay task data")
	}

	persisted := &models.InferenceTask{
		Status:       models.InferenceTaskScoreReady,
		Sequence:     42,
		SamplingSeed: "seed",
		VRFProof:     "proof",
		VRFNumber:    "number",
	}
	if needsRelayTaskData(persisted) {
		t.Fatal("a task with persisted sequence and VRF data must not repeat the persistence step")
	}
}

func TestValidationReadinessUsesExplicitStatuses(t *testing.T) {
	for _, status := range []models.TaskStatus{
		models.InferenceTaskScoreReady,
		models.InferenceTaskErrorReported,
		models.InferenceTaskValidated,
		models.InferenceTaskEndAborted,
		models.InferenceTaskEndGroupRefund,
		models.InferenceTaskEndInvalidated,
		models.InferenceTaskEndSuccess,
		models.InferenceTaskResultDownloaded,
	} {
		if !isTaskReadyOrLater(status) {
			t.Fatalf("status %d must stop group readiness waiting", status)
		}
	}
	if isTaskReadyOrLater(models.TaskStatus(12)) {
		t.Fatal("reserved status 12 must not be treated as ready or terminal")
	}
}
