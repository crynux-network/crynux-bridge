package config

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
var weiPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
var maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

func ValidateWei(value string) (string, error) {
	if !weiPattern.MatchString(value) {
		return "", fmt.Errorf("invalid Wei value %q", value)
	}
	n, ok := new(big.Int).SetString(value, 10)
	if !ok || n.Cmp(maxUint256) > 0 {
		return "", fmt.Errorf("Wei value %q exceeds uint256", value)
	}
	return value, nil
}

func CNXToWei(value string) (string, error) {
	if !decimalPattern.MatchString(value) {
		return "", fmt.Errorf("invalid CNX value %q", value)
	}
	parts := strings.SplitN(value, ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) > 9 {
			return "", fmt.Errorf("CNX value %q has more than 9 decimal places", value)
		}
	}
	weiText := parts[0] + fraction + strings.Repeat("0", 18-len(fraction))
	weiText = strings.TrimLeft(weiText, "0")
	if weiText == "" {
		weiText = "0"
	}
	return ValidateWei(weiText)
}

func MultiplyWei(value string, multiplier uint64) (string, error) {
	if _, err := ValidateWei(value); err != nil {
		return "", err
	}
	n, _ := new(big.Int).SetString(value, 10)
	n.Mul(n, new(big.Int).SetUint64(multiplier))
	return ValidateWei(n.String())
}

func validateTaskFeeConfig(appConfig *AppConfig) error {
	fees := map[string]string{
		"task.default_sd_task_fee_cnx":          appConfig.Task.DefaultSDTaskFeeCNX,
		"task.default_sd_xl_task_fee_cnx":       appConfig.Task.DefaultSDXLTaskFeeCNX,
		"task.default_llm_task_fee_cnx":         appConfig.Task.DefaultLLMTaskFeeCNX,
		"task.default_sd_finetune_task_fee_cnx": appConfig.Task.DefaultSDFinetuneTaskFeeCNX,
	}

	for name, feeCNX := range fees {
		if _, err := CNXToWei(feeCNX); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}

	for i, heartbeatTask := range appConfig.Task.HeartbeatTasks.Tasks {
		if _, err := CNXToWei(heartbeatTask.FeeCNX); err != nil {
			return fmt.Errorf("task.heartbeat_tasks.tasks[%d].fee_cnx: %w", i, err)
		}
	}

	return nil
}
