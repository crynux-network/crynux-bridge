package migrations

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
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

func M20250120(db *gorm.DB) *gormigrate.Gormigrate {
	type TaskStatus uint8
	type ChainTaskType uint8
	type TaskAbortReason uint8
	type TaskError uint8

	type InferenceTask struct {
		ID              uint           `gorm:"primarykey"`
		CreatedAt       time.Time      `gorm:"index"`
		UpdatedAt       time.Time      `gorm:"index"`
		DeletedAt       gorm.DeletedAt `gorm:"index"`
		ClientID        uint           `json:"client_id"`
		ClientTaskID    uint           `json:"client_task_id"`
		TaskArgs        string         `json:"task_args"`
		TaskType        ChainTaskType  `json:"task_type" gorm:"index"`
		TaskModelIDs    StringArray    `json:"task_model_ids" gorm:"text"`
		TaskVersion     string         `json:"task_version"`
		TaskFee         uint64         `json:"task_fee"`
		MinVram         uint64         `json:"min_vram"`
		RequiredGPU     string         `json:"required_gpu"`
		RequiredGPUVram uint64         `json:"required_gpu_vram"`
		TaskSize        uint64         `json:"task_size"`

		Status           TaskStatus `json:"status" gorm:"index"`
		TaskID           string     `json:"task_id" gorm:"index"`
		TaskIDCommitment string     `json:"task_id_commitment" gorm:"index"`
		Nonce            string     `json:"nonce"`
		Sequence         uint64     `json:"sequence"`
		NeedResult       bool       `json:"need_result"`
		SamplingSeed     string     `json:"sampling_seed"`
		VRFProof         string     `json:"vrf_proof"`
		VRFNumber        string     `json:"vrf_number"`

		AbortReason TaskAbortReason `json:"abort_reason"`
		TaskError   TaskError       `json:"task_error"`
	}

	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		{
			ID: "M20250120",
			Migrate: func(tx *gorm.DB) error {
				if err := tx.Migrator().RenameTable("inference_tasks", "old_inference_tasks"); err != nil {
					return err
				}
				if err := tx.Migrator().CreateTable(&InferenceTask{}); err != nil {
					return err
				}

				return nil
			},
			Rollback: func(tx *gorm.DB) error {
				if err := tx.Migrator().DropTable(&InferenceTask{}); err != nil {
					return err
				}
				if err := tx.Migrator().RenameTable("old_inference_tasks", "inference_tasks"); err != nil {
					return err
				}
				return nil
			},
		},
	})
}
