package llm

import (
	"crynux_bridge/config"
	"encoding/json"
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

const defaultLLMAPIRequestLogPath = "/app/data/logs/crynux_bridge_llm_api_requests.log"

var (
	llmFeatureLogWriter     io.Writer
	initLLMFeatureLogWriter sync.Once
	llmFeatureLogWriterErr  error
	llmFeatureLogWriteLock  sync.Mutex
)

func logOpenAICompatibleExchange(api string, authorization string, request any, response any, logErr error) {
	if !isLLMAPIRequestLogEnabled() {
		return
	}

	writer, err := getLLMFeatureLogWriter()
	if err != nil {
		logrus.WithError(err).Error("failed to initialize llm feature log writer")
		return
	}

	requestText := serializeLogValue(request)
	apiLabel := normalizeAPILabel(api)
	maskedAPIKey := maskAuthorizationKey(authorization)
	timestamp := time.Now().Format(time.RFC3339)

	line := fmt.Sprintf("[%s] [INFO] [LLM API Request] [%s] [API Key %s] request=%s", timestamp, apiLabel, maskedAPIKey, requestText)
	if logErr != nil {
		line = fmt.Sprintf("%s, error=%s", line, sanitizeSingleLine(logErr.Error()))
	} else {
		line = fmt.Sprintf("%s, response=%s", line, serializeLogValue(response))
	}

	llmFeatureLogWriteLock.Lock()
	defer llmFeatureLogWriteLock.Unlock()
	if _, err := io.WriteString(writer, line+"\n"); err != nil {
		logrus.WithError(err).Error("failed to write llm feature log")
	}
}

func isLLMAPIRequestLogEnabled() bool {
	appConfig := config.GetConfig()
	return appConfig != nil && appConfig.Log.Features.LLMAPIRequestLogEnabled
}

func getLLMFeatureLogWriter() (io.Writer, error) {
	initLLMFeatureLogWriter.Do(func() {
		appConfig := config.GetConfig()

		outputPath := defaultLLMAPIRequestLogPath
		if appConfig != nil && appConfig.Log.Output != "" && appConfig.Log.Output != "stdout" && appConfig.Log.Output != "stderr" {
			outputPath = filepath.Join(filepath.Dir(appConfig.Log.Output), "crynux_bridge_llm_api_requests.log")
		}

		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			llmFeatureLogWriterErr = err
			return
		}

		fileWriter := &lumberjack.Logger{
			Filename:   outputPath,
			MaxSize:    getLogRotateMaxSize(appConfig),
			MaxAge:     getLogRotateMaxDays(appConfig),
			MaxBackups: getLogRotateMaxFiles(appConfig),
			Compress:   true,
		}
		llmFeatureLogWriter = fileWriter
	})

	return llmFeatureLogWriter, llmFeatureLogWriterErr
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

func sanitizeSingleLine(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "\n", "\\n"), "\r", "\\r")
}
