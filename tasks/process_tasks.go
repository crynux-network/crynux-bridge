package tasks

import (
	"context"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/relay"
	"crynux_bridge/utils"
	"crypto/rand"
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
	"github.com/ethereum/go-ethereum/crypto"
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

func generateTaskIDCommitment(taskID string) (string, string) {
	taskIDBytes := hexutil.MustDecode(taskID)
	nonceBytes := make([]byte, 32)
	rand.Read(nonceBytes)
	nonce := hexutil.Encode(nonceBytes)

	taskIDCommitmentBytes := crypto.Keccak256(append(taskIDBytes, nonceBytes...))
	taskIDCommitment := hexutil.Encode(taskIDCommitmentBytes)

	return nonce, taskIDCommitment
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
		if task.Status == models.InferenceTaskPending {
			var relayErr relay.RelayError
			if errors.As(err, &relayErr) && strings.Contains(relayErr.ErrorMessage, "Task not found") {
				return nil, nil
			}
			return nil, err
		}
		return nil, err
	}

	changed := false
	newTask := &models.InferenceTask{}
	chainTaskStatus := models.ChainTaskStatus(chainTask.Status)
	abortReason := models.TaskAbortReason(chainTask.AbortReason)
	taskError := models.TaskError(chainTask.TaskError)

	if task.Status == models.InferenceTaskPending {
		newTask.Status = models.InferenceTaskCreated
		changed = true
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
		}
	} else if chainTaskStatus == models.ChainTaskParametersUploaded {
		if task.Status != models.InferenceTaskParamsUploaded {
			newTask.Status = models.InferenceTaskParamsUploaded
			changed = true
		}
	} else if chainTaskStatus == models.ChainTaskScoreReady {
		if task.Status != models.InferenceTaskScoreReady {
			newTask.Status = models.InferenceTaskScoreReady
			changed = true
		}
	} else if chainTaskStatus == models.ChainTaskErrorReported {
		if task.Status != models.InferenceTaskErrorReported {
			newTask.Status = models.InferenceTaskErrorReported
			changed = true
		}
	} else if chainTaskStatus == models.ChainTaskValidated || chainTaskStatus == models.ChainTaskGroupValidated {
		if task.Status != models.InferenceTaskValidated {
			newTask.Status = models.InferenceTaskValidated
			changed = true
		}
	} else if chainTaskStatus == models.ChainTaskEndAborted {
		if task.Status != models.InferenceTaskEndAborted {
			newTask.Status = models.InferenceTaskEndAborted
			changed = true
		}
	} else if chainTaskStatus == models.ChainTaskEndInvalidated {
		if task.Status != models.InferenceTaskEndInvalidated {
			newTask.Status = models.InferenceTaskEndInvalidated
			changed = true
		}
	} else if chainTaskStatus == models.ChainTaskEndGroupRefund {
		if task.Status != models.InferenceTaskEndGroupRefund {
			newTask.Status = models.InferenceTaskEndGroupRefund
			changed = true
		}
	} else if chainTaskStatus == models.ChainTaskEndSuccess || chainTaskStatus == models.ChainTaskEndGroupSuccess {
		if task.Status != models.InferenceTaskEndSuccess {
			newTask.Status = models.InferenceTaskEndSuccess
			changed = true
		}
	}

	if changed {
		if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
			return nil, err
		}
	}
	return chainTask, nil
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

	// sync task from relay
	_, err := syncTask(ctx, task)
	if err != nil {
		return err
	}
	log.Infof("ProcessTasks: task %d status %d", task.ID, task.Status)

	// 1. Generate taskIDCommitment if not exist
	// 2. Create task
	// 3. Update task status to InferenceTaskCreated
	if task.Status == models.InferenceTaskPending {
		if len(task.TaskIDCommitment) == 0 {
			nonce, taskIDCommitment := generateTaskIDCommitment(task.TaskID)
			newTask := &models.InferenceTask{
				Nonce:            nonce,
				TaskIDCommitment: taskIDCommitment,
			}
			if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
				return err
			}
		}

		if err := createTask(ctx, task); err != nil {
			return err
		}

		newTask := &models.InferenceTask{
			Status: models.InferenceTaskCreated,
		}
		if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
			return err
		}
		log.Infof("ProcessTasks: create task %d ", task.ID)
	}

	// 1. Sync sequence and sampling seed, update local database
	// 2. If needs two more sub-tasks, generate them and store into database
	// 3. Wait for task result hash to be submitted to relay(Status: InferenceTaskTaskScoreReady)
	if task.Status == models.InferenceTaskCreated || task.Status == models.InferenceTaskStarted || task.Status == models.InferenceTaskParamsUploaded {
		// get task sequence and sampling number
		chainTask, err := getTask(ctx, task.TaskIDCommitment)
		if err != nil {
			return err
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
					for len(chainTask.SelectedNode) == 0 {
						chainTask, err = getTask(ctx, task.TaskIDCommitment)
						if err != nil {
							return err
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
				for i := 0; i < 2; i++ {
					subTask := &models.InferenceTask{
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
						TaskID:          task.TaskID,
						SamplingSeed:    newTask.SamplingSeed,
						VRFProof:        newTask.VRFProof,
						VRFNumber:       newTask.VRFNumber,
					}
					subTasks = append(subTasks, subTask)
				}
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

		// wait task status to be score-ready or error reported
		for {
			_, err := syncTask(ctx, task)
			if err != nil {
				return err
			}
			if task.Status == models.InferenceTaskScoreReady || task.Status == models.InferenceTaskErrorReported || task.Status == models.InferenceTaskEndAborted {
				break
			}
			time.Sleep(time.Second)
		}
		log.Infof("ProcessTasks: task %d status %d", task.ID, task.Status)
	}

	// 1. If single task, validate
	// 2. If task group, wait until all sub-tasks are ready, then validate
	// 3. Wait for validate result(Status: InferenceTaskEndInvalidated, InferenceTaskEndSuccess, InferenceTaskEndGroupRefund, InferenceTaskEndAborted)
	if task.Status == models.InferenceTaskScoreReady || task.Status == models.InferenceTaskErrorReported {
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
				readyCount := 0
				for _, subTask := range taskGroup {
					if subTask.Status >= models.InferenceTaskScoreReady {
						readyCount += 1
					}
				}
				if readyCount == 3 {
					break
				}
				time.Sleep(time.Second)
				taskGroup, err = models.GetTaskGroup(ctx, config.GetDB(), task.TaskID)
				if err != nil {
					log.Errorf("ProcessTasks: get tasks of %s error: %v", task.TaskID, err)
					return err
				}
			}
			validateTaskIDCommitment := ""
			for _, subTask := range taskGroup {
				if subTask.Status == models.InferenceTaskScoreReady || subTask.Status == models.InferenceTaskErrorReported {
					validateTaskIDCommitment = subTask.TaskIDCommitment
					break
				}
			}
			if validateTaskIDCommitment == task.TaskIDCommitment {
				needValidate = true
			}
		}

		// validate task
		if needValidate {
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
			if task.Status >= models.InferenceTaskValidated {
				break
			}
			time.Sleep(time.Second)
		}
		log.Infof("ProcessTasks: task %d status %d", task.ID, task.Status)
	}

	if task.Status == models.InferenceTaskValidated {
		for {
			_, err := syncTask(ctx, task)
			if err != nil {
				return err
			}
			if task.Status == models.InferenceTaskEndSuccess || task.Status == models.InferenceTaskEndAborted {
				break
			}
			time.Sleep(time.Second)
		}
		log.Infof("ProcessTasks: task %d status %d", task.ID, task.Status)
	}

	// download task result
	if task.Status == models.InferenceTaskEndSuccess {
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
		log.Infof("ProcessTasks: download results of task %d", task.ID)
	}

	// update client task status
	if err := processClientTask(ctx, task); err != nil {
		return err
	}

	return nil
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
				Where("status != ?", models.InferenceTaskNeedCancel).
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
					var ctx1 context.Context
					var cancel context.CancelFunc
					// if timeout is 0, use default timeout
					timeout := task.Timeout
					if timeout == 0 {
						appConfig := config.GetConfig()
						timeout = appConfig.Task.DefaultTimeout
						if task.TaskType == models.TaskTypeSDFTLora {
							timeout = appConfig.Task.SDFinetuneTimeout
						}
						timeout *= 60
					}
					duration := time.Duration(timeout)*time.Second + 3*time.Minute // additional 3 minutes for waiting task to start
					deadline := task.CreatedAt.Add(duration)
					ctx1, cancel = context.WithDeadline(ctx, deadline)
					defer cancel()

					for {
						c := make(chan error, 1)
						go func() {
							c <- processOneTask(ctx1, &task)
						}()

						select {
						// process task successfully or failed
						case err := <-c:
							if err != nil {
								log.Errorf("ProcessTasks: process task %d error %v, retry", task.ID, err)
								duration := time.Duration((mrand.Float64()*3 + 2) * 1000)
								time.Sleep(duration * time.Millisecond)
							} else {
								log.Infof("ProcessTasks: process task %d successfully", task.ID)
								return
							}
						// process task timeout
						case <-ctx1.Done():
							err := ctx1.Err()
							log.Errorf("ProcessTasks: process task %d timeout %v, finish", task.ID, err)
							if err == context.DeadlineExceeded {
								newTask := &models.InferenceTask{}
								if task.Status != models.InferenceTaskEndAborted &&
									task.Status != models.InferenceTaskEndInvalidated &&
									task.Status != models.InferenceTaskEndGroupRefund &&
									task.Status != models.InferenceTaskEndSuccess &&
									task.Status != models.InferenceTaskResultDownloaded {
									newTask.Status = models.InferenceTaskNeedCancel
								} else if task.Status == models.InferenceTaskEndSuccess {
									newTask.Status = models.InferenceTaskEndAborted
								}
								if newTask.Status != models.InferenceTaskPending {
									for {
										if err := task.Update(ctx, config.GetDB(), newTask); err != nil {
											log.Errorf("ProcessTasks: save task %d error %v", task.ID, err)
											time.Sleep(2 * time.Second)
										} else {
											break
										}
										if err := processClientTask(ctx, &task); err != nil {
											log.Errorf("ProcessTasks: process client task %d error %v", task.ID, err)
											time.Sleep(2 * time.Second)
										} else {
											break
										}
									}
								}
							}
							return
						}
					}
				}(ctx, task)
			}
		}

		time.Sleep(time.Second)
	}
}
