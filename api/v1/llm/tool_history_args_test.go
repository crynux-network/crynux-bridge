package llm

import (
	"crynux_bridge/api/v1/llm/structs"
	"crynux_bridge/models"
	"encoding/json"
	"testing"
)

func TestBuildTemplateToolCallMessagesUsesObjectsByDefault(t *testing.T) {
	models := []string{
		"Qwen/Qwen2.5-72B",
		"Qwen/Qwen3-8B",
		"Qwen/Qwen3.5-27B",
		"Qwen/Qwen3.6-27B",
		"meta-llama/Llama-3.3-70B-Instruct",
		"acme/unknown-tools-model",
		"deepseek-ai/DeepSeek-R1-Distill-Qwen-32B",
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			payload, err := buildTestLLMTaskArgsPayload(testGPTTaskArgs(model, testTools()))
			if err != nil {
				t.Fatalf("unexpected payload error: %v", err)
			}

			arguments := taskPayloadToolCallArguments(t, payload, 0)
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

func TestBuildTemplateToolCallMessagesPreservesDeepSeekStrings(t *testing.T) {
	models := []string{
		"deepseek-ai/DeepSeek-V3",
		"deepseek-ai/DeepSeek-V3.1",
		"deepseek-ai/DeepSeek-V3.2",
		"deepseek_ai/deepseek_v3_2",
		"deepseek-ai/DeepSeek-R1",
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			payload, err := buildTestLLMTaskArgsPayload(testGPTTaskArgs(model, testTools()))
			if err != nil {
				t.Fatalf("unexpected payload error: %v", err)
			}

			arguments := taskPayloadToolCallArguments(t, payload, 0)
			if arguments != `{"url":"https://example.com"}` {
				t.Fatalf("expected arguments string, got %#v", arguments)
			}
		})
	}
}

func TestBuildTemplateToolCallMessagesConvertsHistoryWithoutCurrentTools(t *testing.T) {
	payload, err := buildTestLLMTaskArgsPayload(testGPTTaskArgs("Qwen/Qwen3-8B", nil))
	if err != nil {
		t.Fatalf("unexpected payload error: %v", err)
	}

	if _, ok := taskPayloadToolCallArguments(t, payload, 0).(map[string]any); !ok {
		t.Fatal("expected arguments object without current tools")
	}
}

func TestBuildTemplateToolCallMessagesLeavesMessagesWithoutHistoryUnchanged(t *testing.T) {
	messages := []models.Message{{Role: models.LLMRoleUser, Content: "hello"}}

	adapted, err := buildTemplateToolCallMessages("Qwen/Qwen3-8B", messages)
	if err != nil {
		t.Fatalf("unexpected adaptation error: %v", err)
	}
	if _, ok := adapted.([]models.Message); !ok {
		t.Fatalf("expected original message representation, got %T", adapted)
	}
}

func TestBuildTemplateToolCallMessagesPreservesNestedValuesAndCallOrder(t *testing.T) {
	args := testGPTTaskArgs("meta-llama/Llama-3.1-8B-Instruct", testTools())
	args.Messages[1].ToolCalls[0].Function.Arguments = `{"filters":{"tags":["news","ai"]},"limit":2}`
	args.Messages[1].ToolCalls = append(args.Messages[1].ToolCalls, structs.ToolCall{
		Id:   "call_2",
		Type: "function",
		Function: structs.FunctionCall{
			Name:      "summarize",
			Arguments: `{"format":"short"}`,
		},
	})

	payload, err := buildTestLLMTaskArgsPayload(args)
	if err != nil {
		t.Fatalf("unexpected payload error: %v", err)
	}

	first := taskPayloadToolCallArguments(t, payload, 0).(map[string]any)
	filters := first["filters"].(map[string]any)
	tags := filters["tags"].([]any)
	if tags[0] != "news" || tags[1] != "ai" || first["limit"] != float64(2) {
		t.Fatalf("unexpected nested arguments: %#v", first)
	}

	second := taskPayloadToolCallArguments(t, payload, 1).(map[string]any)
	if second["format"] != "short" {
		t.Fatalf("unexpected second call arguments: %#v", second)
	}
}

func TestBuildTemplateToolCallMessagesRejectsInvalidArgumentsForAllRepresentations(t *testing.T) {
	for _, model := range []string{"Qwen/Qwen3-8B", "deepseek-ai/DeepSeek-V3.2"} {
		for _, arguments := range []string{"not json", "null", `["not","object"]`, `"scalar"`, "42"} {
			t.Run(model+"/"+arguments, func(t *testing.T) {
				args := testGPTTaskArgs(model, testTools())
				args.Messages[1].ToolCalls[0].Function.Arguments = arguments

				if _, err := buildTestLLMTaskArgsPayload(args); err == nil {
					t.Fatalf("expected invalid arguments error for %q", arguments)
				}
			})
		}
	}
}

func buildTestLLMTaskArgsPayload(args models.GPTTaskArgs) (llmTaskArgsPayload, error) {
	taskMessages, err := buildTemplateToolCallMessages(args.Model, args.Messages)
	if err != nil {
		return llmTaskArgsPayload{}, err
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
		Seed:  42,
		DType: models.DTypeAuto,
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

func taskPayloadToolCallArguments(t *testing.T, payload llmTaskArgsPayload, index int) any {
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
	toolCall := toolCalls[index].(map[string]any)
	function := toolCall["function"].(map[string]any)
	return function["arguments"]
}
