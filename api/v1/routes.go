package v1

import (
	apikey "crynux_bridge/api/v1/api_key"
	"crynux_bridge/api/v1/application"
	"crynux_bridge/api/v1/count"
	"crynux_bridge/api/v1/image"
	"crynux_bridge/api/v1/inference_tasks"
	"crynux_bridge/api/v1/llm"
	"crynux_bridge/api/v1/models"
	"crynux_bridge/api/v1/network"
	"crynux_bridge/api/v1/response"

	"github.com/loopfz/gadgeto/tonic"
	"github.com/wI2L/fizz"
)

func InitRoutes(r *fizz.Fizz) {

	v1g := r.Group("v1", "ApiV1", "API version 1")

	tasksGroup := v1g.Group("inference_tasks", "Inference tasks", "Inference tasks related APIs")

	tasksGroup.POST("", []fizz.OperationOption{
		fizz.Summary("Create an inference task"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(inference_tasks.CreateTask, 200))

	tasksGroup.GET("/:client_id/:client_task_id", []fizz.OperationOption{
		fizz.Summary("Get task details by task id"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(inference_tasks.GetTaskById, 200))

	tasksGroup.GET("/:client_id/:client_task_id/images/:index", []fizz.OperationOption{
		fizz.Summary("Get task details by task id"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(inference_tasks.GetTaskImage, 200))

	modelsGroup := v1g.Group("models", "Models", "Models related APIs")

	modelsGroup.GET("base", []fizz.OperationOption{
		fizz.Summary("Get the list of the base models"),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(models.GetBaseModels, 200))

	modelsGroup.GET("lora", []fizz.OperationOption{
		fizz.Summary("Get the list of the lora models"),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(models.GetLoraModels, 200))

	networkGroup := v1g.Group("network", "Network", "Network status")
	networkGroup.GET("nodes", []fizz.OperationOption{}, tonic.Handler(network.GetNodeStats, 200))

	applicationGroup := v1g.Group("application", "Application", "Application related APIs")
	applicationGroup.GET("/wallet/balance", []fizz.OperationOption{
		fizz.Summary("Get the balance of the application wallet"),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(application.GetWalletBalance, 200))

	countGroup := v1g.Group("count", "Count", "Task count related APIs")
	countGroup.GET("/task", []fizz.OperationOption{
		fizz.Summary("Count the task in the recent period"),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(count.CountTask, 200))

	// for openrouter, api: /completions and /chat/completions
	openrouterGroup := v1g.Group("openrouter", "OpenRouter", "OpenRouter related APIs")

	openrouterGroup.POST("/completions", []fizz.OperationOption{
		fizz.ID("openrouter_completions"),
		fizz.Summary("Api for openrouter, /completions"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(llm.Completions, 200))

	openrouterGroup.POST("/chat/completions", []fizz.OperationOption{
		fizz.ID("openrouter_chat_completions"),
		fizz.Summary("Api for openrouter, /chat/completions"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(llm.ChatCompletions, 200))
	openrouterGroup.GET("/models", []fizz.OperationOption{
		fizz.Summary("Get the list of the models"),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(models.GetOpenrouterModels, 200))

	llmGroup := v1g.Group("llm", "LLM", "LLM related APIs")
	llmGroup.POST("/completions", []fizz.OperationOption{
		fizz.ID("llm_completions"),
		fizz.Summary("Api for llm, /completions"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(llm.Completions, 200))
	llmGroup.POST("/:vram_limit/completions", []fizz.OperationOption{
		fizz.ID("llm_completions_with_vram_limit"),
		fizz.Summary("Api for llm with VRAM limit in URL, /:vram_limit/completions"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(llm.Completions, 200))

	llmGroup.POST("/chat/completions", []fizz.OperationOption{
		fizz.ID("llm_chat_completions"),
		fizz.Summary("Api for openrouter, /chat/completions"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(llm.ChatCompletions, 200))
	llmGroup.POST("/:vram_limit/chat/completions", []fizz.OperationOption{
		fizz.ID("llm_chat_completions_with_vram_limit"),
		fizz.Summary("Api for llm with VRAM limit in URL, /:vram_limit/chat/completions"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(llm.ChatCompletions, 200))

	imagesGroup := v1g.Group("images", "Images", "Images related APIs")
	imagesGroup.POST("", []fizz.OperationOption{
		fizz.ID("images_generations"),
		fizz.Summary("Api for image generations"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(image.CreateImage, 200))
	imagesGroup.POST("/models", []fizz.OperationOption{
		fizz.ID("images_models"),
		fizz.Summary("Api for finetune lora model for image generations"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(image.CreateSDFinetuneLoraTask, 200))
	imagesGroup.GET("/models/:id/status", []fizz.OperationOption{
		fizz.Summary("Get the status of a finetuning image lora model task"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(image.GetSDFinetuneLoraTaskStatus, 200))
	imagesGroup.GET("/models/:id/result", []fizz.OperationOption{
		fizz.Summary("Get the result of a finetuning image lora model task"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(image.DownloadSDFinetuneLoraTaskResult, 200))

	apiKeyGroup := v1g.Group("api_key", "API Key", "API Key related APIs")
	apiKeyGroup.POST("", []fizz.OperationOption{
		fizz.Summary("Generate a new API key"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(apikey.CreateAPIKey, 200))
	apiKeyGroup.DELETE("/:api_key", []fizz.OperationOption{
		fizz.Summary("Delete an API key"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(apikey.DeleteAPIKey, 200))
	apiKeyGroup.POST("/:api_key/role", []fizz.OperationOption{
		fizz.Summary("Add a role to an API key"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(apikey.AddRole, 200))
	apiKeyGroup.POST("/:api_key/use_limit", []fizz.OperationOption{
		fizz.Summary("Change the use limit of an API key"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(apikey.ChangeUseLimit, 200))
	apiKeyGroup.POST("/:api_key/rate_limit", []fizz.OperationOption{
		fizz.Summary("Change the rate limit of an API key"),
		fizz.Response("400", "validation errors", response.ValidationErrorResponse{}, nil, nil),
		fizz.Response("500", "exception", response.ExceptionResponse{}, nil, nil),
	}, tonic.Handler(apikey.ChangeRateLimit, 200))
}
