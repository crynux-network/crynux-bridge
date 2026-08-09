package llm

import (
	"crynux_bridge/llmtask"
	"crynux_bridge/models"
)

func buildTemplateToolCallMessages(model string, messages []models.Message) (any, error) {
	return llmtask.BuildTemplateToolCallMessages(model, messages)
}
