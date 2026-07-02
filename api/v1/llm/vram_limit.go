package llm

import (
	"fmt"
	"strconv"
	"strings"
)

func resolveMinVram(bodyVramLimit *uint64, pathVramLimit string) (uint64, error) {
	minVram := uint64(24)
	if bodyVramLimit != nil {
		minVram = *bodyVramLimit
	}

	trimmedPathVramLimit := strings.TrimSpace(pathVramLimit)
	if trimmedPathVramLimit == "" {
		return minVram, nil
	}

	pathMinVram, err := strconv.ParseUint(trimmedPathVramLimit, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("vram_limit must be an unsigned integer")
	}
	return pathMinVram, nil
}
