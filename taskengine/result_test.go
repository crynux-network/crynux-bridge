package taskengine

import (
	"crynux_bridge/models"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyResultDirectoryVideo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0.mp4"), []byte("mp4-bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	task := &models.InferenceTask{TaskType: models.TaskTypeVideo, TaskSize: 1}
	if err := verifyResultDirectory(dir, task); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyResultDirectoryVideoRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0.mp4"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	task := &models.InferenceTask{TaskType: models.TaskTypeVideo, TaskSize: 1}
	if err := verifyResultDirectory(dir, task); err == nil {
		t.Fatal("expected empty video result to fail")
	}
}

func TestVerifyResultDirectoryVideoRejectsWrongSize(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0.mp4"), []byte("mp4"), 0600); err != nil {
		t.Fatal(err)
	}
	task := &models.InferenceTask{TaskType: models.TaskTypeVideo, TaskSize: 2}
	if err := verifyResultDirectory(dir, task); err == nil {
		t.Fatal("expected task size mismatch to fail")
	}
}
