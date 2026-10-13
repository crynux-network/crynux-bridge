package models_test

import (
	"crynux_bridge/models"
	"errors"
	"testing"
)

func groupTestModelFiles() models.ModelFileEntries {
	return models.ModelFileEntries{
		{
			SourceType:   "huggingface",
			Repository:   "org/model",
			Revision:     "0123456789abcdef0123456789abcdef01234567",
			Format:       "diffusers",
			FormatKwargs: map[string]any{"variant": "fp16"},
		},
		{
			SourceType:   "huggingface",
			Repository:   "org/vae",
			Revision:     "89abcdef0123456789abcdef0123456789abcdef",
			Format:       "diffusers",
			FormatKwargs: map[string]any{},
		},
	}
}

func TestModelFileEntriesValueScanRoundTrip(t *testing.T) {
	files := groupTestModelFiles()
	value, err := files.Value()
	if err != nil {
		t.Fatal(err)
	}
	var scanned models.ModelFileEntries
	if err := scanned.Scan(value); err != nil {
		t.Fatal(err)
	}
	wantKey, _ := files.CanonicalKey()
	gotKey, _ := scanned.CanonicalKey()
	if gotKey != wantKey {
		t.Fatalf("scanned entries = %#v", scanned)
	}

	empty, err := models.ModelFileEntries(nil).Value()
	if err != nil || empty != "" {
		t.Fatalf("empty value = %#v, err = %v", empty, err)
	}
	scanned = files
	if err := scanned.Scan(""); err != nil || len(scanned) != 0 {
		t.Fatalf("scan empty = %#v, err = %v", scanned, err)
	}
}

func TestCheckGroupExecutionModelFiles(t *testing.T) {
	files := groupTestModelFiles()
	reordered := models.ModelFileEntries{files[1], files[0]}
	tests := []struct {
		name    string
		members []models.InferenceTask
		wantErr bool
	}{
		{
			name: "identical sets in different order",
			members: []models.InferenceTask{
				{Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: files},
				{Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: reordered},
				{Status: models.InferenceTaskErrorReported, SelectedExecutionModelFiles: files},
			},
		},
		{
			name: "aborted member that never started is ignored",
			members: []models.InferenceTask{
				{Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: files},
				{Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: files},
				{Status: models.InferenceTaskEndAborted},
			},
		},
		{
			name: "different revision",
			members: []models.InferenceTask{
				{Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: files},
				{Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: files},
				{Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: models.ModelFileEntries{
					{SourceType: "huggingface", Repository: "org/model", Revision: "ffffffffffffffffffffffffffffffffffffffff", Format: "diffusers", FormatKwargs: map[string]any{"variant": "fp16"}},
					files[1],
				}},
			},
			wantErr: true,
		},
		{
			name: "ready member without recorded files",
			members: []models.InferenceTask{
				{Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: files},
				{Status: models.InferenceTaskScoreReady, SelectedExecutionModelFiles: files},
				{Status: models.InferenceTaskScoreReady},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := models.CheckGroupExecutionModelFiles(tt.members)
			if tt.wantErr != (err != nil) {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, models.ErrInconsistentExecutionModelFiles) {
				t.Fatalf("error = %v, want ErrInconsistentExecutionModelFiles", err)
			}
		})
	}
}
