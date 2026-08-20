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

	Admin struct {
		AuthToken         string `mapstructure:"auth_token"`
		TaskTraceMaxTasks int    `mapstructure:"task_trace_max_tasks"`
	} `mapstructure:"admin"`

	Http struct {
		Host         string `mapstructure:"host"`
		Port         string `mapstructure:"port"`
		MaxBodyBytes int64  `mapstructure:"max_body_bytes"`
	} `mapstructure:"http"`

	DataDir struct {
		InferenceTasks              string `mapstructure:"inference_tasks"`
		InferenceTasksRetentionDays int    `mapstructure:"inference_tasks_retention_days"`
		ModelImages                 string `mapstructure:"model_images"`
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
		DefaultSDTaskFeeCNX           float64              `mapstructure:"default_sd_task_fee_cnx"`
		DefaultSDXLTaskFeeCNX         float64              `mapstructure:"default_sd_xl_task_fee_cnx"`
		DefaultLLMTaskFeeCNX          float64              `mapstructure:"default_llm_task_fee_cnx"`
		DefaultLLMMaxCompletionTokens int                  `mapstructure:"default_llm_max_completion_tokens"`
		DefaultSDFinetuneTaskFeeCNX   float64              `mapstructure:"default_sd_finetune_task_fee_cnx"`
		RepeatNum                     int                  `mapstructure:"repeat_num"`
		SDFinetuneTimeout             uint64               `mapstructure:"sd_finetune_timeout"`
		DefaultTaskVersion            string               `mapstructure:"default_task_version"`
		HeartbeatTasks                HeartbeatTasksConfig `mapstructure:"heartbeat_tasks"`
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
	BatchSize       uint64                `mapstructure:"batch_size"`
	MaxTasksPerHour uint64                `mapstructure:"max_tasks_per_hour"`
	TasksFile       string                `mapstructure:"tasks_file"`
	Tasks           []HeartbeatTaskConfig `mapstructure:"tasks" json:"tasks"`
}

type HeartbeatContentBlock struct {
	Type      string `mapstructure:"type" json:"type"`
	Text      string `mapstructure:"text" json:"text,omitempty"`
	Base64    string `mapstructure:"base64" json:"base64,omitempty"`
	ImagePath string `mapstructure:"image_path" json:"image_path,omitempty"`
}

type HeartbeatMessageToolCallFunction struct {
	Name      string `mapstructure:"name" json:"name"`
	Arguments string `mapstructure:"arguments" json:"arguments"`
}

type HeartbeatMessageToolCall struct {
	ID       string                           `mapstructure:"id" json:"id"`
	Type     string                           `mapstructure:"type" json:"type"`
	Function HeartbeatMessageToolCallFunction `mapstructure:"function" json:"function"`
}

type HeartbeatMessageConfig struct {
	Role       string                     `mapstructure:"role" json:"role"`
	Content    any                        `mapstructure:"content" json:"content,omitempty"`
	ToolCallID string                     `mapstructure:"tool_call_id" json:"tool_call_id,omitempty"`
	ToolCalls  []HeartbeatMessageToolCall `mapstructure:"tool_calls" json:"tool_calls,omitempty"`
}

type HeartbeatPromptConfig struct {
	Text           string                   `mapstructure:"text" json:"text,omitempty"`
	NegativePrompt string                   `mapstructure:"negative_prompt" json:"negative_prompt,omitempty"`
	Content        []HeartbeatContentBlock  `mapstructure:"content" json:"content,omitempty"`
	Messages       []HeartbeatMessageConfig `mapstructure:"messages" json:"messages,omitempty"`
}

type HeartbeatTaskConfig struct {
	TaskVersion     string                   `mapstructure:"task_version" json:"task_version"`
	Type            string                   `mapstructure:"type" json:"type"`
	Ratio           float64                  `mapstructure:"ratio" json:"ratio"`
	Model           string                   `mapstructure:"model" json:"model"`
	MinVram         uint64                   `mapstructure:"min_vram" json:"min_vram"`
	FeeCNX          float64                  `mapstructure:"fee_cnx" json:"fee_cnx"`
	MaxPendingTasks uint64                   `mapstructure:"max_pending_tasks" json:"max_pending_tasks"`
	MaxNewTokens    uint64                   `mapstructure:"max_new_tokens" json:"max_new_tokens,omitempty"`
	Steps           uint64                   `mapstructure:"steps" json:"steps,omitempty"`
	Tools           []map[string]interface{} `mapstructure:"tools" json:"tools,omitempty"`
	Prompts         []HeartbeatPromptConfig  `mapstructure:"prompts" json:"prompts,omitempty"`
}
