package taskengine

import (
	"context"
	"errors"
	"fmt"

	"crynux_bridge/models"
	"crynux_bridge/relay"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	batchGetTaskStatus = relay.BatchGetTaskStatus
	batchAbortTasks    = relay.BatchAbortTasks
)

func (e *Engine) AbortClientTask(ctx context.Context, clientID string, clientTaskID uint) (*models.ClientTask, error) {
	clientTask, members, err := e.loadAbortTarget(ctx, clientID, clientTaskID)
	if err != nil {
		return nil, err
	}
	if clientTask.Status == models.ClientTaskStatusFailed && clientTask.CreatorCancelled {
		ApplyClientTaskPhase(clientTask, members)
		return clientTask, nil
	}
	if clientTask.Status == models.ClientTaskStatusSuccess {
		return nil, ErrAbortNotCancellable
	}
	for i := range members {
		if members[i].Status == models.InferenceTaskResultDownloaded {
			return nil, ErrAbortNotCancellable
		}
	}
	if !clientTask.RepeatExpanded {
		if err := e.markUnexpandedClientTaskCancelled(ctx, clientTask.ID); err != nil {
			return nil, err
		}
		return e.Status(ctx, clientID, clientTaskID)
	}
	if !allTaskIDGroupsLocallyCancellable(members) {
		return nil, ErrAbortNotCancellable
	}

	pendingWithoutCommitment := make([]uint, 0)
	commitments := make([]string, 0)
	commitmentTaskIDs := make(map[string]uint)
	for i := range members {
		task := members[i]
		if task.Finished() {
			continue
		}
		if task.TaskIDCommitment == "" {
			pendingWithoutCommitment = append(pendingWithoutCommitment, task.ID)
			continue
		}
		commitments = append(commitments, task.TaskIDCommitment)
		commitmentTaskIDs[task.TaskIDCommitment] = task.ID
	}

	if len(commitments) > 0 {
		statusItems, statusErr := batchGetTaskStatus(ctx, commitments)
		if statusErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrAbortTemporary, statusErr)
		}
		if err := validateAbortPreflight(commitments, statusItems); err != nil {
			return nil, err
		}
	}

	var abortResults []relay.BatchMutationResult
	if len(commitments) > 0 {
		abortResults, err = batchAbortTasks(ctx, commitments)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrAbortTemporary, err)
		}
	}

	cancelledIDs, abortErr := classifyAbortResults(commitments, commitmentTaskIDs, abortResults)
	clientFailed, applyErr := e.applyCreatorAbortLocalState(ctx, clientTask.ID, pendingWithoutCommitment, cancelledIDs, abortErr == nil)
	if applyErr != nil {
		return nil, applyErr
	}
	if abortErr != nil {
		return nil, abortErr
	}
	if !clientFailed {
		return nil, ErrAbortNotCancellable
	}
	return e.Status(ctx, clientID, clientTaskID)
}

func (e *Engine) AbortClientTaskBatch(ctx context.Context, clientID string, clientTaskIDs []uint) ([]AbortBatchItemResult, error) {
	results := make([]AbortBatchItemResult, 0, len(clientTaskIDs))
	for _, clientTaskID := range clientTaskIDs {
		clientTask, err := e.AbortClientTask(ctx, clientID, clientTaskID)
		item := AbortBatchItemResult{ClientTaskID: clientTaskID}
		if err != nil {
			item.Error = abortErrorString(err)
		} else {
			copied := *clientTask
			item.ClientTask = &copied
		}
		results = append(results, item)
	}
	return results, nil
}

type AbortBatchItemResult struct {
	ClientTaskID uint               `json:"client_task_id"`
	ClientTask   *models.ClientTask `json:"client_task,omitempty"`
	Error        string             `json:"error,omitempty"`
}

func abortErrorString(err error) string {
	switch {
	case errors.Is(err, ErrTaskNotFound):
		return "not_found"
	case errors.Is(err, ErrAbortNotCancellable):
		return "not_cancellable"
	case errors.Is(err, ErrAbortTemporary):
		return "temporary_error"
	default:
		return "temporary_error"
	}
}

func (e *Engine) loadAbortTarget(ctx context.Context, clientID string, clientTaskID uint) (*models.ClientTask, []models.InferenceTask, error) {
	var clientTask models.ClientTask
	err := e.db.WithContext(ctx).
		Model(&models.ClientTask{}).
		Joins("JOIN clients ON clients.id = client_tasks.client_id").
		Where("client_tasks.id = ? AND clients.client_id = ?", clientTaskID, clientID).
		First(&clientTask).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	membersByID, err := e.loadClientTaskMembers(ctx, []uint{clientTask.ID})
	if err != nil {
		return nil, nil, err
	}
	return &clientTask, membersByID[clientTask.ID], nil
}

func (e *Engine) markUnexpandedClientTaskCancelled(ctx context.Context, clientTaskID uint) error {
	return e.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var clientTask models.ClientTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&clientTask, clientTaskID).Error; err != nil {
			return err
		}
		if clientTask.Status == models.ClientTaskStatusFailed && clientTask.CreatorCancelled {
			return nil
		}
		if clientTask.Status != models.ClientTaskStatusRunning || clientTask.RepeatExpanded {
			return ErrAbortNotCancellable
		}
		return tx.Model(&models.ClientTask{}).Where("id = ?", clientTaskID).Updates(map[string]any{
			"status":            models.ClientTaskStatusFailed,
			"creator_cancelled": true,
			"failed_count":      0,
		}).Error
	})
}

func memberLocallyBlocksCreatorAbort(task models.InferenceTask) bool {
	if task.Status == models.InferenceTaskResultDownloaded {
		return true
	}
	if task.Finished() {
		return false
	}
	switch task.Status {
	case models.InferenceTaskPending, models.InferenceTaskCreated:
		return false
	default:
		return true
	}
}

func allTaskIDGroupsLocallyCancellable(members []models.InferenceTask) bool {
	groups := make(map[string][]models.InferenceTask)
	for i := range members {
		groups[members[i].TaskID] = append(groups[members[i].TaskID], members[i])
	}
	for _, group := range groups {
		for i := range group {
			if memberLocallyBlocksCreatorAbort(group[i]) {
				return false
			}
		}
	}
	return true
}

func validateAbortPreflight(commitments []string, items []relay.BatchStatusItem) error {
	byCommitment := make(map[string]relay.BatchStatusItem, len(items))
	for _, item := range items {
		byCommitment[item.TaskIDCommitment] = item
	}
	for _, commitment := range commitments {
		item, ok := byCommitment[commitment]
		if !ok || !item.Found {
			return ErrAbortNotCancellable
		}
		if item.Status == models.ChainTaskQueued {
			continue
		}
		if item.Status == models.ChainTaskEndAborted && item.AbortReason == models.TaskAbortCreatorCancelled {
			continue
		}
		return ErrAbortNotCancellable
	}
	return nil
}

func classifyAbortResults(
	commitments []string,
	commitmentTaskIDs map[string]uint,
	results []relay.BatchMutationResult,
) ([]uint, error) {
	byCommitment := make(map[string]relay.BatchMutationResult, len(results))
	for _, result := range results {
		if result.TaskIDCommitment != "" {
			byCommitment[result.TaskIDCommitment] = result
			continue
		}
		for _, commitment := range result.TaskIDCommitments {
			byCommitment[commitment] = result
		}
	}
	cancelledIDs := make([]uint, 0, len(commitments))
	var sawNotCancellable bool
	var sawTemporary bool
	for _, commitment := range commitments {
		result, ok := byCommitment[commitment]
		if !ok {
			sawTemporary = true
			continue
		}
		switch result.Outcome {
		case relay.BatchOutcomeCancelled, relay.BatchOutcomeAlreadyCancelled:
			if taskID, exists := commitmentTaskIDs[commitment]; exists {
				cancelledIDs = append(cancelledIDs, taskID)
			}
		case relay.BatchOutcomeNotCancellable, relay.BatchOutcomeNotFound, relay.BatchOutcomePermanentError:
			sawNotCancellable = true
		default:
			sawTemporary = true
		}
	}
	if sawTemporary {
		return cancelledIDs, ErrAbortTemporary
	}
	if sawNotCancellable {
		return cancelledIDs, ErrAbortNotCancellable
	}
	return cancelledIDs, nil
}

func (e *Engine) applyCreatorAbortLocalState(
	ctx context.Context,
	clientTaskID uint,
	pendingWithoutCommitment []uint,
	cancelledIDs []uint,
	attemptFullSuccess bool,
) (clientTaskFailed bool, err error) {
	err = e.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var clientTask models.ClientTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&clientTask, clientTaskID).Error; err != nil {
			return err
		}
		if clientTask.Status == models.ClientTaskStatusSuccess {
			return ErrAbortNotCancellable
		}
		var members []models.InferenceTask
		if err := tx.Where(
			"client_task_id = ? AND task_type IN ?",
			clientTaskID,
			[]models.ChainTaskType{models.TaskTypeSD, models.TaskTypeLLM},
		).Find(&members).Error; err != nil {
			return err
		}
		for i := range members {
			if members[i].Status == models.InferenceTaskResultDownloaded {
				return ErrAbortNotCancellable
			}
		}

		idsToAbort := make(map[uint]struct{}, len(pendingWithoutCommitment)+len(cancelledIDs))
		if attemptFullSuccess {
			for _, id := range pendingWithoutCommitment {
				idsToAbort[id] = struct{}{}
			}
		}
		for _, id := range cancelledIDs {
			idsToAbort[id] = struct{}{}
		}
		for id := range idsToAbort {
			if err := tx.Model(&models.InferenceTask{}).Where("id = ?", id).Updates(map[string]any{
				"status":                        models.InferenceTaskEndAborted,
				"abort_reason":                  models.TaskAbortCreatorCancelled,
				"sibling_cancellation_required": false,
				"operation_type":                models.TaskOperationNone,
				"operation_status":              models.TaskOperationStatusNone,
				"operation_error":               "",
				"operation_result":              "",
			}).Error; err != nil {
				return err
			}
		}

		if err := tx.Where(
			"client_task_id = ? AND task_type IN ?",
			clientTaskID,
			[]models.ChainTaskType{models.TaskTypeSD, models.TaskTypeLLM},
		).Find(&members).Error; err != nil {
			return err
		}
		allStoppedWithoutSuccess := len(members) > 0
		for i := range members {
			if members[i].Status == models.InferenceTaskResultDownloaded || !members[i].Finished() {
				allStoppedWithoutSuccess = false
				break
			}
		}
		if attemptFullSuccess && allStoppedWithoutSuccess {
			if err := tx.Model(&models.ClientTask{}).Where("id = ?", clientTaskID).Updates(map[string]any{
				"status":            models.ClientTaskStatusFailed,
				"creator_cancelled": true,
			}).Error; err != nil {
				return err
			}
			clientTaskFailed = true
			return nil
		}
		return e.recalculateClientTask(ctx, tx, clientTaskID)
	})
	return clientTaskFailed, err
}
