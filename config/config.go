package config

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/viper"
)

var appConfig *AppConfig

// InitConfig Init is an exported method that takes the config from the config file
// and unmarshal it into AppConfig struct
func InitConfig(configPath string) error {
	v := viper.New()
	v.SetConfigType("yml")
	v.SetConfigName("config")

	if configPath != "" {
		v.AddConfigPath(configPath)
	} else {
		v.AddConfigPath("/app/config")
		v.AddConfigPath("config")
	}

	if err := v.ReadInConfig(); err != nil {
		return err
	}

	appConfig = &AppConfig{}

	if err := v.Unmarshal(appConfig); err != nil {
		return err
	}
	configDir := filepath.Dir(v.ConfigFileUsed())
	if err := loadHeartbeatTasksFile(appConfig, configDir); err != nil {
		return err
	}
	if err := validateTaskFeeConfig(appConfig); err != nil {
		return err
	}
	if err := validateHeartbeatTasksConfig(appConfig); err != nil {
		return err
	}
	if appConfig.Task.RepeatNum <= 0 {
		return errors.New("task.repeat_num must be set to a positive value")
	}
	if appConfig.Task.TaskStatusPollInterval <= 0 {
		return errors.New("task.task_status_poll_interval must be set to a positive value")
	}
	if err := validateTaskEngineConfig(appConfig.Task.Engine); err != nil {
		return err
	}
	if appConfig.Http.MaxBodyBytes <= 0 {
		return errors.New("http.max_body_bytes must be set to a positive value")
	}

	if appConfig.Environment == EnvTest {
		appConfig.Test.RootPrivateKey = NormalizePrivateKey(appConfig.Test.RootPrivateKey)
		if err := checkTestApplicationAccount(); err != nil {
			return err
		}
		appConfig.Blockchain.Account.PrivateKey = appConfig.Test.RootPrivateKey
		appConfig.Blockchain.Account.Address = appConfig.Test.RootAddress
	} else {
		// Load hard-coded private key
		appConfig.Blockchain.Account.PrivateKey = GetPrivateKey(appConfig.Blockchain.Account.PrivateKeyFile)
		if err := checkApplicationAccount(); err != nil {
			return err
		}
	}

	return nil
}

func validateTaskEngineConfig(engine TaskEngineConfig) error {
	positiveDurations := []struct {
		name  string
		value time.Duration
	}{
		{"scan_interval", engine.ScanInterval},
		{"status_poll_interval", engine.StatusPollInterval},
		{"execution_overrun_poll_interval", engine.ExecutionOverrunPollInterval},
		{"retry_interval", engine.RetryInterval},
		{"operation_timeout", engine.OperationTimeout},
	}
	for _, field := range positiveDurations {
		if field.value <= 0 {
			return fmt.Errorf("task.engine.%s must be set to a positive value", field.name)
		}
	}
	if engine.ExecutionPollAdvance < 0 {
		return errors.New("task.engine.execution_poll_advance must not be negative")
	}
	positiveIntegers := []struct {
		name  string
		value int
	}{
		{"expansion_batch_size", engine.ExpansionBatchSize},
		{"operation_result_batch_size", engine.OperationResultBatchSize},
		{"due_task_batch_size", engine.DueTaskBatchSize},
		{"create_batch_size", engine.CreateBatchSize},
		{"status_batch_size", engine.StatusBatchSize},
		{"validation_batch_size", engine.ValidationBatchSize},
		{"cancellation_batch_size", engine.CancellationBatchSize},
		{"create_workers", engine.CreateWorkers},
		{"status_workers", engine.StatusWorkers},
		{"validation_workers", engine.ValidationWorkers},
		{"cancellation_workers", engine.CancellationWorkers},
		{"result_workers", engine.ResultWorkers},
	}
	for _, field := range positiveIntegers {
		if field.value <= 0 {
			return fmt.Errorf("task.engine.%s must be set to a positive value", field.name)
		}
	}
	return nil
}

func checkApplicationAccount() error {

	if appConfig.Blockchain.Account.PrivateKey == "" {
		return errors.New("application account private key not set")
	}

	if appConfig.Blockchain.Account.Address == "" {
		return errors.New("application account address not set")
	}

	pk := NormalizePrivateKey(appConfig.Blockchain.Account.PrivateKey)

	// Check private key and address
	privateKey, err := crypto.HexToECDSA(pk)
	if err != nil {
		return err
	}

	publicKey := privateKey.Public()

	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("error casting public key to ECDSA")
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()

	if address != appConfig.Blockchain.Account.Address {
		return errors.New("account address and private key mismatch.\nAccount address: " + appConfig.Blockchain.Account.Address + "\nPrivate key derived address: " + address + "\n")
	}

	return nil
}

func checkTestApplicationAccount() error {

	if appConfig.Test.RootPrivateKey == "" {
		return errors.New("test private key not set")
	}

	if appConfig.Test.RootAddress == "" {
		return errors.New("test account address not set")
	}

	testPk := NormalizePrivateKey(appConfig.Test.RootPrivateKey)

	testRootPrivateKey, err := crypto.HexToECDSA(testPk)
	if err != nil {
		return err
	}

	testRootPublicKey := testRootPrivateKey.Public()

	testRootPublicKeyECDSA, ok := testRootPublicKey.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("error casting test root public key to ECDSA")
	}

	testRootAddress := crypto.PubkeyToAddress(*testRootPublicKeyECDSA).Hex()

	if testRootAddress != appConfig.Test.RootAddress {
		return errors.New("test root account address and private key mismatch")
	}

	return nil
}

func GetConfig() *AppConfig {
	return appConfig
}
