package inference_tasks

import (
	"context"
	"crynux_bridge/api/v1/tools"
	"errors"
	"os"
	"testing"
	"time"

	"crynux_bridge/api/v1/response"
	"crynux_bridge/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSelectRawInferenceTaskPrefersDownloaded(t *testing.T) {
	tasks := []models.InferenceTask{
		{Status: models.InferenceTaskEndAborted},
		{Status: models.InferenceTaskResultDownloaded, TaskID: "done"},
	}
	selected := selectRawInferenceTask(tasks)
	if selected.Status != models.InferenceTaskResultDownloaded {
		t.Fatalf("status=%d", selected.Status)
	}
}

func TestLoadRawClientTaskAllowsExhaustedOwnerKeyAndRejectsOtherClient(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Client{},
		&models.ClientAPIKey{},
		&models.ClientTask{},
		&models.InferenceTask{},
	); err != nil {
		t.Fatal(err)
	}
	owner := models.Client{ClientId: "owner"}
	other := models.Client{ClientId: "other"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	keyText, _, err := tools.GenerateAPIKey(ctx, db, owner.ClientId)
	if err != nil {
		t.Fatal(err)
	}
	apiKey, err := models.GetAPIKeyByClientID(ctx, db, owner.ClientId)
	if err != nil {
		t.Fatal(err)
	}
	if err := tools.AddAPIKeyRole(ctx, db, apiKey, models.RoleChat); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(apiKey).Update("used_count", apiKey.UseLimit).Error; err != nil {
		t.Fatal(err)
	}

	ownedClientTask := models.ClientTask{ClientID: owner.ID}
	otherClientTask := models.ClientTask{ClientID: other.ID}
	if err := db.Create(&ownedClientTask).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherClientTask).Error; err != nil {
		t.Fatal(err)
	}
	for _, task := range []models.InferenceTask{
		{ClientID: owner.ID, ClientTaskID: ownedClientTask.ID},
		{ClientID: other.ID, ClientTaskID: otherClientTask.ID},
	} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}

	authorization := "Bearer " + keyText
	if _, err := loadRawClientTask(ctx, db, authorization, ownedClientTask.ID); err != nil {
		t.Fatalf("owner read failed: %v", err)
	}
	_, err = loadRawClientTask(ctx, db, authorization, otherClientTask.ID)
	var validationErr *response.ValidationErrorResponse
	if !errors.As(err, &validationErr) {
		t.Fatalf("other-client error = %T, want validation error", err)
	}
}

func TestSelectRawInferenceTaskUsesUpdatedAtThenID(t *testing.T) {
	now := time.Now()
	tasks := []models.InferenceTask{
		{
			RootModel: models.RootModel{ID: 3, UpdatedAt: now.Add(time.Second)},
			Status:    models.InferenceTaskResultDownloaded,
		},
		{
			RootModel: models.RootModel{ID: 2, UpdatedAt: now},
			Status:    models.InferenceTaskResultDownloaded,
		},
		{
			RootModel: models.RootModel{ID: 1, UpdatedAt: now},
			Status:    models.InferenceTaskResultDownloaded,
		},
	}
	selected := selectRawInferenceTask(tasks)
	if selected == nil || selected.ID != 1 {
		t.Fatalf("selected = %#v, want task 1", selected)
	}
}

func TestSelectSDFTRawInferenceTaskKeepsOriginalBehavior(t *testing.T) {
	now := time.Now()
	tasks := []models.InferenceTask{
		{
			RootModel: models.RootModel{ID: 1, UpdatedAt: now},
			Status:    models.InferenceTaskStarted,
		},
		{
			RootModel: models.RootModel{ID: 2, UpdatedAt: now.Add(time.Second)},
			Status:    models.InferenceTaskEndInvalidated,
		},
	}
	selected := selectSDFTRawInferenceTask(tasks)
	if selected == nil || selected.ID != 1 {
		t.Fatalf("selected = %#v, want task 1", selected)
	}
}

func TestClassifyResultFileError(t *testing.T) {
	missing := classifyResultFileError(models.TaskTypeLLM, os.ErrNotExist)
	var validationErr *response.ValidationErrorResponse
	if !errors.As(missing, &validationErr) {
		t.Fatalf("missing error = %T", missing)
	}
	if validationErr.GetFieldName() != "client_task_id" {
		t.Fatalf("LLM missing field = %q", validationErr.GetFieldName())
	}

	permission := classifyResultFileError(models.TaskTypeSD, os.ErrPermission)
	var exceptionErr *response.ExceptionResponse
	if !errors.As(permission, &exceptionErr) {
		t.Fatalf("permission error = %T, want exception", permission)
	}
}

func TestSelectRawInferenceTaskUsesSnapshotRules(t *testing.T) {
	tests := []struct {
		name string
		in   []models.TaskStatus
		want models.TaskStatus
	}{
		{
			name: "unfinished after group refund",
			in:   []models.TaskStatus{models.InferenceTaskEndGroupRefund, models.InferenceTaskStarted},
			want: models.InferenceTaskStarted,
		},
		{
			name: "actual failure after group refund",
			in:   []models.TaskStatus{models.InferenceTaskEndGroupRefund, models.InferenceTaskEndInvalidated},
			want: models.InferenceTaskEndInvalidated,
		},
		{
			name: "downloaded after group refund",
			in:   []models.TaskStatus{models.InferenceTaskEndGroupRefund, models.InferenceTaskResultDownloaded},
			want: models.InferenceTaskResultDownloaded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks := make([]models.InferenceTask, len(tt.in))
			for i, status := range tt.in {
				tasks[i].Status = status
			}
			selected := selectRawInferenceTask(tasks)
			if selected == nil || selected.Status != tt.want {
				t.Fatalf("selected = %#v, want status %d", selected, tt.want)
			}
		})
	}
}
