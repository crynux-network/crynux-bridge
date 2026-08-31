package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type clientTaskEngineFields20260828 struct {
	Submission         string    `gorm:"column:submission;type:longtext"`
	SubmissionTaskType uint8     `gorm:"column:submission_task_type"`
	SubmissionModelID  string    `gorm:"column:submission_model_id;type:varchar(255)"`
	EffectiveRepeatNum int       `gorm:"column:effective_repeat_num;not null;default:1"`
	RepeatExpanded     bool      `gorm:"column:repeat_expanded"`
	NextActionAt       time.Time `gorm:"column:next_action_at"`
	ExpansionError     string    `gorm:"column:expansion_error;type:text"`
}

func (clientTaskEngineFields20260828) TableName() string {
	return "client_tasks"
}

type inferenceTaskEngineFields20260828 struct {
	NextActionAt                time.Time  `gorm:"column:next_action_at"`
	OperationType               string     `gorm:"column:operation_type;type:varchar(16)"`
	OperationStatus             string     `gorm:"column:operation_status;type:varchar(16)"`
	OperationStartedAt          *time.Time `gorm:"column:operation_started_at"`
	OperationResult             string     `gorm:"column:operation_result;type:longtext"`
	OperationError              string     `gorm:"column:operation_error;type:text"`
	OperationRetryCount         uint       `gorm:"column:operation_retry_count"`
	OperationUnknown            bool       `gorm:"column:operation_unknown"`
	RelayChecked                bool       `gorm:"column:relay_checked"`
	RelayFound                  bool       `gorm:"column:relay_found"`
	SelectedExecutionGPU        string     `gorm:"column:selected_execution_gpu"`
	SelectedExecutionGPUVram    uint64     `gorm:"column:selected_execution_gpu_vram"`
	EstimatedCompletionAt       *time.Time `gorm:"column:estimated_completion_at"`
	ResultAvailable             bool       `gorm:"column:result_available"`
	ValidationSubmitted         bool       `gorm:"column:validation_submitted"`
	SiblingCancellationRequired bool       `gorm:"column:sibling_cancellation_required"`
}

func (inferenceTaskEngineFields20260828) TableName() string {
	return "inference_tasks"
}

func M20260828(db *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{{
		ID: "M20260828",
		Migrate: func(tx *gorm.DB) error {
			clientTask := &clientTaskEngineFields20260828{}
			for _, column := range []string{
				"Submission",
				"SubmissionTaskType",
				"SubmissionModelID",
				"EffectiveRepeatNum",
				"RepeatExpanded",
				"NextActionAt",
				"ExpansionError",
			} {
				if err := tx.Migrator().AddColumn(clientTask, column); err != nil {
					return err
				}
			}

			inferenceTask := &inferenceTaskEngineFields20260828{}
			for _, column := range []string{
				"NextActionAt",
				"OperationType",
				"OperationStatus",
				"OperationStartedAt",
				"OperationResult",
				"OperationError",
				"OperationRetryCount",
				"OperationUnknown",
				"RelayChecked",
				"RelayFound",
				"SelectedExecutionGPU",
				"SelectedExecutionGPUVram",
				"EstimatedCompletionAt",
				"ResultAvailable",
				"ValidationSubmitted",
				"SiblingCancellationRequired",
			} {
				if err := tx.Migrator().AddColumn(inferenceTask, column); err != nil {
					return err
				}
			}

			indexes := []string{
				"CREATE INDEX idx_client_task_expansion ON client_tasks (repeat_expanded, next_action_at, id)",
				"CREATE INDEX idx_client_task_pending_submission ON client_tasks (client_id, repeat_expanded, submission_task_type, submission_model_id)",
				"CREATE INDEX idx_inference_task_due ON inference_tasks (operation_status, next_action_at, id)",
				"CREATE INDEX idx_inference_task_operation ON inference_tasks (operation_status, id)",
				"CREATE INDEX idx_inference_task_group ON inference_tasks (task_id, sequence, id)",
				"CREATE INDEX idx_inference_task_client_status ON inference_tasks (client_task_id, status, id)",
			}
			for _, sql := range indexes {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			indexes := []struct {
				model any
				name  string
			}{
				{&clientTaskEngineFields20260828{}, "idx_client_task_expansion"},
				{&clientTaskEngineFields20260828{}, "idx_client_task_pending_submission"},
				{&inferenceTaskEngineFields20260828{}, "idx_inference_task_due"},
				{&inferenceTaskEngineFields20260828{}, "idx_inference_task_operation"},
				{&inferenceTaskEngineFields20260828{}, "idx_inference_task_group"},
				{&inferenceTaskEngineFields20260828{}, "idx_inference_task_client_status"},
			}
			for _, index := range indexes {
				if err := tx.Migrator().DropIndex(index.model, index.name); err != nil {
					return err
				}
			}

			clientTask := &clientTaskEngineFields20260828{}
			for _, column := range []string{
				"ExpansionError",
				"NextActionAt",
				"RepeatExpanded",
				"EffectiveRepeatNum",
				"SubmissionModelID",
				"SubmissionTaskType",
				"Submission",
			} {
				if err := tx.Migrator().DropColumn(clientTask, column); err != nil {
					return err
				}
			}

			inferenceTask := &inferenceTaskEngineFields20260828{}
			for _, column := range []string{
				"SiblingCancellationRequired",
				"ValidationSubmitted",
				"ResultAvailable",
				"EstimatedCompletionAt",
				"SelectedExecutionGPUVram",
				"SelectedExecutionGPU",
				"RelayFound",
				"RelayChecked",
				"OperationUnknown",
				"OperationRetryCount",
				"OperationError",
				"OperationResult",
				"OperationStartedAt",
				"OperationStatus",
				"OperationType",
				"NextActionAt",
			} {
				if err := tx.Migrator().DropColumn(inferenceTask, column); err != nil {
					return err
				}
			}
			return nil
		},
	}})
}
