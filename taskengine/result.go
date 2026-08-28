package taskengine

import (
	"archive/zip"
	"context"
	"crynux_bridge/models"
	"crynux_bridge/relay"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func (e *Engine) runResultWorker(parent context.Context, decision operationDecision) {
	defer func() { <-e.resultCapacity }()
	ctx, cancel := context.WithTimeout(parent, e.config.OperationTimeout)
	defer cancel()
	completed := false
	defer func() {
		if recovered := recover(); recovered != nil {
			e.writeBatchFailure(parent, []operationDecision{decision}, fmt.Errorf("result worker panic: %v", recovered), false)
		} else if !completed {
			e.writeBatchFailure(parent, []operationDecision{decision}, ctx.Err(), false)
		}
	}()
	if err := e.downloadResult(ctx, &decision.task); err != nil {
		e.writeBatchFailure(parent, []operationDecision{decision}, err, false)
		completed = true
		return
	}
	if err := e.writeOperationCompleted(ctx, decision.task.ID, map[string]any{"downloaded": true}); err != nil {
		e.writeBatchFailure(parent, []operationDecision{decision}, err, false)
		completed = true
		return
	}
	completed = true
}

func (e *Engine) downloadResult(ctx context.Context, task *models.InferenceTask) error {
	finalDirectory := filepath.Join(e.config.ResultDirectory, task.TaskIDCommitment)
	if _, err := os.Stat(finalDirectory); err == nil {
		if err := verifyResultDirectory(finalDirectory, task); err == nil {
			return nil
		}
		if err := os.RemoveAll(finalDirectory); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(e.config.ResultDirectory, 0700); err != nil {
		return err
	}
	tempDirectory, err := os.MkdirTemp(e.config.ResultDirectory, ".task-result-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDirectory)

	if task.TaskType == models.TaskTypeLLM {
		if task.TaskSize != 1 {
			return fmt.Errorf("LLM task size is %d, want 1", task.TaskSize)
		}
		tempFile := filepath.Join(tempDirectory, "0.json")
		if err := relay.DownloadWholeTaskResult(ctx, task.TaskIDCommitment, tempFile); err != nil {
			return err
		}
		content, err := os.ReadFile(tempFile)
		if err != nil {
			return err
		}
		if !json.Valid(content) {
			return fmt.Errorf("LLM result is not valid JSON")
		}
	} else if task.TaskType == models.TaskTypeSD {
		archiveFile := filepath.Join(tempDirectory, "result.zip")
		if err := relay.DownloadWholeTaskResult(ctx, task.TaskIDCommitment, archiveFile); err != nil {
			return err
		}
		if err := extractSDResultArchive(archiveFile, tempDirectory, task.TaskSize); err != nil {
			return err
		}
		if err := os.Remove(archiveFile); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("unsupported result task type %d", task.TaskType)
	}
	return os.Rename(tempDirectory, finalDirectory)
}

func verifyResultDirectory(directory string, task *models.InferenceTask) error {
	if task.TaskType == models.TaskTypeLLM {
		content, err := os.ReadFile(filepath.Join(directory, "0.json"))
		if err != nil {
			return err
		}
		if !json.Valid(content) {
			return fmt.Errorf("LLM result is not valid JSON")
		}
		return nil
	}
	if task.TaskType != models.TaskTypeSD {
		return fmt.Errorf("unsupported result task type %d", task.TaskType)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	if uint64(len(entries)) != task.TaskSize {
		return fmt.Errorf("result directory has %d entries, want %d", len(entries), task.TaskSize)
	}
	for index := uint64(0); index < task.TaskSize; index++ {
		info, err := os.Stat(filepath.Join(directory, fmt.Sprintf("%d.png", index)))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("result %d.png is not a regular file", index)
		}
	}
	return nil
}

func extractSDResultArchive(archiveFile, destination string, taskSize uint64) error {
	reader, err := zip.OpenReader(archiveFile)
	if err != nil {
		return err
	}
	defer reader.Close()
	if uint64(len(reader.File)) != taskSize {
		return fmt.Errorf("result archive has %d entries, want %d", len(reader.File), taskSize)
	}
	seen := make(map[string]struct{}, len(reader.File))
	for _, entry := range reader.File {
		expected := false
		for index := uint64(0); index < taskSize; index++ {
			if entry.Name == fmt.Sprintf("%d.png", index) {
				expected = true
				break
			}
		}
		if !expected || filepath.Base(entry.Name) != entry.Name {
			return fmt.Errorf("unexpected result archive entry %q", entry.Name)
		}
		if _, exists := seen[entry.Name]; exists {
			return fmt.Errorf("duplicate result archive entry %q", entry.Name)
		}
		seen[entry.Name] = struct{}{}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		target, err := os.OpenFile(filepath.Join(destination, entry.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(target, source)
		closeErr := target.Close()
		sourceErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if sourceErr != nil {
			return sourceErr
		}
	}
	for index := uint64(0); index < taskSize; index++ {
		if _, ok := seen[fmt.Sprintf("%d.png", index)]; !ok {
			return fmt.Errorf("result archive is missing %d.png", index)
		}
	}
	return nil
}
