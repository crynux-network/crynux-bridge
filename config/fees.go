package config

import (
	"fmt"
	"math"
)

const gweiPerCNX = 1_000_000_000

func CNXToGWei(feeCNX float64) (uint64, error) {
	if math.IsNaN(feeCNX) || math.IsInf(feeCNX, 0) || feeCNX < 0 {
		return 0, fmt.Errorf("invalid fee CNX value %v", feeCNX)
	}

	feeGWei := math.Round(feeCNX * gweiPerCNX)
	if feeGWei > float64(^uint64(0)) {
		return 0, fmt.Errorf("fee CNX value %v is too large", feeCNX)
	}

	return uint64(feeGWei), nil
}

func validateTaskFeeConfig(appConfig *AppConfig) error {
	fees := map[string]float64{
		"task.default_sd_task_fee_cnx":          appConfig.Task.DefaultSDTaskFeeCNX,
		"task.default_sd_xl_task_fee_cnx":       appConfig.Task.DefaultSDXLTaskFeeCNX,
		"task.default_llm_task_fee_cnx":         appConfig.Task.DefaultLLMTaskFeeCNX,
		"task.default_sd_finetune_task_fee_cnx": appConfig.Task.DefaultSDFinetuneTaskFeeCNX,
	}

	for name, feeCNX := range fees {
		if _, err := CNXToGWei(feeCNX); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}

	for i, heartbeatTask := range appConfig.Task.HeartbeatTasks.Tasks {
		if _, err := CNXToGWei(heartbeatTask.FeeCNX); err != nil {
			return fmt.Errorf("task.heartbeat_tasks.tasks[%d].fee_cnx: %w", i, err)
		}
	}

	return nil
}
