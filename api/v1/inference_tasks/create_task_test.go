package inference_tasks

import (
	"context"
	"crynux_bridge/api/v1/response"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"errors"
	"sync"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveTaskTimeoutNormalTasks(t *testing.T) {
	appConfig := &config.AppConfig{}
	for _, taskType := range []models.ChainTaskType{models.TaskTypeSD, models.TaskTypeLLM} {
		timeout, err := resolveTaskTimeout(taskType, nil, appConfig)
		if err != nil {
			t.Fatalf("task type %d: %v", taskType, err)
		}
		if timeout != 0 {
			t.Fatalf("task type %d timeout = %d, want 0", taskType, timeout)
		}
		value := uint64(10)
		if _, err := resolveTaskTimeout(taskType, &value, appConfig); err == nil {
			t.Fatalf("task type %d accepted a direct timeout", taskType)
		}
	}
}

func TestResolveTaskTimeoutSDFT(t *testing.T) {
	appConfig := &config.AppConfig{}
	appConfig.Task.SDFinetuneTimeout = 120

	timeout, err := resolveTaskTimeout(models.TaskTypeSDFTLora, nil, appConfig)
	if err != nil {
		t.Fatal(err)
	}
	if timeout != 120*60 {
		t.Fatalf("default SDFT timeout = %d, want %d", timeout, 120*60)
	}

	requested := uint64(321)
	timeout, err = resolveTaskTimeout(models.TaskTypeSDFTLora, &requested, appConfig)
	if err != nil {
		t.Fatal(err)
	}
	if timeout != requested {
		t.Fatalf("requested SDFT timeout = %d, want %d", timeout, requested)
	}

	zero := uint64(0)
	if _, err := resolveTaskTimeout(models.TaskTypeSDFTLora, &zero, appConfig); err == nil {
		t.Fatal("explicit zero SDFT timeout was accepted")
	}

	appConfig.Task.SDFinetuneTimeout = 0
	if _, err := resolveTaskTimeout(models.TaskTypeSDFTLora, nil, appConfig); err == nil {
		t.Fatal("zero default SDFT timeout was accepted")
	}
}

func TestValidateRawTaskInputRequiresTaskFee(t *testing.T) {
	err := validateRawTaskInput(&TaskInput{})
	var validationErr *response.ValidationErrorResponse
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want validation error", err)
	}
	if validationErr.GetFieldName() != "task_fee" {
		t.Fatalf("field = %q, want task_fee", validationErr.GetFieldName())
	}
}

func TestValidateTaskInputRejectsUnsupportedTaskType(t *testing.T) {
	taskType := models.ChainTaskType(3)
	err := validateTaskInput(&TaskInput{TaskType: &taskType}, &config.AppConfig{})
	var validationErr *response.ValidationErrorResponse
	if !errors.As(err, &validationErr) || validationErr.GetFieldName() != "task_type" {
		t.Fatalf("error = %#v, want task_type validation error", err)
	}
}

func TestValidateTaskInputRejectsNonPositiveRepeatNum(t *testing.T) {
	taskType := models.TaskTypeLLM
	for _, repeatNum := range []int{0, -1} {
		err := validateTaskInput(&TaskInput{TaskType: &taskType, RepeatNum: &repeatNum}, &config.AppConfig{})
		var validationErr *response.ValidationErrorResponse
		if !errors.As(err, &validationErr) || validationErr.GetFieldName() != "repeat_num" {
			t.Fatalf("repeat_num %d error = %#v", repeatNum, err)
		}
	}
}

func TestResolveTaskFeeRejectsInvalidWei(t *testing.T) {
	for _, value := range []string{"-1", "1.0", "01", "1e3"} {
		if _, err := resolveTaskFee(&value, models.TaskTypeLLM, "", 1, &config.AppConfig{}); err == nil {
			t.Fatalf("accepted invalid Wei %q", value)
		}
	}
}

func TestResolveTaskFeeUsesRequestedWeiAsFinalFee(t *testing.T) {
	requested := "1234567890"
	got, err := resolveTaskFee(&requested, models.TaskTypeSD, "", 8, &config.AppConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if got != requested {
		t.Fatalf("task fee = %s, want %s", got, requested)
	}
}

func TestResolveTaskFeeKeepsInternalTaskSizeCalculation(t *testing.T) {
	appConfig := &config.AppConfig{}
	appConfig.Task.DefaultLLMTaskFeeCNX = "0.000000001"
	got, err := resolveTaskFee(nil, models.TaskTypeLLM, "", 2, appConfig)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2000000000" {
		t.Fatalf("task fee = %s, want 2000000000 Wei", got)
	}
}

func TestResolveTaskFeeSupportsTwentyCNXImageFee(t *testing.T) {
	appConfig := &config.AppConfig{}
	appConfig.Task.DefaultSDTaskFeeCNX = "20"
	got, err := resolveTaskFee(
		nil,
		models.TaskTypeSD,
		`{"base_model":{"name":"model"}}`,
		2,
		appConfig,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "40000000000000000000" {
		t.Fatalf("task fee = %s", got)
	}
}

func TestSaveTaskRecordsCreatesNewTaskForEverySubmission(t *testing.T) {
	db := setupTaskCreationDB(t, true)
	client := models.Client{ClientId: "client"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	build := func(clientTask *models.ClientTask) ([]*models.InferenceTask, error) {
		return []*models.InferenceTask{{
			ClientID:     client.ID,
			ClientTaskID: clientTask.ID,
			TaskID:       "task",
		}}, nil
	}

	first, err := saveTaskRecords(context.Background(), db, &client, build)
	if err != nil {
		t.Fatal(err)
	}
	second, err := saveTaskRecords(context.Background(), db, &client, build)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("duplicate submissions reused client task %d", first.ID)
	}

	var count int64
	if err := db.Model(&models.ClientTask{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("client task count = %d, want 2", count)
	}
}

func TestUseWithinLimitIsConditional(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ClientAPIKey{}); err != nil {
		t.Fatal(err)
	}
	apiKey := models.ClientAPIKey{ClientID: "client", UseLimit: 1}
	if err := db.Create(&apiKey).Error; err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := apiKey.UseWithinLimit(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := apiKey.UseWithinLimit(ctx, db); !errors.Is(err, models.ErrAPIKeyQuotaExceeded) {
		t.Fatalf("quota error = %v, want ErrAPIKeyQuotaExceeded", err)
	}
	if err := db.First(&apiKey, apiKey.ID).Error; err != nil {
		t.Fatal(err)
	}
	if apiKey.UsedCount != 1 {
		t.Fatalf("used count after success = %d, want 1", apiKey.UsedCount)
	}
}

func TestCreateRawTaskMapsQuotaExhaustionToValidationError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ClientAPIKey{}); err != nil {
		t.Fatal(err)
	}
	apiKey := models.ClientAPIKey{ClientID: "exhausted", UseLimit: 1, UsedCount: 1}
	if err := db.Create(&apiKey).Error; err != nil {
		t.Fatal(err)
	}

	_, err = createRawTaskAndConsumeQuota(context.Background(), db, &apiKey, &TaskInput{})
	var validationErr *response.ValidationErrorResponse
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want validation error", err)
	}
	if validationErr.GetFieldName() != "Authorization" {
		t.Fatalf("field = %q, want Authorization", validationErr.GetFieldName())
	}
}

func TestCreateRawTaskMapsDatabaseFailureToException(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	apiKey := models.ClientAPIKey{RootModel: models.RootModel{ID: 1}}
	_, err = createRawTaskAndConsumeQuota(context.Background(), db, &apiKey, &TaskInput{})
	var exceptionErr *response.ExceptionResponse
	if !errors.As(err, &exceptionErr) {
		t.Fatalf("error = %T, want exception error", err)
	}
}

func TestCreateRawTaskRollsBackQuotaWhenRecordCreationFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ClientAPIKey{}); err != nil {
		t.Fatal(err)
	}
	apiKey := models.ClientAPIKey{ClientID: "rollback", UseLimit: 1}
	if err := db.Create(&apiKey).Error; err != nil {
		t.Fatal(err)
	}

	_, err = createRawTaskAndConsumeQuota(context.Background(), db, &apiKey, &TaskInput{})
	var exceptionErr *response.ExceptionResponse
	if !errors.As(err, &exceptionErr) {
		t.Fatalf("error = %T, want exception error", err)
	}
	if err := db.First(&apiKey, apiKey.ID).Error; err != nil {
		t.Fatal(err)
	}
	if apiKey.UsedCount != 0 {
		t.Fatalf("used_count = %d, want 0", apiKey.UsedCount)
	}
}

func TestUseWithinLimitConcurrentBoundary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:quota?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.ClientAPIKey{}); err != nil {
		t.Fatal(err)
	}
	apiKey := models.ClientAPIKey{ClientID: "concurrent", UseLimit: 3}
	if err := db.Create(&apiKey).Error; err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- apiKey.UseWithinLimit(context.Background(), db)
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 3 {
		t.Fatalf("successful uses = %d, want 3", successes)
	}
}

func setupTaskCreationDB(t *testing.T, migrateInferenceTask bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Client{}, &models.ClientTask{}); err != nil {
		t.Fatal(err)
	}
	if migrateInferenceTask {
		if err := db.AutoMigrate(&models.InferenceTask{}); err != nil {
			t.Fatal(err)
		}
	}
	return db
}
