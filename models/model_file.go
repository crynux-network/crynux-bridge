package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ModelFileEntry is one exact model file requirement exchanged with Relay.
type ModelFileEntry struct {
	SourceType   string         `json:"source_type"`
	Repository   string         `json:"repository"`
	Revision     string         `json:"revision"`
	Format       string         `json:"format"`
	FormatKwargs map[string]any `json:"format_kwargs"`
}

// ModelFileEntries is a set of model file entries persisted as JSON text.
type ModelFileEntries []ModelFileEntry

func (entries *ModelFileEntries) Scan(val interface{}) error {
	var raw string
	switch v := val.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	case nil:
		*entries = nil
		return nil
	default:
		return fmt.Errorf("unable to parse value to ModelFileEntries: %T", val)
	}
	if raw == "" {
		*entries = nil
		return nil
	}
	return json.Unmarshal([]byte(raw), entries)
}

func (entries ModelFileEntries) Value() (driver.Value, error) {
	if len(entries) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal([]ModelFileEntry(entries))
	if err != nil {
		return nil, err
	}
	return string(encoded), nil
}

// CanonicalKey identifies the set of entries independent of their order.
func (entries ModelFileEntries) CanonicalKey() (string, error) {
	encoded := make([]string, len(entries))
	for i := range entries {
		raw, err := json.Marshal(entries[i])
		if err != nil {
			return "", err
		}
		encoded[i] = string(raw)
	}
	slices.Sort(encoded)
	return strings.Join(encoded, "\n"), nil
}

// ErrInconsistentExecutionModelFiles reports that the members of one validation
// group did not execute with the same model files.
var ErrInconsistentExecutionModelFiles = errors.New("validation group members used different execution model files")

// CheckGroupExecutionModelFiles verifies that every member that started executing
// recorded the same execution model file set. A member that never started has no
// recorded entries and is ignored; a member that did not abort must have recorded them.
func CheckGroupExecutionModelFiles(tasks []InferenceTask) error {
	var groupKey string
	var keyed bool
	for i := range tasks {
		entries := tasks[i].SelectedExecutionModelFiles
		if len(entries) == 0 {
			if tasks[i].Status == InferenceTaskEndAborted {
				continue
			}
			return fmt.Errorf("%w: task %s has no execution model files", ErrInconsistentExecutionModelFiles, tasks[i].TaskIDCommitment)
		}
		key, err := entries.CanonicalKey()
		if err != nil {
			return err
		}
		if !keyed {
			groupKey, keyed = key, true
			continue
		}
		if key != groupKey {
			return fmt.Errorf("%w: task %s", ErrInconsistentExecutionModelFiles, tasks[i].TaskIDCommitment)
		}
	}
	return nil
}
