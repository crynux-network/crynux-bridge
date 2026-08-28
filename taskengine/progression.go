package taskengine

import (
	"context"
	"crynux_bridge/models"
	"crynux_bridge/relay"
	"crynux_bridge/tasktrace"
	"crynux_bridge/utils"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/go-ecvrf"
	"gorm.io/gorm"
)

type mutationOperationResult struct {
	Outcome relay.BatchOutcome `json:"outcome"`
	Error   string             `json:"error,omitempty"`
}

func (e *Engine) applyOperationResults(ctx context.Context) error {
	var tasks []models.InferenceTask
	if err := e.db.WithContext(ctx).
		Where("operation_status IN ?", []models.TaskOperationStatus{
			models.TaskOperationStatusCompleted,
			models.TaskOperationStatusFailed,
		}).
		Order("id ASC").
		Limit(e.config.OperationResultBatchSize).
		Find(&tasks).Error; err != nil {
		return err
	}
	for i := range tasks {
		if err := e.applyOperationResult(ctx, tasks[i]); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) applyOperationResult(ctx context.Context, task models.InferenceTask) error {
	err := e.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.InferenceTask
		if err := tx.First(&current, task.ID).Error; err != nil {
			return err
		}
		if current.OperationStatus == models.TaskOperationStatusFailed {
			if current.OperationUnknown && current.OperationType == models.TaskOperationValidate {
				if err := tx.Model(&models.InferenceTask{}).Where("task_id = ?", current.TaskID).Updates(map[string]any{
					"relay_checked":  false,
					"next_action_at": time.Now(),
				}).Error; err != nil {
					return err
				}
			}
			return tx.Model(&current).Updates(map[string]any{
				"operation_type":        models.TaskOperationNone,
				"operation_status":      models.TaskOperationStatusNone,
				"operation_started_at":  nil,
				"operation_result":      "",
				"operation_retry_count": gorm.Expr("operation_retry_count + ?", 1),
				"next_action_at":        time.Now().Add(e.config.RetryInterval),
			}).Error
		}

		switch current.OperationType {
		case models.TaskOperationSyncStatus:
			var status relay.BatchStatusItem
			if err := json.Unmarshal([]byte(current.OperationResult), &status); err != nil {
				return err
			}
			if err := e.applyRelayStatus(ctx, tx, &current, status); err != nil {
				return err
			}
		case models.TaskOperationCreate:
			var result mutationOperationResult
			if err := json.Unmarshal([]byte(current.OperationResult), &result); err != nil {
				return err
			}
			if result.Outcome == relay.BatchOutcomeCreated || result.Outcome == relay.BatchOutcomeAlreadyExists {
				if err := tx.Model(&current).Updates(map[string]any{
					"status":            models.InferenceTaskCreated,
					"relay_checked":     true,
					"relay_found":       true,
					"operation_unknown": false,
					"next_action_at":    time.Now(),
				}).Error; err != nil {
					return err
				}
			} else if result.Outcome == relay.BatchOutcomeTemporaryError {
				if err := tx.Model(&current).Update("next_action_at", time.Now().Add(e.config.RetryInterval)).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Model(&current).Updates(map[string]any{
					"status":         models.InferenceTaskEndAborted,
					"abort_reason":   models.TaskAbortCreatorCancelled,
					"next_action_at": time.Now(),
				}).Error; err != nil {
					return err
				}
			}
		case models.TaskOperationValidate:
			var result mutationOperationResult
			if err := json.Unmarshal([]byte(current.OperationResult), &result); err != nil {
				return err
			}
			if result.Outcome == relay.BatchOutcomeValidated || result.Outcome == relay.BatchOutcomeAlreadyApplied {
				if err := tx.Model(&models.InferenceTask{}).Where("task_id = ?", current.TaskID).Updates(map[string]any{
					"validation_submitted": true,
					"next_action_at":       time.Now(),
				}).Error; err != nil {
					return err
				}
			} else if result.Outcome == relay.BatchOutcomeTemporaryError {
				if err := tx.Model(&current).Update("next_action_at", time.Now().Add(e.config.RetryInterval)).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Model(&models.InferenceTask{}).Where("task_id = ?", current.TaskID).Updates(map[string]any{
					"status":         models.InferenceTaskEndAborted,
					"next_action_at": time.Now(),
				}).Error; err != nil {
					return err
				}
			}
		case models.TaskOperationCancel:
			if err := tx.Model(&current).Updates(map[string]any{
				"sibling_cancellation_required": false,
				"next_action_at":                time.Now(),
			}).Error; err != nil {
				return err
			}
		case models.TaskOperationResult:
			if err := tx.Model(&current).Updates(map[string]any{
				"status":         models.InferenceTaskResultDownloaded,
				"next_action_at": time.Now(),
			}).Error; err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown operation type %q", current.OperationType)
		}

		if err := tx.Model(&models.InferenceTask{}).Where("id = ?", current.ID).Updates(map[string]any{
			"operation_type":       models.TaskOperationNone,
			"operation_status":     models.TaskOperationStatusNone,
			"operation_started_at": nil,
			"operation_result":     "",
			"operation_error":      "",
		}).Error; err != nil {
			return err
		}
		if err := e.scheduleSiblingCancellation(tx, &current); err != nil {
			return err
		}
		return e.recalculateClientTask(ctx, tx, current.ClientTaskID)
	})
	if err != nil {
		return err
	}
	var current models.InferenceTask
	if err := e.db.WithContext(ctx).First(&current, task.ID).Error; err != nil {
		return err
	}
	if err := e.RegisterTraceTasks(ctx, current.ClientTaskID); err != nil {
		return err
	}
	if current.Status != task.Status {
		tasktrace.RecordEvent(&current, "status_changed", map[string]any{
			"previous_status": task.Status,
			"operation":       task.OperationType,
		})
	}
	return nil
}

func (e *Engine) applyRelayStatus(ctx context.Context, tx *gorm.DB, task *models.InferenceTask, status relay.BatchStatusItem) error {
	if !status.Found {
		updates := map[string]any{
			"relay_checked":                 true,
			"relay_found":                   false,
			"operation_unknown":             false,
			"sibling_cancellation_required": false,
			"next_action_at":                time.Now(),
		}
		if task.Status != models.InferenceTaskPending {
			updates["status"] = models.InferenceTaskEndAborted
		}
		return tx.Model(task).Updates(updates).Error
	}
	localStatus := relayStatusToLocal(status.Status)
	nextAction := time.Now().Add(e.config.StatusPollInterval)
	if (localStatus == models.InferenceTaskStarted || localStatus == models.InferenceTaskParamsUploaded) &&
		status.EstimatedExecutionCompletionAt != nil {
		nextAction = status.EstimatedExecutionCompletionAt.Add(-e.config.ExecutionPollAdvance)
		minimum := time.Now().Add(e.config.ScanInterval)
		if nextAction.Before(minimum) {
			nextAction = time.Now().Add(e.config.ExecutionOverrunPollInterval)
		}
	}
	updates := map[string]any{
		"status":                      localStatus,
		"abort_reason":                status.AbortReason,
		"task_error":                  status.TaskError,
		"sequence":                    status.Sequence,
		"relay_checked":               true,
		"relay_found":                 true,
		"operation_unknown":           false,
		"selected_execution_gpu":      status.SelectedExecutionGPU,
		"selected_execution_gpu_vram": status.SelectedExecutionGPUVram,
		"estimated_completion_at":     status.EstimatedExecutionCompletionAt,
		"result_available":            status.ResultAvailable,
		"next_action_at":              nextAction,
	}
	if task.SiblingCancellationRequired && localStatus != models.InferenceTaskCreated {
		updates["sibling_cancellation_required"] = false
	}
	if err := tx.Model(task).Updates(updates).Error; err != nil {
		return err
	}
	task.Status = localStatus
	task.Sequence = status.Sequence
	task.AbortReason = status.AbortReason
	if task.SamplingSeed == "" && status.SamplingSeed != "" {
		return e.persistVRFAndMembers(ctx, tx, task, status)
	}
	return nil
}

func (e *Engine) persistVRFAndMembers(
	ctx context.Context,
	tx *gorm.DB,
	task *models.InferenceTask,
	status relay.BatchStatusItem,
) error {
	seed, err := hexutil.Decode(status.SamplingSeed)
	if err != nil {
		return err
	}
	privateKey, err := hexutil.Decode("0x" + e.config.PrivateKey)
	if err != nil {
		return err
	}
	key := secp256k1.PrivKeyFromBytes(privateKey)
	vrfNumber, vrfProof, err := ecvrf.Secp256k1Sha256Tai.Prove(key.ToECDSA(), seed)
	if err != nil {
		return err
	}
	if utils.VrfNeedValidation(vrfNumber) && task.TaskType == models.TaskTypeLLM && status.SelectedExecutionGPU == "" {
		return nil
	}
	updates := map[string]any{
		"sampling_seed": status.SamplingSeed,
		"vrf_number":    hexutil.Encode(vrfNumber),
		"vrf_proof":     hexutil.Encode(vrfProof),
	}
	if err := tx.Model(task).Updates(updates).Error; err != nil {
		return err
	}
	if !utils.VrfNeedValidation(vrfNumber) {
		return nil
	}
	var count int64
	if err := tx.Model(&models.InferenceTask{}).Where("task_id = ?", task.TaskID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return nil
	}
	requiredGPU := task.RequiredGPU
	requiredGPUVram := task.RequiredGPUVram
	if task.TaskType == models.TaskTypeLLM {
		requiredGPU = status.SelectedExecutionGPU
		requiredGPUVram = status.SelectedExecutionGPUVram
	}
	now := time.Now()
	members := make([]models.InferenceTask, 2)
	for i := range members {
		members[i] = models.InferenceTask{
			ClientID:        task.ClientID,
			ClientTaskID:    task.ClientTaskID,
			TaskArgs:        task.TaskArgs,
			TaskType:        task.TaskType,
			TaskModelIDs:    task.TaskModelIDs,
			TaskVersion:     task.TaskVersion,
			TaskFee:         task.TaskFee,
			MinVram:         task.MinVram,
			RequiredGPU:     requiredGPU,
			RequiredGPUVram: requiredGPUVram,
			TaskSize:        task.TaskSize,
			Timeout:         task.Timeout,
			TaskID:          task.TaskID,
			SamplingSeed:    status.SamplingSeed,
			VRFNumber:       hexutil.Encode(vrfNumber),
			VRFProof:        hexutil.Encode(vrfProof),
			NextActionAt:    now,
		}
		nonce, commitment := models.GenerateTaskIDCommitment(task.TaskID)
		members[i].Nonce = nonce
		members[i].TaskIDCommitment = commitment
	}
	return tx.WithContext(ctx).Create(&members).Error
}

func (e *Engine) scheduleSiblingCancellation(tx *gorm.DB, task *models.InferenceTask) error {
	var current models.InferenceTask
	if err := tx.First(&current, task.ID).Error; err != nil {
		return err
	}
	if current.Status != models.InferenceTaskEndAborted {
		return nil
	}
	var count int64
	if err := tx.Model(&models.InferenceTask{}).Where("task_id = ?", current.TaskID).Count(&count).Error; err != nil {
		return err
	}
	if count != 3 {
		return nil
	}
	return tx.Model(&models.InferenceTask{}).
		Where("task_id = ? AND id <> ? AND status NOT IN ?", current.TaskID, current.ID, terminalStatuses()).
		Updates(map[string]any{
			"sibling_cancellation_required": true,
			"next_action_at":                time.Now(),
		}).Error
}

func relayStatusToLocal(status models.ChainTaskStatus) models.TaskStatus {
	switch status {
	case models.ChainTaskQueued:
		return models.InferenceTaskCreated
	case models.ChainTaskStarted:
		return models.InferenceTaskStarted
	case models.ChainTaskParametersUploaded:
		return models.InferenceTaskParamsUploaded
	case models.ChainTaskScoreReady:
		return models.InferenceTaskScoreReady
	case models.ChainTaskErrorReported:
		return models.InferenceTaskErrorReported
	case models.ChainTaskValidated, models.ChainTaskGroupValidated:
		return models.InferenceTaskValidated
	case models.ChainTaskEndAborted:
		return models.InferenceTaskEndAborted
	case models.ChainTaskEndInvalidated:
		return models.InferenceTaskEndInvalidated
	case models.ChainTaskEndGroupRefund:
		return models.InferenceTaskEndGroupRefund
	case models.ChainTaskEndSuccess, models.ChainTaskEndGroupSuccess:
		return models.InferenceTaskEndSuccess
	default:
		return models.InferenceTaskCreated
	}
}

func terminalStatuses() []models.TaskStatus {
	return []models.TaskStatus{
		models.InferenceTaskEndAborted,
		models.InferenceTaskEndGroupRefund,
		models.InferenceTaskEndInvalidated,
		models.InferenceTaskEndSuccess,
		models.InferenceTaskResultDownloaded,
	}
}

func isReadyForValidation(status models.TaskStatus) bool {
	return status == models.InferenceTaskScoreReady || status == models.InferenceTaskErrorReported
}

func hasValidationTimeout(tasks []models.InferenceTask) bool {
	for i := range tasks {
		if tasks[i].Status == models.InferenceTaskEndAborted &&
			tasks[i].AbortReason == models.TaskAbortCreatorValidationTimeout {
			return true
		}
	}
	return false
}

func validationGroupReady(tasks []models.InferenceTask) bool {
	if len(tasks) != 3 || hasValidationTimeout(tasks) {
		return false
	}
	for i := range tasks {
		if !isReadyForValidation(tasks[i].Status) && tasks[i].Status != models.InferenceTaskEndAborted {
			return false
		}
	}
	return true
}

var errOperationNoLongerRequired = errors.New("operation no longer required")
