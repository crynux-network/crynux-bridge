package llm

import (
	"crynux_bridge/api/v1/llm/structs"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStreamChatCompletionsResponsePreservesContentAndFinishReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	response := &structs.ChatCompletionsResponse{
		Id:      "task-1",
		Created: 1,
		Model:   "test/model",
		Choices: []structs.CCResChoice{
			{
				Index: 0,
				Message: structs.CCResMessage{
					Role:    structs.ChatCompletionsRoleAssistant,
					Content: "ordinary answer",
				},
				FinishReason: "stop",
			},
		},
	}

	if err := streamChatCompletionsResponse(context, response, false); err != nil {
		t.Fatalf("unexpected streaming error: %v", err)
	}

	events := decodeChatCompletionEvents(t, recorder.Body.String())
	if len(events) != 3 {
		t.Fatalf("expected role, content, and finish events, got %d", len(events))
	}
	if events[1].Choices[0].Delta.Content != "ordinary answer" {
		t.Fatalf("unexpected streamed content: %q", events[1].Choices[0].Delta.Content)
	}
	if events[2].Choices[0].FinishReason == nil || *events[2].Choices[0].FinishReason != "stop" {
		t.Fatalf("unexpected finish reason: %#v", events[2].Choices[0].FinishReason)
	}
}

func TestStreamChatCompletionsResponseEmitsMultipleToolCallsAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	response := &structs.ChatCompletionsResponse{
		Id:      "task-2",
		Created: 2,
		Model:   "test/model",
		Choices: []structs.CCResChoice{
			{
				Index: 0,
				Message: structs.CCResMessage{
					Role:    structs.ChatCompletionsRoleAssistant,
					Content: "kept text",
					ToolCalls: []structs.ToolCall{
						{
							Id:   "call-1",
							Type: "function",
							Function: structs.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location":"Paris"}`,
							},
						},
						{
							Id:   "call-2",
							Type: "function",
							Function: structs.FunctionCall{
								Name:      "get_time",
								Arguments: `{"timezone":"Europe/Paris"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: structs.CCResUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	if err := streamChatCompletionsResponse(context, response, true); err != nil {
		t.Fatalf("unexpected streaming error: %v", err)
	}

	events := decodeChatCompletionEvents(t, recorder.Body.String())
	if len(events) != 5 {
		t.Fatalf("expected role, content, tool-call, finish, and usage events, got %d", len(events))
	}
	if events[1].Choices[0].Delta.Content != "kept text" {
		t.Fatalf("unexpected streamed content: %q", events[1].Choices[0].Delta.Content)
	}
	toolCalls := events[2].Choices[0].Delta.ToolCalls
	if len(toolCalls) != 2 {
		t.Fatalf("expected two tool calls, got %d", len(toolCalls))
	}
	if toolCalls[0].Function.Name != "get_weather" || toolCalls[1].Function.Name != "get_time" {
		t.Fatalf("unexpected tool calls: %#v", toolCalls)
	}
	if events[3].Choices[0].FinishReason == nil || *events[3].Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("unexpected finish reason: %#v", events[3].Choices[0].FinishReason)
	}
	if events[4].Usage == nil || events[4].Usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage event: %#v", events[4].Usage)
	}
}

func decodeChatCompletionEvents(t *testing.T, body string) []chatCompletionChunk {
	t.Helper()

	var events []chatCompletionChunk
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		var event chatCompletionChunk
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatalf("failed to decode event %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}
