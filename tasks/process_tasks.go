package tasks

import (
	"context"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/relay"
	"crynux_bridge/tasktrace"
	"crynux_bridge/utils"
	"errors"
	"fmt"
	mrand "math/rand"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ethereum/go-ethereum/common/hexutil"
	log "github.com/sirupsen/logrus"
	"github.com/vechain/go-ecvrf"
	"gorm.io/gorm"
)

// Get task by taskIDCommitment
func getTask(ctx context.Context, taskIDCommitment string) (*models.RelayTask, error) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return relay.GetTaskByCommitment(callCtx, taskIDCommitment)
}

func vrfProve(privateKey, samplingSeed []byte) ([]byte, []byte, error) {
	privKey := secp256k1.PrivKeyFromBytes(privateKey)
	beta, pi, err := ecvrf.Secp256k1Sha256Tai.Prove(privKey.ToECDSA(), samplingSeed)
	if err != nil {
		return nil, nil, err
	}
	return beta, pi, nil
}

func createTask(ctx context.Context, task *models.InferenceTask) error {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := relay.CreateTask(callCtx, task); err != nil {
		log.Errorf("ProcessTasks: %d createTask failed: err: %v", task.ID, err)
		return err
	}
	return nil
}

func getNode(ctx context.Context, address string) (*models.RelayNode, error) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return relay.GetNodeByAddress(callCtx, address)
}

func validateSingleTask(ctx context.Context, task *models.InferenceTask) error {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := relay.ValidateTask(callCtx, []*models.InferenceTask{task}); err != nil {
		log.Errorf("ProcessTasks: %d validateSingleTask failed: err: %v", task.ID, err)
		return err
	}
	return nil
}

func validateTaskGroup(ctx context.Context, task1, task2, task3 *models.InferenceTask) error {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := relay.ValidateTask(callCtx, []*models.InferenceTask{task1, task2, task3}); err != nil {
		log.Errorf("ProcessTasks: %d validateTaskGroup failed: err: %v", task1.ID, err)
		return err
	}
	return nil
}

func syncTask(ctx context.Context, task *models.InferenceTask) (*models.RelayTask, error) {
	if len(task.TaskIDCommitment) == 0 {
		return nil, nil
	}

	chainTask, err := getTask(ctx, task.TaskIDCommitment)
	if err != nil {
		if isRelayTaskNotFound(err) {
			if task.Status == models.InferenceTaskPending {
				return nil, nil
			}
			// The task was created on the relay before, so the relay has removed it
			// and will never report a terminal status for it. Abort it locally.
			if err := abortTaskLocally(ctx, task, "task_aborted", map[string]any{
				"reason": "relay_task_not_found",
			}); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, err
	}

	changed := false
	statusChanged := false
	newTask := &models.InferenceTask{}
	chainTaskStatus := models.ChainTaskStatus(chainTask.Status)
	abortReason := models.TaskAbortReason(chainTask.AbortReason)
	taskError := models.TaskError(chainTask.TaskError)

	if task.Status == models.InferenceTaskPending {
		newTask.Status = models.InferenceTaskCreated
		changed = true
		statusChanged = true
	}

	if abortReason != task.AbortReason {
		newTask.AbortReason = abortReason
		changed = true
	}
	if taskError != task.TaskError {
		newTask.TaskError = taskError
		changed = true
	}

	if chainTaskStatus == models.ChainTaskStarted {
		if task.Status != models.InferenceTaskStarted {
			newTask.Status = models.InferenceTaskStarted
			changed = true
			statusChanged = true
		}
	} else if chainTaskStatus == models.ChainTaskParametersUploaded {
		if task.Status != models.InferenceTaskParamsUploaded {
			newTask.Status = models.InferenceTaskParamsUploaded
			changed = true
			statusChanged = true
		}
	} else if chainTaskStatus == models.ChainTaskScoreReady {
		if task.Status != models.InferenceTaskScoreReady {
			newTask.Status = models.InferenceTaskScoreReady
			changed = true
			statusChanged = true
		}
	} else if chainTaskStatus == models.ChainTaskErrorReported {
		if task.Status != models.InferenceTaskErrorReported {
			newTask.Status = models.InferenceTaskErrorReported
			changed = true
			statusChanged = true
		}
	} else if chainTaskStatus == models.ChainTaskValidated || chainTaskStatus == models.ChainTaskGroupValidated {
		if task.Status != models.InferenceTaskValidated {
			newTask.Status = models.InferenceTaskValidated
			changed = true
			statusChanged = true
		}
	} else if chainTaskStatus == models.ChainTaskEndAborted {
		if task.Status != models.InferenceTaskEndAborted {
			newTask.Status = models.InferenceTaskEndAborted
			changed = true
			statusChanged = true
		}
	} else if chainTaskStatus == models.ChainTaskEndInvalidated {
		if task.Status != models.InferenceTaskEndInvalidated {
			newTask.Status = models.InferenceTaskEndInvalidated
			changed = true
			statusChanged = true
		}
	} else if chainTaskStatus == models.ChainTaskEndGroupRefund {
		if task.Status != models.InferenceTaskEndGroupRefund {
			newTask.Status = models.InferenceTaskEndGroupRefund
			changed = true
			statusChanged = true
		}
	} else if chainTaskStatus == models.ChainTaskEndSuccess || chainTaskStatus == models.ChainTaskEndGroupSuccess {
		if task.Status != models.InferenceTaskEndSuccess {
			newTask.Status = models.InferenceTaskEndSuccess
			changed = true
			statusChanged = true
		}
	}

	if changed {
		if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
			return nil, err
		}
		task.AbortReason = abortReason
		task.TaskError = taskError
		if statusChanged {
			task.Status = newTask.Status
			tasktrace.RecordEvent(task, taskStatusEventName(newTask.Status), map[string]any{
				"chain_status": chainTaskStatus,
				"abort_reason": abortReason,
				"task_error":   taskError,
			})
		}
	}
	return chainTask, nil
}

func isTaskBeforeValidation(status models.TaskStatus) bool {
	return status == models.InferenceTaskCreated ||
		status == models.InferenceTaskStarted ||
		status == models.InferenceTaskParamsUploaded
}

func isTaskReadyForValidation(status models.TaskStatus) bool {
	return status == models.InferenceTaskScoreReady ||
		status == models.InferenceTaskErrorReported
}

func isTaskReadyOrLater(status models.TaskStatus) bool {
	return isTaskReadyForValidation(status) ||
		status == models.InferenceTaskValidated ||
		models.IsRelayTerminalTaskStatus(status)
}

func isRelayTaskNotFound(err error) bool {
	var relayErr relay.RelayError
	return errors.As(err, &relayErr) && strings.Contains(relayErr.ErrorMessage, "Task not found")
}

// A 400 response means the relay validated and rejected the request itself,
// so resubmitting the same request can never succeed.
func isRelayRequestRejected(err error) bool {
	var relayErr relay.RelayError
	return errors.As(err, &relayErr) && relayErr.StatusCode == 400
}

func abortTaskLocally(ctx context.Context, task *models.InferenceTask, event string, payload map[string]any) error {
	newTask := &models.InferenceTask{Status: models.InferenceTaskEndAborted}
	if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
		return err
	}
	task.Status = models.InferenceTaskEndAborted
	tasktrace.RecordEvent(task, event, payload)
	return nil
}

// The relay sequence, sampling seed, and VRF data of a task must be persisted before
// validation. Validation sub-tasks carry VRF data at creation and only need the sequence.
func needsRelayTaskData(task *models.InferenceTask) bool {
	return task.Sequence == 0 || len(task.VRFNumber) == 0
}

func hasCreatorValidationTimeout(tasks []models.InferenceTask) bool {
	for _, task := range tasks {
		if task.Status == models.InferenceTaskEndAborted &&
			task.AbortReason == models.TaskAbortCreatorValidationTimeout {
			return true
		}
	}
	return false
}

func allTasksEligibleForGroupValidation(tasks []models.InferenceTask) bool {
	for _, task := range tasks {
		if !isTaskReadyForValidation(task.Status) &&
			task.Status != models.InferenceTaskEndAborted {
			return false
		}
	}
	return true
}

func buildValidationTasks(
	task *models.InferenceTask,
	requiredGPU string,
	requiredGPUVram uint64,
	samplingSeed string,
	vrfProof string,
	vrfNumber string,
) []*models.InferenceTask {
	tasks := make([]*models.InferenceTask, 0, 2)
	for i := 0; i < 2; i++ {
		tasks = append(tasks, &models.InferenceTask{
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
			SamplingSeed:    samplingSeed,
			VRFProof:        vrfProof,
			VRFNumber:       vrfNumber,
		})
	}
	return tasks
}

func syncTaskGroup(ctx context.Context, tasks []models.InferenceTask) error {
	for i := range tasks {
		if _, err := syncTask(ctx, &tasks[i]); err != nil {
			return err
		}
	}
	return nil
}

func doDownloadTaskResult(ctx context.Context, taskIDCommitment string, index uint64, filename string) error {
	for {
		err := func() error {
			file, err := os.Create(filename)
			if err != nil {
				return err
			}
			defer file.Close()

			if err := relay.DownloadTaskResult(ctx, taskIDCommitment, index, file); err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
			var relayErr relay.RelayError
			if errors.As(err, &relayErr) && relayErr.StatusCode == 400 {
				log.Errorf("ProcessTasks: cannot get result of %s:%d, error %v, retry", taskIDCommitment, index, err)
				time.Sleep(time.Second)
				continue
			} else {
				log.Errorf("ProcessTasks: cannot get result of %s:%d, error %v", taskIDCommitment, index, err)
				return err
			}
		}
		return nil
	}
}

func doDownloadTaskResultCheckpoint(ctx context.Context, taskIDCommitment string, filename string) error {
	for {
		err := func() error {
			file, err := os.Create(filename)
			if err != nil {
				return err
			}
			defer file.Close()

			if err := relay.DownloadTaskResultCheckpoint(ctx, taskIDCommitment, file); err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
			var relayErr relay.RelayError
			if errors.As(err, &relayErr) && relayErr.StatusCode == 400 {
				log.Errorf("ProcessTasks: cannot get result checkpoint of %s, error %v, retry", taskIDCommitment, err)
				time.Sleep(time.Second)
				continue
			} else {
				log.Errorf("ProcessTasks: cannot get result checkpoint of %s, error %v", taskIDCommitment, err)
				return err
			}
		}
		return nil
	}

}

func downloadTaskResult(ctx context.Context, task *models.InferenceTask) error {
	appConfig := config.GetConfig()

	taskFolder := path.Join(
		appConfig.DataDir.InferenceTasks,
		task.TaskIDCommitment,
	)

	if err := os.MkdirAll(taskFolder, 0700); err != nil {
		log.Errorf("ProcessTasks: cannot create task result dir of %d", task.ID)
		return err
	}

	ctx1, cancel := context.WithCancel(ctx)
	defer cancel()

	if task.TaskType == models.TaskTypeSDFTLora {
		filename := path.Join(taskFolder, "checkpoint.zip")
		if err := doDownloadTaskResultCheckpoint(ctx1, task.TaskIDCommitment, filename); err != nil {
			return err
		}
		return nil
	} else {
		ext := "png"
		if task.TaskType == models.TaskTypeLLM {
			ext = "json"
		}

		var wg sync.WaitGroup
		errCh := make(chan error, int(task.TaskSize))
		for i := uint64(0); i < task.TaskSize; i++ {
			filename := path.Join(taskFolder, fmt.Sprintf("%d.%s", i, ext))
			wg.Add(1)
			go func(ctx context.Context, taskIDCommitment string, index uint64, filename string) {
				defer wg.Done()
				errCh <- doDownloadTaskResult(ctx, taskIDCommitment, index, filename)
			}(ctx1, task.TaskIDCommitment, i, filename)
		}
		wg.Wait()
		for i := 0; i < int(task.TaskSize); i++ {
			err := <-errCh
			if err != nil {
				return err
			}
		}
		return nil
	}

}

func processOneTask(ctx context.Context, task *models.InferenceTask) error {
	// sync task from database
	if err := task.Sync(ctx, config.GetDB()); err != nil {
		return err
	}
	tasktrace.RecordEvent(task, "worker_processing_started", nil)

	// sync task from relay
	chainTask, err := syncTask(ctx, task)
	if err != nil {
		return err
	}
	log.Infof("ProcessTasks: task %d status %d", task.ID, task.Status)
	if models.IsRelayTerminalTaskStatus(task.Status) && task.Status != models.InferenceTaskEndSuccess {
		return processClientTask(ctx, task)
	}

	// 1. Generate taskIDCommitment if not exist
	// 2. Create task
	// 3. Update task status to InferenceTaskCreated
	if task.Status == models.InferenceTaskPending {
		if len(task.TaskIDCommitment) == 0 {
			nonce, taskIDCommitment := models.GenerateTaskIDCommitment(task.TaskID)
			newTask := &models.InferenceTask{
				Nonce:            nonce,
				TaskIDCommitment: taskIDCommitment,
			}
			if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
				return err
			}
			task.Nonce = nonce
			task.TaskIDCommitment = taskIDCommitment
		}

		tasktrace.RecordEvent(task, "relay_task_create_submitted", nil)
		if err := createTask(ctx, task); err != nil {
			// No relay-side deadline exists before the task is created, so a
			// permanently rejected create request must terminate the task locally.
			if isRelayRequestRejected(err) {
				if err := abortTaskLocally(ctx, task, "relay_task_create_rejected", map[string]any{
					"error": err.Error(),
				}); err != nil {
					return err
				}
				return processClientTask(ctx, task)
			}
			return err
		}

		newTask := &models.InferenceTask{
			Status: models.InferenceTaskCreated,
		}
		if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
			return err
		}
		task.Status = models.InferenceTaskCreated
		tasktrace.RecordEvent(task, "relay_task_created", nil)
		log.Infof("ProcessTasks: create task %d ", task.ID)
	}

	// 1. Sync sequence and sampling seed, update local database
	// 2. If needs two more sub-tasks, generate them and store into database
	// Validation requires the persisted VRF data, so this step must also run when the
	// task has already reached score-ready or error-reported before the data was persisted.
	if !models.IsRelayTerminalTaskStatus(task.Status) && needsRelayTaskData(task) {
		// get task sequence and sampling number
		chainTask, err = syncTask(ctx, task)
		if err != nil {
			return err
		}
		if models.IsRelayTerminalTaskStatus(task.Status) {
			if task.Status == models.InferenceTaskEndSuccess {
				goto download
			}
			return processClientTask(ctx, task)
		}
		newTask := &models.InferenceTask{}
		newTask.Sequence = chainTask.Sequence

		subTasks := make([]*models.InferenceTask, 0)

		// validation tasks' sampling seed is not empty
		// avoid generating validation tasks for validation tasks
		if len(task.SamplingSeed) == 0 {
			newTask.SamplingSeed = chainTask.SamplingSeed
			samplingSeedBytes, err := hexutil.Decode(chainTask.SamplingSeed)
			if err != nil {
				log.Errorf("ProcessTasks: %d decode sampling seed failed: %v", task.ID, err)
				return err
			}
			// generate vrf proof
			appConfig := config.GetConfig()
			pk := appConfig.Blockchain.Account.PrivateKey
			privateKey, err := hexutil.Decode("0x" + pk)
			if err != nil {
				log.Errorf("ProcessTasks: %d decode private key failed: %v", task.ID, err)
				return err
			}
			vrfNum, vrfProof, err := vrfProve(privateKey, samplingSeedBytes)
			if err != nil {
				log.Errorf("ProcessTasks: %d vrf prove failed: %v", task.ID, err)
				return err
			}
			newTask.VRFProof = hexutil.Encode(vrfProof)
			newTask.VRFNumber = hexutil.Encode(vrfNum)

			if utils.VrfNeedValidation(vrfNum) {
				requiredGPU := task.RequiredGPU
				requiredGPUVram := task.RequiredGPUVram
				if task.TaskType == models.TaskTypeLLM {
					// for LLM type task, need to wait the task is started to determine required gpu for sub tasks
					// once the task reaches score-ready or error-reported, the selected node is
					// already set, so the loop exits through the selected-node check
					for len(chainTask.SelectedNode) == 0 {
						chainTask, err = syncTask(ctx, task)
						if err != nil {
							return err
						}
						if models.IsRelayTerminalTaskStatus(task.Status) {
							if task.Status == models.InferenceTaskEndSuccess {
								goto download
							}
							return processClientTask(ctx, task)
						}
						if len(chainTask.SelectedNode) > 0 {
							break
						}
						time.Sleep(time.Second)
					}
					node, err := getNode(ctx, chainTask.SelectedNode)
					if err != nil {
						return err
					}
					requiredGPU = node.GPUName
					requiredGPUVram = node.GPUVram
				}
				subTasks = buildValidationTasks(
					task,
					requiredGPU,
					requiredGPUVram,
					newTask.SamplingSeed,
					newTask.VRFProof,
					newTask.VRFNumber,
				)
			}
		}

		err = config.GetDB().Transaction(func(tx *gorm.DB) error {
			if err := task.Update(ctx, tx, newTask); err != nil {
				return err
			}
			if len(subTasks) > 0 {
				if err := models.SaveTasks(ctx, tx, subTasks); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if len(subTasks) > 0 {
			tasktrace.RegisterTasks(task, dereferenceTasks(subTasks), "validation")
			tasktrace.RecordEvent(task, "validation_tasks_created", map[string]any{
				"validation_task_count": len(subTasks),
			})
		}
	}

	// wait task status to be score-ready or error reported
	if isTaskBeforeValidation(task.Status) {
		for {
			_, err := syncTask(ctx, task)
			if err != nil {
				return err
			}
			if isTaskReadyForValidation(task.Status) || models.IsRelayTerminalTaskStatus(task.Status) {
				break
			}
			time.Sleep(time.Second)
		}
		log.Infof("ProcessTasks: task %d status %d", task.ID, task.Status)
		if models.IsRelayTerminalTaskStatus(task.Status) {
			if task.Status == models.InferenceTaskEndSuccess {
				goto download
			}
			return processClientTask(ctx, task)
		}
	}

	// 1. If single task, validate
	// 2. If task group, wait until all sub-tasks are ready, then validate
	// 3. Wait for validate result(Status: InferenceTaskEndInvalidated, InferenceTaskEndSuccess, InferenceTaskEndGroupRefund, InferenceTaskEndAborted)
	if isTaskReadyForValidation(task.Status) {
		needValidate := false
		taskGroup, err := models.GetTaskGroup(ctx, config.GetDB(), task.TaskID)
		if err != nil {
			log.Errorf("ProcessTasks: get tasks of task id %s error: %v", task.TaskID, err)
			return err
		}
		if len(taskGroup) == 1 {
			needValidate = true
		} else if len(taskGroup) == 3 {
			// wait all tasks in group be in status score ready, error reported or aborted
			for {
				if err := syncTaskGroup(ctx, taskGroup); err != nil {
					return err
				}
				readyCount := 0
				for _, subTask := range taskGroup {
					if isTaskReadyOrLater(subTask.Status) {
						readyCount += 1
					}
				}
				if readyCount == 3 || hasCreatorValidationTimeout(taskGroup) {
					break
				}
				time.Sleep(time.Second)
				taskGroup, err = models.GetTaskGroup(ctx, config.GetDB(), task.TaskID)
				if err != nil {
					log.Errorf("ProcessTasks: get tasks of %s error: %v", task.TaskID, err)
					return err
				}
			}
			if allTasksEligibleForGroupValidation(taskGroup) && !hasCreatorValidationTimeout(taskGroup) {
				validateTaskIDCommitment := ""
				for _, subTask := range taskGroup {
					if isTaskReadyForValidation(subTask.Status) {
						validateTaskIDCommitment = subTask.TaskIDCommitment
						break
					}
				}
				if validateTaskIDCommitment == task.TaskIDCommitment {
					needValidate = true
				}
			}
		}

		// validate task
		if needValidate {
			taskGroup, err = models.GetTaskGroup(ctx, config.GetDB(), task.TaskID)
			if err != nil {
				return err
			}
			if err := syncTaskGroup(ctx, taskGroup); err != nil {
				return err
			}
			if !allTasksEligibleForGroupValidation(taskGroup) || hasCreatorValidationTimeout(taskGroup) {
				needValidate = false
			}
		}
		if needValidate {
			tasktrace.RecordEvent(task, "validation_submitted", map[string]any{
				"task_group_size": len(taskGroup),
			})
			if len(taskGroup) == 1 {
				if err := validateSingleTask(ctx, task); err != nil {
					return err
				}
				log.Infof("ProcessTasks: validate single task %d", task.ID)
			} else if len(taskGroup) == 3 {
				if err := validateTaskGroup(ctx, &taskGroup[0], &taskGroup[1], &taskGroup[2]); err != nil {
					return err
				}
				log.Infof("ProcessTasks: %d validate task group task %d, %d, %d", task.ID, taskGroup[0].ID, taskGroup[1].ID, taskGroup[2].ID)
			}
		}

		// wait task status to be validated, invalidated, success, group refund or aborted
		for {
			_, err := syncTask(ctx, task)
			if err != nil {
				return err
			}
			if task.Status == models.InferenceTaskValidated || models.IsRelayTerminalTaskStatus(task.Status) {
				break
			}
			time.Sleep(time.Second)
		}
		log.Infof("ProcessTasks: task %d status %d", task.ID, task.Status)
		if models.IsRelayTerminalTaskStatus(task.Status) {
			if task.Status == models.InferenceTaskEndSuccess {
				goto download
			}
			return processClientTask(ctx, task)
		}
	}

	if task.Status == models.InferenceTaskValidated {
		for {
			_, err := syncTask(ctx, task)
			if err != nil {
				return err
			}
			if models.IsRelayTerminalTaskStatus(task.Status) {
				break
			}
			time.Sleep(time.Second)
		}
		log.Infof("ProcessTasks: task %d status %d", task.ID, task.Status)
		if task.Status != models.InferenceTaskEndSuccess {
			return processClientTask(ctx, task)
		}
	}

	// download task result
download:
	if task.Status == models.InferenceTaskEndSuccess {
		tasktrace.RecordEvent(task, "result_download_started", nil)
		err := downloadTaskResult(ctx, task)
		if err != nil {
			return err
		}
		newTask := &models.InferenceTask{
			Status: models.InferenceTaskResultDownloaded,
		}
		if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
			return err
		}
		task.Status = models.InferenceTaskResultDownloaded
		tasktrace.RecordEvent(task, "result_downloaded", nil)
		log.Infof("ProcessTasks: download results of task %d", task.ID)
	}

	// update client task status
	if err := processClientTask(ctx, task); err != nil {
		return err
	}

	return nil
}

func taskStatusEventName(status models.TaskStatus) string {
	switch status {
	case models.InferenceTaskCreated:
		return "relay_task_created_observed"
	case models.InferenceTaskStarted:
		return "task_started"
	case models.InferenceTaskParamsUploaded:
		return "parameters_uploaded"
	case models.InferenceTaskScoreReady:
		return "score_ready"
	case models.InferenceTaskErrorReported:
		return "error_reported"
	case models.InferenceTaskValidated:
		return "validation_passed"
	case models.InferenceTaskEndAborted:
		return "task_aborted"
	case models.InferenceTaskEndGroupRefund:
		return "task_group_refunded"
	case models.InferenceTaskEndInvalidated:
		return "task_invalidated"
	case models.InferenceTaskEndSuccess:
		return "task_execution_succeeded"
	case models.InferenceTaskResultDownloaded:
		return "result_downloaded"
	default:
		return fmt.Sprintf("status_%d", status)
	}
}

func dereferenceTasks(tasks []*models.InferenceTask) []models.InferenceTask {
	result := make([]models.InferenceTask, 0, len(tasks))
	for _, task := range tasks {
		if task != nil {
			result = append(result, *task)
		}
	}
	return result
}

func processClientTask(ctx context.Context, task *models.InferenceTask) error {
	if task.TaskType == models.TaskTypeSDFTLora {
		return nil
	}

	clientTask, err := models.GetClientTaskByID(ctx, config.GetDB(), task.ClientTaskID)
	if err != nil {
		return err
	}
	if clientTask.Status == models.ClientTaskStatusRunning && task.Finished() {
		if task.Success() {
			clientTask.Status = models.ClientTaskStatusSuccess
			if err := clientTask.Update(ctx, config.GetDB(), clientTask); err != nil {
				return err
			}
		} else {
			taskGroup, err := models.GetTaskGroup(ctx, config.GetDB(), task.TaskID)
			if err != nil {
				return err
			}
			if len(taskGroup) == 1 {
				clientTask.FailedCount += 1
				clientTask.Status = models.ClientTaskStatusFailed
				if err := clientTask.Update(ctx, config.GetDB(), clientTask); err != nil {
					return err
				}
			} else {
				allFinished := true
				success := false
				for _, subTask := range taskGroup {
					if !subTask.Finished() {
						allFinished = false
					}
					if subTask.Success() {
						success = true
					}
				}
				if allFinished {
					if success {
						clientTask.Status = models.ClientTaskStatusSuccess
					} else {
						clientTask.FailedCount += 1
						clientTask.Status = models.ClientTaskStatusFailed
					}
					if err := clientTask.Update(ctx, config.GetDB(), clientTask); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func getUnprocessedTasks(ctx context.Context) ([]models.InferenceTask, error) {
	allTasks := make([]models.InferenceTask, 0)

	limit := 100
	offset := 0

	for {
		tasks, err := func() ([]models.InferenceTask, error) {
			dbCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			tasks := make([]models.InferenceTask, 0)
			err := config.GetDB().WithContext(dbCtx).Model(&models.InferenceTask{}).
				Where("status != ?", models.InferenceTaskEndAborted).
				Where("status != ?", models.InferenceTaskEndInvalidated).
				Where("status != ?", models.InferenceTaskEndGroupRefund).
				Where("status != ?", models.InferenceTaskResultDownloaded).
				Order("id ASC").
				Limit(limit).
				Offset(offset).
				Find(&tasks).
				Error
			if err != nil {
				return nil, err
			}
			return tasks, nil
		}()
		if err != nil {
			return nil, err
		}
		if len(tasks) == 0 {
			break
		}
		allTasks = append(allTasks, tasks...)
		offset += len(tasks)
	}
	return allTasks, nil
}

// Get unprocessed tasks from database and process them, each task is processed in a goroutine
func ProcessTasks(ctx context.Context) {
	var d sync.Map
	for {
		// get unprocessed tasks from database
		tasks, err := getUnprocessedTasks(ctx)
		if err != nil {
			log.Errorf("ProcessTasks: cannot get unprocessed tasks: %v", err)
			time.Sleep(time.Duration(mrand.Float64()*1000) * time.Millisecond)
			continue
		}

		// process tasks one by one
		if len(tasks) > 0 {
			for _, task := range tasks {
				_, loaded := d.LoadOrStore(task.ID, struct{}{})
				if loaded {
					continue
				}
				go func(ctx context.Context, task models.InferenceTask) {
					defer d.Delete(task.ID)
					log.Infof("ProcessTasks: start processing task %d", task.ID)
					for {
						if err := processOneTask(ctx, &task); err != nil {
							if ctx.Err() != nil {
								return
							}
							log.Errorf("ProcessTasks: process task %d error %v, retry", task.ID, err)
							duration := time.Duration((mrand.Float64()*3 + 2) * 1000)
							select {
							case <-ctx.Done():
								return
							case <-time.After(duration * time.Millisecond):
							}
						} else {
							log.Infof("ProcessTasks: process task %d successfully", task.ID)
							return
						}
					}
				}(ctx, task)
			}
		}

		time.Sleep(time.Second)
	}
}
