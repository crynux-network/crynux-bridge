package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ParseTaskArgsVersion reads TaskArgs.version and requires major.minor.patch numeric form.
func ParseTaskArgsVersion(taskArgs string) (string, error) {
	var args struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(taskArgs), &args); err != nil {
		return "", fmt.Errorf("invalid task args: %w", err)
	}
	if args.Version == "" {
		return "", errors.New("task args version is missing")
	}
	parts := strings.Split(args.Version, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid task version %q", args.Version)
	}
	for _, part := range parts {
		if _, err := strconv.ParseUint(part, 10, 64); err != nil {
			return "", fmt.Errorf("invalid task version %q", args.Version)
		}
	}
	return args.Version, nil
}
