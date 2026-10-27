package taskengine

import (
	"crynux_bridge/models"
	"time"
)

const (
	ClientTaskPhaseQueued     = "queued"
	ClientTaskPhaseRunning    = "running"
	ClientTaskPhaseValidating = "validating"
	ClientTaskPhaseUploading  = "uploading"
	ClientTaskPhaseFinalizing = "finalizing"
	ClientTaskPhaseSuccess    = "success"
	ClientTaskPhaseFailed     = "failed"
)

type clientTaskPhaseInfo struct {
	Phase                 string
	EstimatedCompletionAt *time.Time
	ErrorMessage          string
}

func phaseRank(phase string) int {
	switch phase {
	case ClientTaskPhaseQueued:
		return 1
	case ClientTaskPhaseRunning:
		return 2
	case ClientTaskPhaseValidating:
		return 3
	case ClientTaskPhaseUploading:
		return 4
	case ClientTaskPhaseFinalizing:
		return 5
	case ClientTaskPhaseSuccess:
		return 6
	case ClientTaskPhaseFailed:
		return 0
	default:
		return 0
	}
}

func phaseFromInferenceStatus(status models.TaskStatus) string {
	switch status {
	case models.InferenceTaskPending, models.InferenceTaskCreated:
		return ClientTaskPhaseQueued
	case models.InferenceTaskStarted, models.InferenceTaskParamsUploaded:
		return ClientTaskPhaseRunning
	case models.InferenceTaskScoreReady, models.InferenceTaskErrorReported:
		return ClientTaskPhaseValidating
	case models.InferenceTaskValidated:
		return ClientTaskPhaseUploading
	case models.InferenceTaskEndSuccess:
		return ClientTaskPhaseFinalizing
	case models.InferenceTaskResultDownloaded:
		return ClientTaskPhaseSuccess
	case models.InferenceTaskEndAborted, models.InferenceTaskEndInvalidated, models.InferenceTaskEndGroupRefund:
		return ClientTaskPhaseFailed
	default:
		return ClientTaskPhaseQueued
	}
}

func abortReasonMessage(reason models.TaskAbortReason) string {
	switch reason {
	case models.TaskAbortTimeout:
		return "timeout"
	case models.TaskAbortModelDownloadFailed:
		return "model_download_failed"
	case models.TaskAbortIncorrectResult:
		return "incorrect_result"
	case models.TaskAbortTaskFeeTooLow:
		return "task_fee_too_low"
	case models.TaskAbortGroupTimeout:
		return "group_timeout"
	case models.TaskAbortErrorReported:
		return "error_reported"
	case models.TaskAbortCreatorCancelled:
		return "creator_cancelled"
	case models.TaskAbortCreatorValidationTimeout:
		return "creator_validation_timeout"
	case models.TaskAbortResultUploadTimeout:
		return "result_upload_timeout"
	case models.TaskAbortNodeSlashed:
		return "node_slashed"
	default:
		return "failed"
	}
}

func taskErrorMessage(task models.InferenceTask) string {
	if task.Status == models.InferenceTaskEndInvalidated {
		return "incorrect_result"
	}
	if task.Status == models.InferenceTaskEndGroupRefund {
		return "group_refund"
	}
	if task.AbortReason != models.TaskAbortReasonNone {
		return abortReasonMessage(task.AbortReason)
	}
	if task.TaskError == models.TaskErrorParametersValidationFailed {
		return "parameters_validation_failed"
	}
	return "failed"
}

func earlierTask(a, b *models.InferenceTask) *models.InferenceTask {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if b.UpdatedAt.Before(a.UpdatedAt) || (b.UpdatedAt.Equal(a.UpdatedAt) && b.ID < a.ID) {
		return b
	}
	return a
}

func computeClientTaskPhase(clientTask models.ClientTask, tasks []models.InferenceTask) clientTaskPhaseInfo {
	if clientTask.Status == models.ClientTaskStatusSuccess {
		return clientTaskPhaseInfo{Phase: ClientTaskPhaseSuccess}
	}

	var downloaded *models.InferenceTask
	var unfinished []*models.InferenceTask
	var failed *models.InferenceTask
	for i := range tasks {
		task := &tasks[i]
		if task.Status == models.InferenceTaskResultDownloaded {
			downloaded = earlierTask(downloaded, task)
			continue
		}
		if !task.Finished() {
			unfinished = append(unfinished, task)
			continue
		}
		if task.Status == models.InferenceTaskEndAborted ||
			task.Status == models.InferenceTaskEndInvalidated ||
			task.Status == models.InferenceTaskEndGroupRefund {
			failed = earlierTask(failed, task)
		}
	}

	if downloaded != nil || clientTask.Status == models.ClientTaskStatusSuccess {
		return clientTaskPhaseInfo{Phase: ClientTaskPhaseSuccess}
	}

	if len(unfinished) == 0 {
		if clientTask.Status == models.ClientTaskStatusFailed || failed != nil || len(tasks) > 0 {
			info := clientTaskPhaseInfo{Phase: ClientTaskPhaseFailed, ErrorMessage: "failed"}
			if clientTask.CreatorCancelled {
				info.ErrorMessage = ClientTaskAbortErrorMessage
			} else if failed != nil {
				info.ErrorMessage = taskErrorMessage(*failed)
			}
			return info
		}
		return clientTaskPhaseInfo{Phase: ClientTaskPhaseQueued}
	}

	var leading *models.InferenceTask
	leadingPhase := ""
	for _, task := range unfinished {
		phase := phaseFromInferenceStatus(task.Status)
		if leading == nil ||
			phaseRank(phase) > phaseRank(leadingPhase) ||
			(phase == leadingPhase &&
				(task.UpdatedAt.Before(leading.UpdatedAt) ||
					(task.UpdatedAt.Equal(leading.UpdatedAt) && task.ID < leading.ID))) {
			leading = task
			leadingPhase = phase
		}
	}
	if leading == nil {
		return clientTaskPhaseInfo{Phase: ClientTaskPhaseQueued}
	}

	info := clientTaskPhaseInfo{Phase: leadingPhase}
	if leadingPhase == ClientTaskPhaseRunning && leading.EstimatedCompletionAt != nil {
		info.EstimatedCompletionAt = leading.EstimatedCompletionAt
	}
	return info
}

func ApplyClientTaskPhase(clientTask *models.ClientTask, tasks []models.InferenceTask) {
	info := computeClientTaskPhase(*clientTask, tasks)
	clientTask.Phase = info.Phase
	clientTask.EstimatedCompletionAt = info.EstimatedCompletionAt
	clientTask.ErrorMessage = info.ErrorMessage
}
