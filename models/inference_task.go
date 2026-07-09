package models

import (
	"context"
	"crynux_bridge/utils"
	"crypto/rand"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"gorm.io/gorm"
)

type ChainTaskStatus uint8

const (
	ChainTaskQueued ChainTaskStatus = iota
	ChainTaskStarted
	ChainTaskParametersUploaded
	ChainTaskErrorReported
	ChainTaskScoreReady
	ChainTaskValidated
	ChainTaskGroupValidated
	ChainTaskEndInvalidated
	ChainTaskEndSuccess
	ChainTaskEndAborted
	ChainTaskEndGroupRefund
	ChainTaskEndGroupSuccess
)

type TaskStatus int

const (
	InferenceTaskPending TaskStatus = iota
	InferenceTaskCreated
	InferenceTaskStarted
	InferenceTaskParamsUploaded
	InferenceTaskScoreReady
	InferenceTaskErrorReported
	InferenceTaskValidated
	InferenceTaskEndAborted
	InferenceTaskEndGroupRefund
	InferenceTaskEndInvalidated
	InferenceTaskEndSuccess
	InferenceTaskResultDownloaded
	InferenceTaskNeedCancel
)

type ChainTaskType uint8

const (
	TaskTypeSD ChainTaskType = iota
	TaskTypeLLM
	TaskTypeSDFTLora
)

type TaskAbortReason uint8

const (
	TaskAbortReasonNone TaskAbortReason = iota
	TaskAbortTimeout
	TaskAbortModelDownloadFailed
	TaskAbortIncorrectResult
	TaskAbortTaskFeeTooLow
)

type TaskError uint8

const (
	TaskErrorNone TaskError = iota
	TaskErrorParametersValidationFailed
)

type StringArray []string

func (arr *StringArray) Scan(val interface{}) error {
	var arrString string
	switch v := val.(type) {
	case string:
		arrString = v
	case []byte:
		arrString = string(v)
	case nil:
		return nil
	default:
		return errors.New(fmt.Sprint("Unable to parse value to StringArray: ", val))
	}
	*arr = strings.Split(arrString, ";")
	return nil
}

func (arr StringArray) Value() (driver.Value, error) {
	res := strings.Join(arr, ";")
	return res, nil
}

func (arr StringArray) MarshalJSON() ([]byte, error) {
	return json.Marshal([]string(arr))
}

func (arr *StringArray) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, (*[]string)(arr))
}

type InferenceTask struct {
	RootModel
	ClientID        uint          `json:"client_id"`
	Client          Client        `json:"-"`
	ClientTaskID    uint          `json:"client_task_id"`
	ClientTask      ClientTask    `json:"-"`
	TaskArgs        string        `json:"task_args"`
	TaskType        ChainTaskType `json:"task_type"`
	TaskModelIDs    StringArray   `json:"task_model_ids"`
	TaskVersion     string        `json:"task_version"`
	TaskFee         uint64        `json:"task_fee"`
	MinVram         uint64        `json:"min_vram"`
	RequiredGPU     string        `json:"required_gpu"`
	RequiredGPUVram uint64        `json:"required_gpu_vram"`
	TaskSize        uint64        `json:"task_size"`
	Timeout         uint64        `json:"timeout"`

	Status           TaskStatus `json:"status"`
	TaskID           string     `json:"task_id"`
	TaskIDCommitment string     `json:"task_id_commitment"`
	Nonce            string     `json:"nonce"`
	Sequence         uint64     `json:"sequence"`
	NeedResult       bool       `json:"need_result"`
	SamplingSeed     string     `json:"sampling_seed"`
	VRFProof         string     `json:"vrf_proof"`
	VRFNumber        string     `json:"vrf_number"`

	AbortReason TaskAbortReason `json:"abort_reason"`
	TaskError   TaskError       `json:"task_error"`
}

func (t *InferenceTask) BeforeCreate(*gorm.DB) error {
	t.Status = InferenceTaskPending
	return nil
}

func GenerateTaskIDCommitment(taskID string) (string, string) {
	taskIDBytes := hexutil.MustDecode(taskID)
	nonceBytes := make([]byte, 32)
	rand.Read(nonceBytes)
	nonce := hexutil.Encode(nonceBytes)

	taskIDCommitmentBytes := crypto.Keccak256(append(taskIDBytes, nonceBytes...))
	taskIDCommitment := hexutil.Encode(taskIDCommitmentBytes)

	return nonce, taskIDCommitment
}

func (task *InferenceTask) Finished() bool {
	return task.Status == InferenceTaskEndAborted || task.Status == InferenceTaskEndGroupRefund || task.Status == InferenceTaskEndInvalidated || task.Status == InferenceTaskResultDownloaded || task.Status == InferenceTaskNeedCancel
}

func (task *InferenceTask) Success() bool {
	return task.Status == InferenceTaskResultDownloaded
}

func (task *InferenceTask) Save(ctx context.Context, db *gorm.DB) error {
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := db.WithContext(dbCtx).Save(&task).Error; err != nil {
		return err
	}
	return nil
}

// Update the task in the database
func (task *InferenceTask) Update(ctx context.Context, db *gorm.DB, newTask *InferenceTask) error {
	if task.ID == 0 {
		return errors.New("InferenceTask.ID cannot be 0 when update")
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.WithContext(dbCtx).Model(task).Updates(newTask).Error; err != nil {
		return err
	}
	return nil
}

func (task *InferenceTask) Sync(ctx context.Context, db *gorm.DB) error {
	if task.ID == 0 {
		return errors.New("InferenceTask.ID cannot be 0 when sync")
	}
	dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return db.WithContext(dbCtx).Model(task).Where("id = ?", task.ID).First(task).Error
}

func SaveTasks(ctx context.Context, db *gorm.DB, tasks []*InferenceTask) error {
	dbCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return db.WithContext(dbCtx).Save(tasks).Error
}

func GetTaskGroup(ctx context.Context, db *gorm.DB, taskID string) ([]InferenceTask, error) {
	tasks := make([]InferenceTask, 0)
	dbCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	err := db.WithContext(dbCtx).Model(&InferenceTask{}).Where("task_id = ?", taskID).Order("sequence").Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (t *InferenceTask) GetTaskHash() (*[32]byte, error) {

	hash := crypto.Keccak256Hash([]byte(t.TaskArgs))
	byte32Hash := byteArrayToByte32Array(hash.Bytes())
	return byte32Hash, nil
}

func (t *InferenceTask) GetDataHash() (*[32]byte, error) {
	return nil, nil
}

func byteArrayToByte32Array(input []byte) *[32]byte {
	var output [32]byte
	copy(output[:], input)
	return &output
}

func WaitTaskGroup(ctx context.Context, db *gorm.DB, task *InferenceTask) ([]InferenceTask, error) {
	for {
		err := task.Sync(ctx, db)
		if err != nil {
			return nil, err
		}
		if len(task.VRFNumber) > 0 {
			break
		}
		time.Sleep(time.Second)
	}
	vrfNumber, _ := hexutil.Decode(task.VRFNumber)
	if utils.VrfNeedValidation(vrfNumber) {
		taskGroup, err := GetTaskGroup(ctx, db, task.TaskID)
		if err != nil {
			return nil, err
		}
		return taskGroup, nil
	} else {
		return []InferenceTask{*task}, nil
	}
}

var ErrTaskEndWithoutResult = errors.New("task end without result downloaded")
var ErrTaskTimeout = errors.New("task timeout")

func WaitAllTaskGroup(ctx context.Context, db *gorm.DB, tasks []InferenceTask) ([]InferenceTask, error) {
	var waitGroup sync.WaitGroup
	taskGroupChan := make(chan []InferenceTask, len(tasks))
	for _, task := range tasks {
		waitGroup.Add(1)
		go func(task InferenceTask) {
			defer waitGroup.Done()
			taskGroup, err := WaitTaskGroup(ctx, db, &task)
			if err != nil {
				return
			}
			taskGroupChan <- taskGroup
		}(task)
	}
	go func() {
		waitGroup.Wait()
		close(taskGroupChan)
	}()

	taskGroups := make([]InferenceTask, 0)
	for taskGroup := range taskGroupChan {
		taskGroups = append(taskGroups, taskGroup...)
	}
	return taskGroups, nil
}

func WaitForTaskFinish(ctx context.Context, db *gorm.DB, task *InferenceTask) (TaskStatus, error) {
	for {
		// 1. get task by id
		err := task.Sync(ctx, db)
		if err != nil {
			return task.Status, err
		}

		// 2. check task status
		taskStatus := task.Status
		// task end without result downloaded
		if taskStatus == InferenceTaskEndInvalidated || taskStatus == InferenceTaskEndGroupRefund || taskStatus == InferenceTaskEndAborted {
			return taskStatus, nil
		}
		// task end with result downloaded
		if taskStatus == InferenceTaskResultDownloaded {
			return taskStatus, nil
		}
		// task not end, then sleep and continue looping
		time.Sleep(time.Second)
	}
}

func WaitResultTask(ctx context.Context, db *gorm.DB, tasks []InferenceTask) (*InferenceTask, error) {
	resultChan := make(chan *InferenceTask, len(tasks))
	errChan := make(chan error, len(tasks))
	doneChan := make(chan struct{})
	var closeDoneOnce sync.Once
	closeDone := func() {
		closeDoneOnce.Do(func() {
			close(doneChan)
		})
	}

	var wg sync.WaitGroup
	wg.Add(len(tasks))

	for _, t := range tasks {
		task := t
		go func(ctx context.Context, t *InferenceTask) {
			defer wg.Done()

			select {
			case <-doneChan:
				return
			default:
				status, err := WaitForTaskFinish(ctx, db, t)
				if err != nil {
					select {
					case errChan <- err:
					case <-doneChan:
					}
					return
				}
				if status == InferenceTaskResultDownloaded {
					select {
					case resultChan <- t:
						closeDone()
					case <-doneChan:
					}
					return
				}
				select {
				case errChan <- nil:
				case <-doneChan:
				}
			}
		}(ctx, &task)
	}

	var errs []error
	for i := 0; i < len(tasks); i++ {
		select {
		case err := <-errChan:
			if err != nil {
				errs = append(errs, err)
			}
		case result := <-resultChan:
			go func() {
				wg.Wait()
				close(errChan)
				close(resultChan)
			}()
			return result, nil
		case <-ctx.Done():
			closeDone()
			wg.Wait()
			if deadline, ok := ctx.Deadline(); ok {
				return nil, fmt.Errorf("%w: task did not finish before deadline %s (%v)", ErrTaskTimeout, deadline.Format(time.RFC3339), ctx.Err())
			}
			return nil, fmt.Errorf("%w: %v", ErrTaskTimeout, ctx.Err())
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("all tasks failed: %v", errs)
	}
	return nil, ErrTaskEndWithoutResult
}

func GetSDFTTaskFinalTask(ctx context.Context, db *gorm.DB, clientTaskID uint) (*InferenceTask, error) {
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	task := InferenceTask{}
	err := db.WithContext(dbCtx).Model(&InferenceTask{}).
		Where("client_task_id = ?", clientTaskID).
		Where("status = ?", InferenceTaskResultDownloaded).
		Order("id DESC").First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

func GetSDFTTaskFailedCount(ctx context.Context, db *gorm.DB, clientTaskID uint) (uint, error) {
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	totalCount := int64(0)
	err := db.WithContext(dbCtx).Model(&InferenceTask{}).
		Where("client_task_id = ?", clientTaskID).
		Group("task_id").
		Count(&totalCount).Error
	if err != nil {
		return 0, err
	}
	successCount := int64(0)
	err = db.WithContext(dbCtx).Model(&InferenceTask{}).
		Where("client_task_id = ?", clientTaskID).
		Where("status = ?", InferenceTaskResultDownloaded).
		Count(&successCount).Error
	if err != nil {
		return 0, err
	}

	return uint(totalCount - successCount), nil
}
