package taskengine

import (
	"context"
	"crynux_bridge/models"
	"crynux_bridge/relay"
	"crynux_bridge/utils"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type operationDecision struct {
	task      models.InferenceTask
	operation models.TaskOperationType
	group     []models.InferenceTask
}

func (e *Engine) dispatchDueTasks(ctx context.Context) error {
	if e.allWorkerPoolsFull() {
		return nil
	}
	tasks, err := e.loadDueTasks(ctx)
	if err != nil {
		return err
	}
	decisions := make([]operationDecision, 0, len(tasks))
	for i := range tasks {
		if tasks[i].TaskIDCommitment == "" {
			nonce, commitment := models.GenerateTaskIDCommitment(tasks[i].TaskID)
			if err := e.db.WithContext(ctx).Model(&models.InferenceTask{}).Where("id = ?", tasks[i].ID).Updates(map[string]any{
				"nonce":              nonce,
				"task_id_commitment": commitment,
			}).Error; err != nil {
				return err
			}
			tasks[i].Nonce = nonce
			tasks[i].TaskIDCommitment = commitment
		}
		decision, err := e.decideOperation(ctx, tasks[i])
		if err != nil {
			return err
		}
		if decision.operation != models.TaskOperationNone {
			decisions = append(decisions, decision)
		}
	}
	return e.dispatchDecisions(ctx, decisions)
}

func (e *Engine) loadDueTasks(ctx context.Context) ([]models.InferenceTask, error) {
	base := func() *gorm.DB {
		return e.db.WithContext(ctx).
			Where("operation_status = ? AND next_action_at <= ? AND task_type IN ?",
				models.TaskOperationStatusNone,
				time.Now(),
				[]models.ChainTaskType{models.TaskTypeSD, models.TaskTypeLLM})
	}
	queries := []func(*gorm.DB) *gorm.DB{
		func(query *gorm.DB) *gorm.DB {
			return query.Where("sibling_cancellation_required = ?", true)
		},
		func(query *gorm.DB) *gorm.DB {
			return query.Where("status = ?", models.InferenceTaskEndSuccess)
		},
		func(query *gorm.DB) *gorm.DB {
			return query.Where("status IN ?", []models.TaskStatus{
				models.InferenceTaskScoreReady,
				models.InferenceTaskErrorReported,
			})
		},
		func(query *gorm.DB) *gorm.DB {
			return query.Where(
				"status = ? AND relay_checked = ? AND relay_found = ? AND operation_unknown = ?",
				models.InferenceTaskPending, true, false, false,
			)
		},
		func(query *gorm.DB) *gorm.DB {
			return query.Where(
				"operation_unknown = ? OR relay_checked = ? OR relay_found = ? OR status IN ?",
				true,
				false,
				true,
				[]models.TaskStatus{
					models.InferenceTaskCreated,
					models.InferenceTaskStarted,
					models.InferenceTaskParamsUploaded,
					models.InferenceTaskValidated,
				},
			).Where("status NOT IN ?", terminalStatuses())
		},
	}
	byID := make(map[uint]models.InferenceTask)
	order := make([]uint, 0)
	for _, apply := range queries {
		var batch []models.InferenceTask
		if err := apply(base()).
			Order("next_action_at ASC, id ASC").
			Limit(e.config.DueTaskBatchSize).
			Find(&batch).Error; err != nil {
			return nil, err
		}
		for i := range batch {
			if _, exists := byID[batch[i].ID]; exists {
				continue
			}
			byID[batch[i].ID] = batch[i]
			order = append(order, batch[i].ID)
		}
	}
	tasks := make([]models.InferenceTask, 0, len(order))
	for _, id := range order {
		tasks = append(tasks, byID[id])
	}
	return tasks, nil
}

func (e *Engine) decideOperation(ctx context.Context, task models.InferenceTask) (operationDecision, error) {
	decision := operationDecision{task: task}
	if task.OperationUnknown || !task.RelayChecked {
		decision.operation = models.TaskOperationSyncStatus
		return decision, nil
	}
	if task.SiblingCancellationRequired {
		decision.operation = models.TaskOperationCancel
		return decision, nil
	}
	if task.Status == models.InferenceTaskEndSuccess {
		decision.operation = models.TaskOperationResult
		return decision, nil
	}
	if task.Finished() {
		return decision, nil
	}
	if task.Status == models.InferenceTaskPending {
		if task.RelayFound {
			decision.operation = models.TaskOperationSyncStatus
		} else {
			decision.operation = models.TaskOperationCreate
		}
		return decision, nil
	}
	if !isReadyForValidation(task.Status) || task.ValidationSubmitted {
		decision.operation = models.TaskOperationSyncStatus
		return decision, nil
	}
	var group []models.InferenceTask
	if err := e.db.WithContext(ctx).
		Where("task_id = ?", task.TaskID).
		Order("sequence ASC, id ASC").
		Find(&group).Error; err != nil {
		return decision, err
	}
	if len(group) == 1 {
		decision.operation = models.TaskOperationValidate
		decision.group = group
		return decision, nil
	}
	if !validationGroupReady(group) {
		decision.operation = models.TaskOperationSyncStatus
		return decision, nil
	}
	var ownerID uint
	for i := range group {
		if isReadyForValidation(group[i].Status) {
			ownerID = group[i].ID
			break
		}
	}
	if ownerID == 0 {
		return decision, fmt.Errorf("validation-ready group %s has no eligible owner", task.TaskID)
	}
	if ownerID != task.ID {
		decision.operation = models.TaskOperationSyncStatus
		return decision, nil
	}
	decision.operation = models.TaskOperationValidate
	decision.group = group
	return decision, nil
}

func (e *Engine) dispatchDecisions(
	ctx context.Context,
	decisions []operationDecision,
) error {
	byType := make(map[models.TaskOperationType][]operationDecision)
	for _, decision := range decisions {
		byType[decision.operation] = append(byType[decision.operation], decision)
	}
	batches := []struct {
		operation models.TaskOperationType
		size      int
	}{
		{models.TaskOperationCreate, e.config.CreateBatchSize},
		{models.TaskOperationSyncStatus, e.config.StatusBatchSize},
		{models.TaskOperationValidate, e.config.ValidationBatchSize},
		{models.TaskOperationCancel, e.config.CancellationBatchSize},
	}
	for _, item := range batches {
		values := byType[item.operation]
		capacity := e.capacityFor(item.operation)
		available := cap(capacity) - len(capacity)
		for len(values) > 0 && available > 0 {
			count := item.size
			if count > len(values) {
				count = len(values)
			}
			batch := append([]operationDecision(nil), values[:count]...)
			values = values[count:]
			capacity <- struct{}{}
			if err := e.markRunning(ctx, batch); err != nil {
				<-capacity
				return err
			}
			go e.runRequestWorker(ctx, item.operation, batch)
			available--
		}
	}
	resultSlots := cap(e.resultCapacity) - len(e.resultCapacity)
	for _, decision := range byType[models.TaskOperationResult] {
		if resultSlots == 0 {
			break
		}
		batch := []operationDecision{decision}
		e.resultCapacity <- struct{}{}
		if err := e.markRunning(ctx, batch); err != nil {
			<-e.resultCapacity
			return err
		}
		go e.runResultWorker(ctx, decision)
		resultSlots--
	}
	return nil
}

func (e *Engine) capacityFor(operation models.TaskOperationType) chan struct{} {
	switch operation {
	case models.TaskOperationCreate:
		return e.createCapacity
	case models.TaskOperationSyncStatus:
		return e.statusCapacity
	case models.TaskOperationValidate:
		return e.validationCapacity
	case models.TaskOperationCancel:
		return e.cancellationCapacity
	default:
		panic(fmt.Sprintf("operation %q has no request worker pool", operation))
	}
}

func (e *Engine) allWorkerPoolsFull() bool {
	for _, capacity := range []chan struct{}{
		e.createCapacity,
		e.statusCapacity,
		e.validationCapacity,
		e.cancellationCapacity,
		e.resultCapacity,
	} {
		if len(capacity) < cap(capacity) {
			return false
		}
	}
	return true
}

func (e *Engine) markRunning(ctx context.Context, batch []operationDecision) error {
	now := time.Now()
	return e.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, decision := range batch {
			result := tx.Model(&models.InferenceTask{}).
				Where("id = ? AND operation_status = ?", decision.task.ID, models.TaskOperationStatusNone).
				Updates(map[string]any{
					"operation_type":       decision.operation,
					"operation_status":     models.TaskOperationStatusRunning,
					"operation_started_at": now,
					"operation_result":     "",
					"operation_error":      "",
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("task %d already has an operation", decision.task.ID)
			}
		}
		return nil
	})
}

func (e *Engine) runRequestWorker(parent context.Context, operation models.TaskOperationType, batch []operationDecision) {
	defer func() { <-e.capacityFor(operation) }()
	ctx, cancel := context.WithTimeout(parent, e.config.OperationTimeout)
	defer cancel()
	completed := false
	defer func() {
		if recovered := recover(); recovered != nil {
			e.writeBatchFailure(parent, batch, fmt.Errorf("request worker panic: %v", recovered), operation != models.TaskOperationSyncStatus)
		} else if !completed {
			e.writeBatchFailure(parent, batch, ctx.Err(), operation != models.TaskOperationSyncStatus)
		}
	}()
	var err error
	switch operation {
	case models.TaskOperationCreate:
		err = e.executeCreateBatch(ctx, batch)
	case models.TaskOperationSyncStatus:
		err = e.executeStatusBatch(ctx, batch)
	case models.TaskOperationValidate:
		err = e.executeValidationBatch(ctx, batch)
	case models.TaskOperationCancel:
		err = e.executeCancellationBatch(ctx, batch)
	default:
		err = fmt.Errorf("unsupported request operation %q", operation)
	}
	if err != nil {
		e.writeBatchFailure(parent, batch, err, operation != models.TaskOperationSyncStatus)
		completed = true
	} else {
		completed = true
	}
}

func (e *Engine) executeCreateBatch(ctx context.Context, batch []operationDecision) error {
	tasks := make([]*models.InferenceTask, len(batch))
	for i := range batch {
		tasks[i] = &batch[i].task
	}
	results, err := relay.BatchCreateTasks(ctx, tasks)
	if err != nil {
		return err
	}
	if len(results) != len(batch) {
		return fmt.Errorf("create batch returned %d results for %d tasks", len(results), len(batch))
	}
	for i := range batch {
		if err := e.writeOperationCompleted(ctx, batch[i].task.ID, mutationOperationResult{
			Outcome: results[i].Outcome,
			Error:   results[i].Error,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) executeStatusBatch(ctx context.Context, batch []operationDecision) error {
	commitments := make([]string, len(batch))
	for i := range batch {
		commitments[i] = batch[i].task.TaskIDCommitment
	}
	results, err := relay.BatchGetTaskStatus(ctx, commitments)
	if err != nil {
		return err
	}
	if len(results) != len(batch) {
		return fmt.Errorf("status batch returned %d results for %d tasks", len(results), len(batch))
	}
	for i := range batch {
		if err := e.writeOperationCompleted(ctx, batch[i].task.ID, results[i]); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) executeValidationBatch(ctx context.Context, batch []operationDecision) error {
	publicKey, err := utils.GetPubKeyFromPrivKey(e.config.PrivateKey)
	if err != nil {
		return err
	}
	items := make([]relay.BatchValidationItem, len(batch))
	for i := range batch {
		group := batch[i].group
		commitments := make([]string, len(group))
		for j := range group {
			commitments[j] = group[j].TaskIDCommitment
		}
		items[i] = relay.BatchValidationItem{
			PublicKey:         publicKey,
			TaskID:            batch[i].task.TaskID,
			TaskIDCommitments: commitments,
			VrfProof:          batch[i].task.VRFProof,
		}
	}
	results, err := relay.BatchValidateTasks(ctx, items)
	if err != nil {
		return err
	}
	if len(results) != len(batch) {
		return fmt.Errorf("validation batch returned %d results for %d units", len(results), len(batch))
	}
	for i := range batch {
		if err := e.writeOperationCompleted(ctx, batch[i].task.ID, mutationOperationResult{
			Outcome: results[i].Outcome,
			Error:   results[i].Error,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) executeCancellationBatch(ctx context.Context, batch []operationDecision) error {
	commitments := make([]string, len(batch))
	for i := range batch {
		commitments[i] = batch[i].task.TaskIDCommitment
	}
	results, err := relay.BatchAbortTasks(ctx, commitments)
	if err != nil {
		return err
	}
	if len(results) != len(batch) {
		return fmt.Errorf("cancellation batch returned %d results for %d tasks", len(results), len(batch))
	}
	for i := range batch {
		if err := e.writeOperationCompleted(ctx, batch[i].task.ID, mutationOperationResult{
			Outcome: results[i].Outcome,
			Error:   results[i].Error,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) writeOperationCompleted(ctx context.Context, taskID uint, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return e.retryOperationWrite(ctx, taskID, map[string]any{
		"operation_status": models.TaskOperationStatusCompleted,
		"operation_result": string(encoded),
		"operation_error":  "",
	})
}

func (e *Engine) writeBatchFailure(ctx context.Context, batch []operationDecision, err error, unknown bool) {
	if err == nil {
		err = context.DeadlineExceeded
	}
	for _, decision := range batch {
		if writeErr := e.retryOperationWrite(ctx, decision.task.ID, map[string]any{
			"operation_status":  models.TaskOperationStatusFailed,
			"operation_error":   err.Error(),
			"operation_unknown": unknown,
		}); writeErr != nil && !errors.Is(writeErr, errOperationNotRunning) && ctx.Err() == nil {
			log.Errorf("TaskEngine: failed to persist operation failure for task %d: %v", decision.task.ID, writeErr)
		}
	}
}

func (e *Engine) retryOperationWrite(ctx context.Context, taskID uint, updates map[string]any) error {
	for {
		result := e.db.WithContext(ctx).Model(&models.InferenceTask{}).
			Where("id = ? AND operation_status = ?", taskID, models.TaskOperationStatusRunning).
			Updates(updates)
		if result.Error == nil && result.RowsAffected == 1 {
			return nil
		}
		if result.Error == nil {
			return errOperationNotRunning
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(e.config.ScanInterval):
		}
	}
}

var errOperationNotRunning = errors.New("task operation is not running")
