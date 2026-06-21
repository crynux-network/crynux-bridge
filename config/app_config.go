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
			LLMAPIRequestLogEnabled         bool `mapstructure:"llm_api_request_log_enabled"`
			LLMAPIRequestLogToolCallEnabled bool `mapstructure:"llm_api_request_log_toolcall_enabled"`
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
		Account struct {
			Address        string `mapstructure:"address"`
			PrivateKey     string `mapstructure:"private_key"`
			PrivateKeyFile string `mapstructure:"private_key_file"`
		} `mapstructure:"account"`
	} `mapstructure:"blockchain"`

	Relay struct {
		BaseURL string `mapstructure:"base_url"`
	} `mapstructure:"relay"`

	Task struct {
		DefaultSDTaskFeeCNX         float64              `mapstructure:"default_sd_task_fee_cnx"`
		DefaultSDXLTaskFeeCNX       float64              `mapstructure:"default_sd_xl_task_fee_cnx"`
		DefaultLLMTaskFeeCNX        float64              `mapstructure:"default_llm_task_fee_cnx"`
		DefaultSDFinetuneTaskFeeCNX float64              `mapstructure:"default_sd_finetune_task_fee_cnx"`
		RepeatNum                   int                  `mapstructure:"repeat_num"`
		DefaultTimeout              uint64               `mapstructure:"default_timeout"`
		SDFinetuneTimeout           uint64               `mapstructure:"sd_finetune_timeout"`
		DefaultTaskVersion          string               `mapstructure:"default_task_version"`
		HeartbeatTasks              HeartbeatTasksConfig `mapstructure:"heartbeat_tasks"`
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

type HeartbeatTasksConfig struct {
	PendingTasksLimit uint64                `mapstructure:"pending_tasks_limit"`
	BatchSize         uint64                `mapstructure:"batch_size"`
	MaxTasksPerHour   uint64                `mapstructure:"max_tasks_per_hour"`
	Tasks             []HeartbeatTaskConfig `mapstructure:"tasks"`
}

type HeartbeatTaskConfig struct {
	TaskVersion string  `mapstructure:"task_version"`
	Type        string  `mapstructure:"type"`
	Ratio       float64 `mapstructure:"ratio"`
	Model       string  `mapstructure:"model"`
	MinVram     uint64  `mapstructure:"min_vram"`
	FeeCNX      float64 `mapstructure:"fee_cnx"`
}
