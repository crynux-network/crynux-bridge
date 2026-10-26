package taskengine

import (
	"crynux_bridge/models"
	"testing"
	"time"
)

func TestStatusSyncNextActionAt(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	statusPoll := time.Second
	scan := time.Second
	advance := time.Second
	maxInterval := time.Second
	overrun := time.Second
	estimated := now.Add(30 * time.Second)

	tests := []struct {
		name       string
		status     models.TaskStatus
		estimated  *time.Time
		wantEqual  time.Time
		wantImmediate bool
	}{
		{name: "score ready is immediate", status: models.InferenceTaskScoreReady, wantImmediate: true},
		{name: "error reported is immediate", status: models.InferenceTaskErrorReported, wantImmediate: true},
		{name: "end success is immediate", status: models.InferenceTaskEndSuccess, wantImmediate: true},
		{name: "validated keeps status poll", status: models.InferenceTaskValidated, wantEqual: now.Add(statusPoll)},
		{name: "started without estimate keeps status poll", status: models.InferenceTaskStarted, wantEqual: now.Add(statusPoll)},
		{name: "created keeps status poll", status: models.InferenceTaskCreated, wantEqual: now.Add(statusPoll)},
		{
			name:      "started with estimate uses execution schedule",
			status:    models.InferenceTaskStarted,
			estimated: &estimated,
			wantEqual: executionStatusNextActionAt(now, estimated, scan, advance, maxInterval, overrun),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusSyncNextActionAt(
				now,
				tt.status,
				tt.estimated,
				statusPoll,
				scan,
				advance,
				maxInterval,
				overrun,
			)
			want := tt.wantEqual
			if tt.wantImmediate {
				want = now
			}
			if !got.Equal(want) {
				t.Fatalf("next action = %v, want %v", got, want)
			}
		})
	}
}

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
