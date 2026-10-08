package taskengine

import (
	"testing"
	"time"
)

func TestExecutionStatusNextActionAt(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	scan := time.Second
	advance := 5 * time.Second
	maxInterval := 15 * time.Second
	overrun := 5 * time.Second

	tests := []struct {
		name      string
		estimated time.Time
		want      time.Time
	}{
		{
			name:      "far estimate is capped by max interval",
			estimated: now.Add(15 * time.Minute),
			want:      now.Add(maxInterval),
		},
		{
			name:      "near estimate is not capped",
			estimated: now.Add(20 * time.Second),
			want:      now.Add(20*time.Second - advance),
		},
		{
			name:      "past estimate uses overrun interval",
			estimated: now.Add(-time.Minute),
			want:      now.Add(overrun),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executionStatusNextActionAt(now, tt.estimated, scan, advance, maxInterval, overrun)
			if !got.Equal(tt.want) {
				t.Fatalf("next action = %v, want %v", got, tt.want)
			}
		})
	}
}
