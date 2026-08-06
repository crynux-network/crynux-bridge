package tasks

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunSDFTTaskUntilCompleteUsesCallerContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := runSDFTTaskUntilComplete(ctx, func() error {
		return errors.New("retry")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Fatalf("caller context was not observed promptly: %s", elapsed)
	}
}
