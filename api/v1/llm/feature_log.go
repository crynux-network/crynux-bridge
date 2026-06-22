package llm

import (
	"crynux_bridge/config"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	llmFeatureAPIRequests       = "llm_api_requests"
	llmFeatureAPIRequestTools   = "llm_api_request_toolcalls"
	llmFeatureLogLevelInfo      = "INFO"
	llmFeatureLogLevelError     = "ERROR"
	llmFeatureLogInfoFileLevel  = "info"
	llmFeatureLogErrorFileLevel = "error"
	llmFeatureLogEntrySeparator = "\n\n"
)

var (
	llmFeatureLogWriters    = map[string]io.Writer{}
	llmFeatureLogWriterLock sync.Mutex
	llmFeatureLogWriteLock  sync.Mutex
)

func logOpenAICompatibleExchange(api string, authorization string, taskIDCommitment string, request any, response any, logErr error, elapsedSeconds float64) {
	if !isLLMAPIRequestLogEnabled() {
		return
	}

	logLevel := llmFeatureLogLevelInfo
	if logErr != nil {
		logLevel = llmFeatureLogLevelError
	}

	writer, err := getLLMFeatureLogWriter(llmFeatureAPIRequests, logLevel)
	if err != nil {
		logrus.WithError(err).Error("failed to initialize llm feature log writer")
		return
	}

	requestText := serializeLogValue(sanitizeLLMRequestLogPayload(request))
	apiLabel := normalizeAPILabel(api)
	maskedAPIKey := maskAuthorizationKey(authorization)
	timestamp := time.Now().Format(time.RFC3339)

	line := fmt.Sprintf("[%s] [%s] [LLM API Request] [%s] [API Key %s] [Task ID Commitment: %s] duration_seconds=%.3f, request=%s", timestamp, logLevel, apiLabel, maskedAPIKey, taskIDCommitment, elapsedSeconds, requestText)
	if logErr != nil {
		line = fmt.Sprintf("%s, error=%s", line, formatLLMAPIError(logErr))
	} else {
		line = fmt.Sprintf("%s, response=%s", line, serializeLogValue(response))
	}

	llmFeatureLogWriteLock.Lock()
	defer llmFeatureLogWriteLock.Unlock()
	if _, err := io.WriteString(writer, line+llmFeatureLogEntrySeparator); err != nil {
		logrus.WithError(err).Error("failed to write llm feature log")
	}
}

func logOpenAICompatibleToolCallExchange(api string, authorization string, taskIDCommitment string, request any, response any, matchedToolCall bool, logErr error, elapsedSeconds float64) {
	if !isLLMAPIRequestToolCallLogEnabled() {
		return
	}

	logLevel := llmFeatureLogLevelInfo
	logErrorText := ""
	if logErr != nil {
		logLevel = llmFeatureLogLevelError
		logErrorText = formatLLMAPIError(logErr)
	} else if !matchedToolCall {
		logLevel = llmFeatureLogLevelError
		logErrorText = "no tool call matched in llm response"
	}

	writer, err := getLLMFeatureLogWriter(llmFeatureAPIRequestTools, logLevel)
	if err != nil {
		logrus.WithError(err).Error("failed to initialize llm feature log writer")
		return
	}

	requestText := serializeLogValue(sanitizeLLMRequestLogPayload(request))
	apiLabel := normalizeAPILabel(api)
	maskedAPIKey := maskAuthorizationKey(authorization)
	timestamp := time.Now().Format(time.RFC3339)
	line := fmt.Sprintf("[%s] [%s] [LLM API Request Tool Call] [%s] [API Key %s] [Task ID Commitment: %s] duration_seconds=%.3f, request=%s, tool_call_matched=%t", timestamp, logLevel, apiLabel, maskedAPIKey, taskIDCommitment, elapsedSeconds, requestText, matchedToolCall)
	if logErrorText != "" {
		line = fmt.Sprintf("%s, error=%s", line, logErrorText)
	}
	if response != nil {
		line = fmt.Sprintf("%s, response=%s", line, serializeLogValue(response))
	}

	llmFeatureLogWriteLock.Lock()
	defer llmFeatureLogWriteLock.Unlock()
	if _, err := io.WriteString(writer, line+llmFeatureLogEntrySeparator); err != nil {
		logrus.WithError(err).Error("failed to write llm feature log")
	}
}

func isLLMAPIRequestLogEnabled() bool {
	appConfig := config.GetConfig()
	return appConfig != nil && appConfig.Log.Features.LLMAPIRequestLogEnabled
}

func isLLMAPIRequestToolCallLogEnabled() bool {
	appConfig := config.GetConfig()
	return appConfig != nil && appConfig.Log.Features.LLMAPIRequestLogToolCallEnabled
}

func getLLMFeatureLogWriter(feature string, logLevel string) (io.Writer, error) {
	fileLevel := llmFeatureLogInfoFileLevel
	if logLevel == llmFeatureLogLevelError {
		fileLevel = llmFeatureLogErrorFileLevel
	}
	key := feature + "." + fileLevel

	llmFeatureLogWriterLock.Lock()
	defer llmFeatureLogWriterLock.Unlock()

	if writer, ok := llmFeatureLogWriters[key]; ok {
		return writer, nil
	}

	appConfig := config.GetConfig()
	outputPath := getLLMFeatureLogPath(appConfig, feature, fileLevel)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return nil, err
	}

	fileWriter := &lumberjack.Logger{
		Filename:   outputPath,
		MaxSize:    getLogRotateMaxSize(appConfig),
		MaxAge:     getLogRotateMaxDays(appConfig),
		MaxBackups: getLogRotateMaxFiles(appConfig),
		Compress:   true,
	}
	llmFeatureLogWriters[key] = fileWriter

	return fileWriter, nil
}

func getLLMFeatureLogPath(appConfig *config.AppConfig, feature string, fileLevel string) string {
	filename := fmt.Sprintf("crynux_bridge_%s.%s.log", feature, fileLevel)
	if appConfig != nil && appConfig.Log.Output != "" && appConfig.Log.Output != "stdout" && appConfig.Log.Output != "stderr" {
		return filepath.Join(filepath.Dir(appConfig.Log.Output), filename)
	}
	return filepath.Join("/app/data/logs", filename)
}

func getLogRotateMaxSize(appConfig *config.AppConfig) int {
	if appConfig != nil && appConfig.Log.MaxFileSize > 0 {
		return appConfig.Log.MaxFileSize
	}
	return 500
}

func getLogRotateMaxDays(appConfig *config.AppConfig) int {
	if appConfig != nil && appConfig.Log.MaxDays > 0 {
		return appConfig.Log.MaxDays
	}
	return 30
}

func getLogRotateMaxFiles(appConfig *config.AppConfig) int {
	if appConfig != nil && appConfig.Log.MaxFileNum > 0 {
		return appConfig.Log.MaxFileNum
	}
	return 10
}

func normalizeAPILabel(api string) string {
	switch api {
	case "chat_completions":
		return "chat completions"
	case "completions":
		return "completions"
	default:
		return strings.ReplaceAll(api, "_", " ")
	}
}

func maskAuthorizationKey(authorization string) string {
	const bearerPrefix = "Bearer "
	key := strings.TrimSpace(authorization)
	if strings.HasPrefix(key, bearerPrefix) {
		key = strings.TrimSpace(key[len(bearerPrefix):])
	}
	if key == "" {
		return "<empty>"
	}
	if len(key) <= 10 {
		if len(key) <= 4 {
			return "****"
		}
		return key[:2] + "****" + key[len(key)-2:]
	}
	return key[:4] + "..." + key[len(key)-4:]
}

func serializeLogValue(value any) string {
	if value == nil {
		return "null"
	}
	b, err := json.Marshal(value)
	if err != nil {
		return sanitizeSingleLine(fmt.Sprintf("%v", value))
	}
	return sanitizeSingleLine(string(b))
}

func sanitizeLLMRequestLogPayload(value any) any {
	return sanitizeJSONLikeValue(value)
}

func sanitizeJSONLikeValue(value any) any {
	b, err := json.Marshal(value)
	if err != nil {
		return value
	}

	var decoded any
	if err := json.Unmarshal(b, &decoded); err != nil {
		return value
	}
	return sanitizeDecodedLogValue(decoded)
}

func sanitizeDecodedLogValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		sanitized := make(map[string]any, len(typed))
		for key, child := range typed {
			lowerKey := strings.ToLower(key)
			if lowerKey == "tools" {
				sanitized["tools_count"] = countLogArrayItems(child)
				continue
			}
			if isPromptLogField(lowerKey) {
				if text, ok := child.(string); ok {
					sanitized[key] = abbreviateLogText(text)
					continue
				}
			}
			sanitized[key] = sanitizeDecodedLogValue(child)
		}
		return sanitized
	case []any:
		sanitized := make([]any, len(typed))
		for i, child := range typed {
			sanitized[i] = sanitizeDecodedLogValue(child)
		}
		return sanitized
	default:
		return value
	}
}

func countLogArrayItems(value any) int {
	items, ok := value.([]any)
	if !ok {
		return 0
	}
	return len(items)
}

func isPromptLogField(key string) bool {
	return key == "prompt" || key == "content" || key == "text"
}

func abbreviateLogText(text string) string {
	const keepRunes = 100
	runes := []rune(text)
	if len(runes) <= keepRunes*2 {
		return text
	}
	return string(runes[:keepRunes]) + "..." + string(runes[len(runes)-keepRunes:])
}

func sanitizeSingleLine(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "\n", "\\n"), "\r", "\\r")
}

func formatLLMAPIError(err error) string {
	var validationErr interface {
		GetErrorType() string
		GetFieldName() string
		GetFieldMessage() string
	}
	if errors.As(err, &validationErr) {
		return fmt.Sprintf(
			"%s(field=%s, message=%s)",
			sanitizeSingleLine(validationErr.GetErrorType()),
			sanitizeSingleLine(validationErr.GetFieldName()),
			sanitizeSingleLine(validationErr.GetFieldMessage()),
		)
	}

	var exceptionErr interface {
		GetException() string
	}
	if errors.As(err, &exceptionErr) {
		return sanitizeSingleLine(exceptionErr.GetException())
	}

	return sanitizeSingleLine(err.Error())
}
