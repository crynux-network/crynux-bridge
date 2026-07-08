package llm

import (
	"crynux_bridge/api/v1/llm/structs"
	"crynux_bridge/models"
	"encoding/json"
	"testing"
)

func TestBuildLLMTaskArgsPayloadConvertsQwenXMLToolCallArguments(t *testing.T) {
	for _, model := range []string{"Qwen/Qwen3.5-27B", "Qwen/Qwen3.6-27B"} {
		t.Run(model, func(t *testing.T) {
			payload, err := buildTestLLMTaskArgsPayload(testGPTTaskArgs(model, testTools()))
			if err != nil {
				t.Fatalf("unexpected payload error: %v", err)
			}

			arguments := taskPayloadToolCallArguments(t, payload)
			argumentsObject, ok := arguments.(map[string]any)
			if !ok {
				t.Fatalf("expected arguments object, got %#v", arguments)
			}
			if argumentsObject["url"] != "https://example.com" {
				t.Fatalf("unexpected arguments object: %#v", argumentsObject)
			}
		})
	}
}

func TestBuildLLMTaskArgsPayloadSkipsNonQwenXMLToolCallModels(t *testing.T) {
	for _, model := range []string{"Qwen/Qwen3-8B", "Qwen/Qwen2.5-72B", "DeepSeek-V3.2"} {
		t.Run(model, func(t *testing.T) {
			payload, err := buildTestLLMTaskArgsPayload(testGPTTaskArgs(model, testTools()))
			if err != nil {
				t.Fatalf("unexpected payload error: %v", err)
			}

			arguments := taskPayloadToolCallArguments(t, payload)
			if arguments != `{"url":"https://example.com"}` {
				t.Fatalf("expected arguments string, got %#v", arguments)
			}
		})
	}
}

func TestBuildLLMTaskArgsPayloadSkipsRequestsWithoutTools(t *testing.T) {
	payload, err := buildTestLLMTaskArgsPayload(testGPTTaskArgs("Qwen/Qwen3.6-27B", nil))
	if err != nil {
		t.Fatalf("unexpected payload error: %v", err)
	}

	arguments := taskPayloadToolCallArguments(t, payload)
	if arguments != `{"url":"https://example.com"}` {
		t.Fatalf("expected arguments string, got %#v", arguments)
	}
}

func TestBuildLLMTaskArgsPayloadRejectsInvalidQwenToolCallArguments(t *testing.T) {
	args := testGPTTaskArgs("Qwen/Qwen3.6-27B", testTools())
	args.Messages[1].ToolCalls[0].Function.Arguments = "not json"

	if _, err := buildTestLLMTaskArgsPayload(args); err == nil {
		t.Fatalf("expected invalid arguments error")
	}
}

func TestBuildLLMTaskArgsPayloadRejectsNonObjectQwenToolCallArguments(t *testing.T) {
	args := testGPTTaskArgs("Qwen/Qwen3.6-27B", testTools())
	args.Messages[1].ToolCalls[0].Function.Arguments = `["not","object"]`

	if _, err := buildTestLLMTaskArgsPayload(args); err == nil {
		t.Fatalf("expected non-object arguments error")
	}
}

func buildTestLLMTaskArgsPayload(args models.GPTTaskArgs) (llmTaskArgsPayload, error) {
	taskMessages := any(args.Messages)
	if shouldAdaptQwenXMLToolCallMessages(args.Model, args.Tools) {
		adaptedMessages, err := buildQwenXMLToolCallMessages(args.Messages)
		if err != nil {
			return llmTaskArgsPayload{}, err
		}
		taskMessages = adaptedMessages
	}
	return buildLLMTaskArgsPayload(args, taskMessages), nil
}

func testGPTTaskArgs(model string, tools []map[string]interface{}) models.GPTTaskArgs {
	return models.GPTTaskArgs{
		Model: model,
		Messages: []models.Message{
			{
				Role:    models.LLMRoleUser,
				Content: "inspect this url",
			},
			{
				Role:    models.LLMRoleAssistant,
				Content: "",
				ToolCalls: []structs.ToolCall{
					{
						Id:   "call_1",
						Type: "function",
						Function: structs.FunctionCall{
							Name:      "web_extract",
							Arguments: `{"url":"https://example.com"}`,
						},
					},
				},
			},
		},
		Tools: tools,
		GenerationConfig: &models.GPTGenerationConfig{
			MaxNewTokens: 128,
		},
		TemplateArgs: buildLLMTemplateArgs(model, tools),
		Seed:         42,
		DType:        models.DTypeAuto,
	}
}

func testTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name": "web_extract",
			},
		},
	}
}

func taskPayloadToolCallArguments(t *testing.T, payload llmTaskArgsPayload) any {
	t.Helper()

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var payloadMap map[string]any
	if err := json.Unmarshal(payloadBytes, &payloadMap); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	messages := payloadMap["messages"].([]any)
	assistantMessage := messages[1].(map[string]any)
	toolCalls := assistantMessage["tool_calls"].([]any)
	toolCall := toolCalls[0].(map[string]any)
	function := toolCall["function"].(map[string]any)
	return function["arguments"]
}
