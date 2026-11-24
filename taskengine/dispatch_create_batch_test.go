package taskengine

import (
	"crynux_bridge/models"
	"strings"
	"testing"
)

func TestTakeCreateBatchSplitsByBytes(t *testing.T) {
	largeArgs := strings.Repeat("a", 10*1024*1024)
	values := []operationDecision{
		{task: models.InferenceTask{TaskArgs: largeArgs}},
		{task: models.InferenceTask{TaskArgs: largeArgs}},
		{task: models.InferenceTask{TaskArgs: largeArgs}},
	}
	batch, remaining := takeCreateBatch(values, 100, 24*1024*1024)
	if len(batch) != 2 {
		t.Fatalf("first batch size = %d, want 2", len(batch))
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining = %d, want 1", len(remaining))
	}
	batch2, remaining2 := takeCreateBatch(remaining, 100, 24*1024*1024)
	if len(batch2) != 1 || len(remaining2) != 0 {
		t.Fatalf("second batch size = %d remaining = %d", len(batch2), len(remaining2))
	}
}

func TestTakeCreateBatchAllowsOversizedSingleItem(t *testing.T) {
	oversized := strings.Repeat("b", 30*1024*1024)
	values := []operationDecision{
		{task: models.InferenceTask{TaskArgs: oversized}},
		{task: models.InferenceTask{TaskArgs: "small"}},
	}
	batch, remaining := takeCreateBatch(values, 100, 24*1024*1024)
	if len(batch) != 1 {
		t.Fatalf("batch size = %d, want 1", len(batch))
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining = %d, want 1", len(remaining))
	}
}

func TestTakeCreateBatchRespectsItemLimit(t *testing.T) {
	values := []operationDecision{
		{task: models.InferenceTask{TaskArgs: "a"}},
		{task: models.InferenceTask{TaskArgs: "b"}},
		{task: models.InferenceTask{TaskArgs: "c"}},
	}
	batch, remaining := takeCreateBatch(values, 2, 24*1024*1024)
	if len(batch) != 2 || len(remaining) != 1 {
		t.Fatalf("batch=%d remaining=%d", len(batch), len(remaining))
	}
}
