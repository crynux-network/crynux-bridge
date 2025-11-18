package models

import (
	"time"
)

type RelayTask struct {
	Sequence           uint64          `json:"sequence"`
	TaskArgs           string          `json:"task_args"`
	TaskIDCommitment   string          `json:"task_id_commitment"`
	Creator            string          `json:"creator"`
	SamplingSeed       string          `json:"sampling_seed"`
	Nonce              string          `json:"nonce"`
	Status             ChainTaskStatus `json:"status"`
	TaskType           ChainTaskType   `json:"task_type"`
	TaskVersion        string          `json:"task_version"`
	Timeout            uint64          `json:"timeout"`
	MinVRAM            uint64          `json:"min_vram"`
	RequiredGPU        string          `json:"required_gpu"`
	RequiredGPUVRAM    uint64          `json:"required_gpu_vram"`
	TaskFee            BigInt          `json:"task_fee"`
	TaskSize           uint64          `json:"task_size"`
	ModelIDs           []string        `json:"model_ids"`
	AbortReason        TaskAbortReason `json:"abort_reason"`
	TaskError          TaskError       `json:"task_error"`
	Score              string          `json:"score"`
	QOSScore           uint64          `json:"qos_score"`
	SelectedNode       string          `json:"selected_node"`
	CreateTime         *time.Time      `json:"create_time,omitempty"`
	StartTime          *time.Time      `json:"start_time,omitempty"`
	ScoreReadyTime     *time.Time      `json:"score_ready_time,omitempty"`
	ValidatedTime      *time.Time      `json:"validated_time,omitempty"`
	ResultUploadedTime *time.Time      `json:"result_uploaded_time,omitempty"`
}

type NodeStatus uint8

const (
	NodeStatusQuit = iota
	NodeStatusAvailable
	NodeStatusBusy
	NodeStatusPendingPause
	NodeStatusPendingQuit
	NodeStatusPaused
)

type RelayNode struct {
	Address                string     `json:"address" gorm:"index"`
	Status                 NodeStatus `json:"status" gorm:"index"`
	GPUName                string     `json:"gpu_name" gorm:"index"`
	GPUVram                uint64     `json:"gpu_vram" gorm:"index"`
	Version                string     `json:"version"`
	InUseModelIDs          []string   `json:"in_use_model_ids"`
	ModelIDs               []string   `json:"model_ids"`
	StakingScore           float64    `json:"staking_score"`
	QOSScore               float64    `json:"qos_score"`
	ProbWeight             float64    `json:"prob_weight"`
	OperatorStaking        string     `json:"operator_staking"`
	DelegatorStaking       string     `json:"delegator_staking"`
	DelegatorShare         uint8      `json:"delegator_share"`
	DelegatorsNum          int        `json:"delegators_num"`
	TotalOperatorEarnings  string     `json:"total_operator_earnings"`
	TodayOperatorEarnings  string     `json:"today_operator_earnings"`
	TotalDelegatorEarnings string     `json:"total_delegator_earnings"`
	TodayDelegatorEarnings string     `json:"today_delegator_earnings"`
}
