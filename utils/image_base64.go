package utils

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// ExtractBase64PayloadFromDataURL extracts and validates the base64 payload from a
// data:*;base64,<payload> URL. It returns the raw base64 payload without the data URL prefix.
func ExtractBase64PayloadFromDataURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("image_url.url is required")
	}

	commaIdx := strings.Index(rawURL, ",")
	if commaIdx <= 0 || commaIdx == len(rawURL)-1 {
		return "", fmt.Errorf("image_url.url must be a non-empty data URL with base64 payload")
	}

	metadata := rawURL[:commaIdx]
	payload := rawURL[commaIdx+1:]
	if !strings.HasPrefix(strings.ToLower(metadata), "data:") || !strings.Contains(strings.ToLower(metadata), ";base64") {
		return "", fmt.Errorf("image_url.url must use data:*;base64,<payload> format")
	}

	if err := validateBase64Payload(payload); err != nil {
		return "", fmt.Errorf("image_url.url contains invalid base64 payload")
	}

	return payload, nil
}

// NormalizeImageBase64 accepts raw base64 or a data:*;base64,<payload> URL and returns
// the raw base64 payload. The returned value is validated as decodable base64.
func NormalizeImageBase64(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("base64 is required")
	}

	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		return ExtractBase64PayloadFromDataURL(trimmed)
	}

	if err := validateBase64Payload(trimmed); err != nil {
		return "", fmt.Errorf("invalid base64 payload")
	}
	return trimmed, nil
}

func validateBase64Payload(payload string) error {
	if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
		if _, rawErr := base64.RawStdEncoding.DecodeString(payload); rawErr != nil {
			return fmt.Errorf("invalid base64 payload")
		}
	}
	return nil
}
