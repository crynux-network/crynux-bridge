package llm

import (
	"encoding/json"
	"testing"
)

func TestNormalizeAssistantContentParsesHermesJSONToolCall(t *testing.T) {
	content := `<tool_call>
{"name":"write_file","arguments":{"path":"~/hello.py","content":"print(\"hello\")\n"}}
</tool_call>`

	cleanContent, toolCalls := normalizeAssistantContent(content)
	if cleanContent != "" {
		t.Fatalf("expected tool call content to be cleared, got %q", cleanContent)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "write_file" {
		t.Fatalf("unexpected tool name: %s", toolCalls[0].Name)
	}

	var args map[string]string
	if err := json.Unmarshal([]byte(toolCalls[0].Arguments), &args); err != nil {
		t.Fatalf("unexpected arguments JSON error: %v", err)
	}
	if args["path"] != "~/hello.py" || args["content"] != "print(\"hello\")\n" {
		t.Fatalf("unexpected parsed arguments: %#v", args)
	}
}

func TestNormalizeAssistantContentParsesQwenXMLToolCallAfterThinking(t *testing.T) {
	content := `The user wants a simple Python script.
</think>

<tool_call>
<function=write_file>
<parameter=path>
~/hello_world.py
</parameter>
<parameter=content>
print("Hello, World!")

</parameter>
</function>
</tool_call>`

	cleanContent, toolCalls := normalizeAssistantContent(content)
	if cleanContent != "" {
		t.Fatalf("expected tool call content to be cleared, got %q", cleanContent)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "write_file" {
		t.Fatalf("unexpected tool name: %s", toolCalls[0].Name)
	}

	var args map[string]string
	if err := json.Unmarshal([]byte(toolCalls[0].Arguments), &args); err != nil {
		t.Fatalf("unexpected arguments JSON error: %v", err)
	}
	if args["path"] != "~/hello_world.py" {
		t.Fatalf("unexpected path argument: %q", args["path"])
	}
	if args["content"] != "print(\"Hello, World!\")" {
		t.Fatalf("unexpected content argument: %q", args["content"])
	}
}

func TestNormalizeAssistantContentParsesMultipleQwenXMLToolCalls(t *testing.T) {
	content := `<tool_call>
<function=get_weather>
<parameter=location>Paris</parameter>
</function>
</tool_call>

<tool_call>
<function=get_time>
<parameter=timezone>Europe/Paris</parameter>
</function>
</tool_call>`

	_, toolCalls := normalizeAssistantContent(content)
	if len(toolCalls) != 2 {
		t.Fatalf("expected two tool calls, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "get_weather" || toolCalls[1].Name != "get_time" {
		t.Fatalf("unexpected tool call names: %#v", toolCalls)
	}
}

func TestNormalizeAssistantContentStripsThinkingWithoutToolCall(t *testing.T) {
	content := "I should answer briefly.\n</thinking>\n\nHello there."

	cleanContent, toolCalls := normalizeAssistantContent(content)
	if len(toolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %d", len(toolCalls))
	}
	if cleanContent != "Hello there." {
		t.Fatalf("unexpected clean content: %q", cleanContent)
	}
}

func TestNormalizeAssistantContentRecoversQwenXMLWithTrailingDrift(t *testing.T) {
	content := `<tool_call>
noise
<function=get_weather>
<parameter=location>Paris</parameter>
</function>
</function>
</function_invocation>
</tool_call>`

	_, toolCalls := normalizeAssistantContent(content)
	if len(toolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "get_weather" {
		t.Fatalf("unexpected tool name: %s", toolCalls[0].Name)
	}
}
