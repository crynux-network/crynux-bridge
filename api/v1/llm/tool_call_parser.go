package llm

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	thinkingEndRegex      = regexp.MustCompile(`(?i)</think(?:ing)?>`)
	toolCallBlockRegex    = regexp.MustCompile(`(?is)<tool_call>\s*(.*?)\s*</tool_call>`)
	qwenFunctionRegex     = regexp.MustCompile(`(?is)<function=([^\s>]+)>\s*(.*?)\s*</function>`)
	qwenParameterRegex    = regexp.MustCompile(`(?is)<parameter=([^\s>]+)>\s*(.*?)\s*</parameter>`)
	qwenFunctionOpenRegex = regexp.MustCompile(`(?is)<function=([^\s>]+)>`)
)

type parsedLlmToolCall struct {
	Name      string
	Arguments string
}

type parsedHermesToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func normalizeAssistantContent(content string) (string, []parsedLlmToolCall) {
	cleanContent := stripThinkingContent(content)
	toolCalls := parseToolCalls(cleanContent)
	if len(toolCalls) > 0 {
		return "", toolCalls
	}
	return cleanContent, nil
}

func stripThinkingContent(content string) string {
	loc := thinkingEndRegex.FindStringIndex(content)
	if loc == nil {
		return content
	}
	return strings.TrimLeft(content[loc[1]:], "\r\n\t ")
}

func parseToolCalls(content string) []parsedLlmToolCall {
	matches := toolCallBlockRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	toolCalls := make([]parsedLlmToolCall, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		block := strings.TrimSpace(match[1])
		if toolCall, ok := parseHermesJSONToolCall(block); ok {
			toolCalls = append(toolCalls, toolCall)
			continue
		}
		if toolCall, ok := parseQwenXMLToolCall(block); ok {
			toolCalls = append(toolCalls, toolCall)
		}
	}

	if len(toolCalls) == 0 {
		return nil
	}
	return toolCalls
}

func parseHermesJSONToolCall(block string) (parsedLlmToolCall, bool) {
	var parsedArgs parsedHermesToolCall
	if err := json.Unmarshal([]byte(block), &parsedArgs); err != nil {
		return parsedLlmToolCall{}, false
	}
	if parsedArgs.Name == "" || len(parsedArgs.Arguments) == 0 {
		return parsedLlmToolCall{}, false
	}

	finalArgumentsString := string(parsedArgs.Arguments)
	var tempStr string
	if err := json.Unmarshal(parsedArgs.Arguments, &tempStr); err == nil {
		finalArgumentsString = tempStr
	}

	return parsedLlmToolCall{
		Name:      parsedArgs.Name,
		Arguments: finalArgumentsString,
	}, true
}

func parseQwenXMLToolCall(block string) (parsedLlmToolCall, bool) {
	functionMatch := qwenFunctionRegex.FindStringSubmatch(block)
	if len(functionMatch) < 3 {
		functionMatch = recoverQwenFunctionBlock(block)
	}
	if len(functionMatch) < 3 {
		return parsedLlmToolCall{}, false
	}

	args := map[string]any{}
	for _, paramMatch := range qwenParameterRegex.FindAllStringSubmatch(functionMatch[2], -1) {
		if len(paramMatch) < 3 {
			continue
		}
		paramName := strings.TrimSpace(paramMatch[1])
		if paramName == "" {
			continue
		}
		args[paramName] = parseQwenParameterValue(paramMatch[2])
	}
	if len(args) == 0 {
		return parsedLlmToolCall{}, false
	}

	arguments, err := json.Marshal(args)
	if err != nil {
		return parsedLlmToolCall{}, false
	}
	return parsedLlmToolCall{
		Name:      strings.TrimSpace(functionMatch[1]),
		Arguments: string(arguments),
	}, true
}

func recoverQwenFunctionBlock(block string) []string {
	openMatch := qwenFunctionOpenRegex.FindStringSubmatchIndex(block)
	if len(openMatch) == 0 {
		return nil
	}

	name := block[openMatch[2]:openMatch[3]]
	body := block[openMatch[1]:]
	if end := strings.Index(strings.ToLower(body), "</function>"); end >= 0 {
		body = body[:end]
	}
	return []string{"", name, body}
}

func parseQwenParameterValue(raw string) any {
	value := strings.Trim(raw, "\r\n\t ")
	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err == nil {
		return parsed
	}
	return value
}
