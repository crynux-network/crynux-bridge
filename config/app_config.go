package config

const (
	EnvProduction = "production"
	EnvDebug      = "debug"
	EnvTest       = "test"
)

type AppConfig struct {
	Environment string `mapstructure:"environment"`

	Db struct {
		Driver           string `mapstructure:"driver"`
		ConnectionString string `mapstructure:"connection"`
		Log              struct {
			Level       string `mapstructure:"level"`
			Output      string `mapstructure:"output"`
			MaxFileSize int    `mapstructure:"max_file_size"`
			MaxDays     int    `mapstructure:"max_days"`
			MaxFileNum  int    `mapstructure:"max_file_num"`
		} `mapstructure:"log"`
	} `mapstructure:"db"`

	Log struct {
		Level       string `mapstructure:"level"`
		Output      string `mapstructure:"output"`
		MaxFileSize int    `mapstructure:"max_file_size"`
		MaxDays     int    `mapstructure:"max_days"`
		MaxFileNum  int    `mapstructure:"max_file_num"`
		Features    struct {
			LLMAPIRequestLogEnabled bool `mapstructure:"llm_api_request_log_enabled"`
		} `mapstructure:"features"`
	} `mapstructure:"log"`

	Http struct {
		Host string `mapstructure:"host"`
		Port string `mapstructure:"port"`
	} `mapstructure:"http"`

	DataDir struct {
		InferenceTasks string `mapstructure:"inference_tasks"`
		ModelImages    string `mapstructure:"model_images"`
	} `mapstructure:"data_dir"`

	Blockchain struct {
		RPS           uint64 `mapstructure:"rps"`
		RpcEndpoint   string `mapstructure:"rpc_endpoint"`
		StartBlockNum uint64 `mapstructure:"start_block_num"`
		GasLimit      uint64 `mapstructure:"gas_limit"`
		GasPrice      uint64 `mapstructure:"gas_price"`
		ChainID       uint64 `mapstructure:"chain_id"`

		Account struct {
			Address        string `mapstructure:"address"`
			PrivateKey     string `mapstructure:"private_key"`
			PrivateKeyFile string `mapstructure:"private_key_file"`
		} `mapstructure:"account"`

		Contracts struct {
			Netstats string `mapstructure:"netstats"`
			Task     string `mapstructure:"task"`
			Node     string `mapstructure:"node"`
		} `mapstructure:"contracts"`
	} `mapstructure:"blockchain"`

	Relay struct {
		BaseURL string `mapstructure:"base_url"`
	} `mapstructure:"relay"`

	Task struct {
		SDTaskFee                     uint64    `mapstructure:"sd_task_fee"`
		SDXLTaskFee                   uint64    `mapstructure:"sd_xl_task_fee"`
		LLMTaskFee                    uint64    `mapstructure:"llm_task_fee"`
		LLMQuantTaskFee               uint64    `mapstructure:"llm_quant_task_fee"`
		RepeatNum                     int       `mapstructure:"repeat_num"`
		PendingAutoTasksLimit         uint64    `mapstructure:"pending_auto_tasks_limit"`
		PendingLargeVramLLMTasksLimit uint64    `mapstructure:"pending_large_vram_llm_tasks_limit"`
		AutoTasksBatchSize            uint64    `mapstructure:"auto_tasks_batch_size"`
		DefaultTimeout                uint64    `mapstructure:"default_timeout"`
		SDFinetuneTimeout             uint64    `mapstructure:"sd_finetune_timeout"`
		TaskVersions                  []string  `mapstructure:"task_versions"`
		AutoTaskVersionRatio          []float64 `mapstructure:"auto_task_version_ratio"`
		AutoTaskTypeRatio             []float64 `mapstructure:"auto_task_type_ratio"`
	} `mapstructure:"task"`

	TaskSchema struct {
		StableDiffusionInference    string `mapstructure:"stable_diffusion_inference"`
		GPTInference                string `mapstructure:"gpt_inference"`
		StableDiffusionFinetuneLora string `mapstructure:"stable_diffusion_finetune_lora"`
	} `mapstructure:"task_schema"`

	OpenRouter struct {
		ModelsFile string `mapstructure:"models_file"`
	}

	Test struct {
		RootAddress    string `mapstructure:"root_address"`
		RootPrivateKey string `mapstructure:"root_private_key"`
	} `mapstructure:"test"`
}
