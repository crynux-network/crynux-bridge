package taskengine

import (
	"crynux_bridge/models"
	"testing"
	"time"
)

func TestComputeClientTaskPhaseQueuedWithoutMembers(t *testing.T) {
	info := computeClientTaskPhase(models.ClientTask{Status: models.ClientTaskStatusRunning}, nil)
	if info.Phase != ClientTaskPhaseQueued {
		t.Fatalf("phase = %s, want queued", info.Phase)
	}
}

func TestComputeClientTaskPhaseSingleTaskProgression(t *testing.T) {
	cases := []struct {
		status models.TaskStatus
		phase  string
	}{
		{models.InferenceTaskCreated, ClientTaskPhaseQueued},
		{models.InferenceTaskStarted, ClientTaskPhaseRunning},
		{models.InferenceTaskScoreReady, ClientTaskPhaseValidating},
		{models.InferenceTaskValidated, ClientTaskPhaseUploading},
		{models.InferenceTaskEndSuccess, ClientTaskPhaseFinalizing},
		{models.InferenceTaskResultDownloaded, ClientTaskPhaseSuccess},
	}
	for _, tc := range cases {
		info := computeClientTaskPhase(
			models.ClientTask{Status: models.ClientTaskStatusRunning},
			[]models.InferenceTask{{Status: tc.status}},
		)
		if info.Phase != tc.phase {
			t.Fatalf("status %d phase = %s, want %s", tc.status, info.Phase, tc.phase)
		}
	}
}

func TestComputeClientTaskPhaseRunningIncludesEstimatedCompletion(t *testing.T) {
	eta := time.Unix(1_700_000_000, 0).UTC()
	info := computeClientTaskPhase(
		models.ClientTask{Status: models.ClientTaskStatusRunning},
		[]models.InferenceTask{{
			Status:                models.InferenceTaskStarted,
			EstimatedCompletionAt: &eta,
		}},
	)
	if info.Phase != ClientTaskPhaseRunning {
		t.Fatalf("phase = %s, want running", info.Phase)
	}
	if info.EstimatedCompletionAt == nil || !info.EstimatedCompletionAt.Equal(eta) {
		t.Fatalf("estimated_completion_at = %v, want %v", info.EstimatedCompletionAt, eta)
	}
}

func TestComputeClientTaskPhaseRepeatPicksFurthest(t *testing.T) {
	info := computeClientTaskPhase(
		models.ClientTask{Status: models.ClientTaskStatusRunning},
		[]models.InferenceTask{
			{RootModel: models.RootModel{ID: 1}, Status: models.InferenceTaskCreated},
			{RootModel: models.RootModel{ID: 2}, Status: models.InferenceTaskValidated},
			{RootModel: models.RootModel{ID: 3}, Status: models.InferenceTaskStarted},
		},
	)
	if info.Phase != ClientTaskPhaseUploading {
		t.Fatalf("phase = %s, want uploading", info.Phase)
	}
}

func TestComputeClientTaskPhaseVSSMapsToValidating(t *testing.T) {
	info := computeClientTaskPhase(
		models.ClientTask{Status: models.ClientTaskStatusRunning},
		[]models.InferenceTask{
			{RootModel: models.RootModel{ID: 1}, TaskID: "vss", Status: models.InferenceTaskScoreReady},
			{RootModel: models.RootModel{ID: 2}, TaskID: "vss", Status: models.InferenceTaskStarted},
			{RootModel: models.RootModel{ID: 3}, TaskID: "vss", Status: models.InferenceTaskCreated},
		},
	)
	if info.Phase != ClientTaskPhaseValidating {
		t.Fatalf("phase = %s, want validating", info.Phase)
	}
}

func TestComputeClientTaskPhaseFailedUsesAbortReason(t *testing.T) {
	info := computeClientTaskPhase(
		models.ClientTask{Status: models.ClientTaskStatusFailed},
		[]models.InferenceTask{{
			Status:      models.InferenceTaskEndAborted,
			AbortReason: models.TaskAbortTimeout,
		}},
	)
	if info.Phase != ClientTaskPhaseFailed {
		t.Fatalf("phase = %s, want failed", info.Phase)
	}
	if info.ErrorMessage != "timeout" {
		t.Fatalf("error_message = %s, want timeout", info.ErrorMessage)
	}
}

func TestComputeClientTaskPhaseSuccessFromDownloaded(t *testing.T) {
	info := computeClientTaskPhase(
		models.ClientTask{Status: models.ClientTaskStatusRunning},
		[]models.InferenceTask{
			{RootModel: models.RootModel{ID: 1}, Status: models.InferenceTaskResultDownloaded},
			{RootModel: models.RootModel{ID: 2}, Status: models.InferenceTaskStarted},
		},
	)
	if info.Phase != ClientTaskPhaseSuccess {
		t.Fatalf("phase = %s, want success", info.Phase)
	}
}

func TestApplyClientTaskPhaseWritesFields(t *testing.T) {
	eta := time.Unix(1_700_000_100, 0).UTC()
	clientTask := models.ClientTask{Status: models.ClientTaskStatusRunning}
	ApplyClientTaskPhase(&clientTask, []models.InferenceTask{{
		Status:                models.InferenceTaskStarted,
		EstimatedCompletionAt: &eta,
	}})
	if clientTask.Phase != ClientTaskPhaseRunning {
		t.Fatalf("phase = %s, want running", clientTask.Phase)
	}
	if clientTask.EstimatedCompletionAt == nil || !clientTask.EstimatedCompletionAt.Equal(eta) {
		t.Fatalf("estimated_completion_at = %v, want %v", clientTask.EstimatedCompletionAt, eta)
	}
}
