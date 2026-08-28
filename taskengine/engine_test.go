package taskengine

import (
	"context"
	"crynux_bridge/models"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestEngine(t *testing.T) (*Engine, *gorm.DB) {
	t.Helper()
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
	engine, err := New(db, testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	return engine, db
}

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		DefaultRepeatNum:             2,
		ScanInterval:                 time.Millisecond,
		StatusPollInterval:           time.Millisecond,
		ExecutionPollAdvance:         0,
		ExecutionOverrunPollInterval: time.Millisecond,
		RetryInterval:                time.Millisecond,
		OperationTimeout:             time.Second,
		ExpansionBatchSize:           10,
		OperationResultBatchSize:     10,
		DueTaskBatchSize:             10,
		CreateBatchSize:              10,
		StatusBatchSize:              10,
		ValidationBatchSize:          10,
		CancellationBatchSize:        10,
		CreateWorkers:                1,
		StatusWorkers:                1,
		ValidationWorkers:            1,
		CancellationWorkers:          1,
		ResultWorkers:                1,
		ResultDirectory:              t.TempDir(),
		PrivateKey:                   "1",
	}
}

func testSubmission() Submission {
	return Submission{
		TaskArgs:     `{"model":"test"}`,
		TaskType:     models.TaskTypeLLM,
		TaskModelIDs: []string{"base:test"},
		TaskVersion:  "1",
		TaskFee:      "1",
		MinVram:      1,
		TaskSize:     1,
	}
}

func TestCreateTaskPersistsOnlyLogicalSubmission(t *testing.T) {
	engine, db := newTestEngine(t)
	apiKey := models.ClientAPIKey{ClientID: "client", UseLimit: 1}
	if err := db.Create(&apiKey).Error; err != nil {
		t.Fatal(err)
	}
	clientTask, err := engine.CreateTask(context.Background(), "client", &apiKey, testSubmission())
	if err != nil {
		t.Fatal(err)
	}
	if clientTask.RepeatExpanded || clientTask.EffectiveRepeatNum != 2 || clientTask.Submission == "" {
		t.Fatalf("unexpected client task: %#v", clientTask)
	}
	var inferenceCount int64
	if err := db.Model(&models.InferenceTask{}).Count(&inferenceCount).Error; err != nil {
		t.Fatal(err)
	}
	if inferenceCount != 0 {
		t.Fatalf("inference task count = %d, want 0", inferenceCount)
	}
	if err := db.First(&apiKey, apiKey.ID).Error; err != nil {
		t.Fatal(err)
	}
	if apiKey.UsedCount != 1 {
		t.Fatalf("used count = %d, want 1", apiKey.UsedCount)
	}
}

func TestExpandClientTaskCreatesAllRepeatsAtomically(t *testing.T) {
	engine, db := newTestEngine(t)
	repeat := 3
	submission := testSubmission()
	submission.RepeatNum = &repeat
	clientTask, err := engine.CreateTask(context.Background(), "client", nil, submission)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.expandDueClientTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	var updated models.ClientTask
	if err := db.First(&updated, clientTask.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !updated.RepeatExpanded || updated.Submission != "" {
		t.Fatalf("expanded client task = %#v", updated)
	}
	var tasks []models.InferenceTask
	if err := db.Where("client_task_id = ?", clientTask.ID).Find(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	if len(tasks) != repeat {
		t.Fatalf("repeat count = %d, want %d", len(tasks), repeat)
	}
	for i := range tasks {
		if tasks[i].TaskID == "" || tasks[i].TaskIDCommitment == "" || tasks[i].Nonce == "" {
			t.Fatalf("task %d has incomplete identity: %#v", i, tasks[i])
		}
	}
}

func TestRecoverClearsRunningOperationAndRepairsClientTask(t *testing.T) {
	engine, db := newTestEngine(t)
	client := models.Client{ClientId: "client"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	clientTask := models.ClientTask{
		ClientID:           client.ID,
		EffectiveRepeatNum: 1,
		RepeatExpanded:     true,
	}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatal(err)
	}
	task := models.InferenceTask{
		ClientID:        client.ID,
		ClientTaskID:    clientTask.ID,
		TaskType:        models.TaskTypeLLM,
		TaskID:          "task",
		Status:          models.InferenceTaskResultDownloaded,
		OperationType:   models.TaskOperationCreate,
		OperationStatus: models.TaskOperationStatusRunning,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := engine.recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if task.OperationStatus != models.TaskOperationStatusNone || !task.OperationUnknown {
		t.Fatalf("recovered operation = %q, unknown=%v", task.OperationStatus, task.OperationUnknown)
	}
	if err := db.First(&clientTask, clientTask.ID).Error; err != nil {
		t.Fatal(err)
	}
	if clientTask.Status != models.ClientTaskStatusSuccess {
		t.Fatalf("client status = %s, want success", clientTask.Status)
	}
}

func TestMarkRunningAllowsOnlyOneOperationPerTask(t *testing.T) {
	engine, db := newTestEngine(t)
	task := models.InferenceTask{
		TaskType:     models.TaskTypeLLM,
		TaskID:       "task",
		NextActionAt: time.Now(),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	decision := operationDecision{task: task, operation: models.TaskOperationSyncStatus}
	if err := engine.markRunning(context.Background(), []operationDecision{decision}); err != nil {
		t.Fatal(err)
	}
	if err := engine.markRunning(context.Background(), []operationDecision{decision}); err == nil {
		t.Fatal("second operation was accepted while the first operation was running")
	}
	var updated models.InferenceTask
	if err := db.First(&updated, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.OperationType != models.TaskOperationSyncStatus ||
		updated.OperationStatus != models.TaskOperationStatusRunning {
		t.Fatalf("operation = %q/%q", updated.OperationType, updated.OperationStatus)
	}
}

func TestValidationOwnerSkipsAbortedFirstMember(t *testing.T) {
	engine, db := newTestEngine(t)
	group := []models.InferenceTask{
		{TaskType: models.TaskTypeLLM, TaskID: "group", Sequence: 1, Status: models.InferenceTaskEndAborted, RelayChecked: true},
		{TaskType: models.TaskTypeLLM, TaskID: "group", Sequence: 2, Status: models.InferenceTaskScoreReady, RelayChecked: true},
		{TaskType: models.TaskTypeLLM, TaskID: "group", Sequence: 3, Status: models.InferenceTaskErrorReported, RelayChecked: true},
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	ownerDecision, err := engine.decideOperation(context.Background(), group[1])
	if err != nil {
		t.Fatal(err)
	}
	if ownerDecision.operation != models.TaskOperationValidate || ownerDecision.task.ID != group[1].ID {
		t.Fatalf("owner decision = %#v", ownerDecision)
	}
	otherDecision, err := engine.decideOperation(context.Background(), group[2])
	if err != nil {
		t.Fatal(err)
	}
	if otherDecision.operation != models.TaskOperationSyncStatus {
		t.Fatalf("other member operation = %q, want status", otherDecision.operation)
	}
}

func TestCountPendingIncludesUnexpandedSubmissionWithoutDoubleCounting(t *testing.T) {
	engine, db := newTestEngine(t)
	repeat := 1
	submission := testSubmission()
	submission.RepeatNum = &repeat
	clientTask, err := engine.CreateTask(context.Background(), "heartbeat-task", nil, submission)
	if err != nil {
		t.Fatal(err)
	}
	count, err := engine.CountPending(context.Background(), "heartbeat-task", models.TaskTypeLLM, "base:test")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("unexpanded count = %d, want 1", count)
	}
	if err := engine.expandClientTask(context.Background(), *clientTask); err != nil {
		t.Fatal(err)
	}
	count, err = engine.CountPending(context.Background(), "heartbeat-task", models.TaskTypeLLM, "base:test")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expanded count = %d, want 1", count)
	}
	if err := db.Model(&models.InferenceTask{}).
		Where("client_task_id = ?", clientTask.ID).
		Update("status", models.InferenceTaskEndAborted).Error; err != nil {
		t.Fatal(err)
	}
	count, err = engine.CountPending(context.Background(), "heartbeat-task", models.TaskTypeLLM, "base:test")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("terminal count = %d, want 0", count)
	}
}

func TestRecalculateClientTaskCountsOnlyCompletedFailedGroups(t *testing.T) {
	engine, db := newTestEngine(t)
	client := models.Client{ClientId: "client"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	clientTask := models.ClientTask{
		ClientID:       client.ID,
		Status:         models.ClientTaskStatusRunning,
		RepeatExpanded: true,
	}
	if err := db.Create(&clientTask).Error; err != nil {
		t.Fatal(err)
	}
	tasks := []models.InferenceTask{
		{ClientID: client.ID, ClientTaskID: clientTask.ID, TaskType: models.TaskTypeLLM, TaskID: "vss", Status: models.InferenceTaskEndAborted},
		{ClientID: client.ID, ClientTaskID: clientTask.ID, TaskType: models.TaskTypeLLM, TaskID: "vss", Status: models.InferenceTaskStarted},
		{ClientID: client.ID, ClientTaskID: clientTask.ID, TaskType: models.TaskTypeLLM, TaskID: "vss", Status: models.InferenceTaskStarted},
		{ClientID: client.ID, ClientTaskID: clientTask.ID, TaskType: models.TaskTypeLLM, TaskID: "failed", Status: models.InferenceTaskEndAborted},
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return engine.recalculateClientTask(context.Background(), tx, clientTask.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&clientTask, clientTask.ID).Error; err != nil {
		t.Fatal(err)
	}
	if clientTask.FailedCount != 1 {
		t.Fatalf("failed count = %d, want 1", clientTask.FailedCount)
	}
	if clientTask.Status != models.ClientTaskStatusRunning {
		t.Fatalf("client status = %s, want running", clientTask.Status)
	}
}

func TestCountSubmittedSinceUsesPersistedClientTasks(t *testing.T) {
	engine, _ := newTestEngine(t)
	since := time.Now().Add(-time.Minute)
	for range 2 {
		if _, err := engine.CreateTask(context.Background(), "heartbeat-task", nil, testSubmission()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := engine.CreateTask(context.Background(), "other-client", nil, testSubmission()); err != nil {
		t.Fatal(err)
	}
	count, err := engine.CountSubmittedSince(context.Background(), "heartbeat-task", since)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("submitted count = %d, want 2", count)
	}
}

func TestNewRejectsIncompleteConfig(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(db, Config{}); err == nil {
		t.Fatal("incomplete config was accepted")
	}
}

func TestWorkerPoolsHaveIndependentCapacity(t *testing.T) {
	engine, _ := newTestEngine(t)
	engine.createCapacity <- struct{}{}
	if len(engine.createCapacity) != cap(engine.createCapacity) {
		t.Fatal("create pool is not full")
	}
	for name, capacity := range map[string]chan struct{}{
		"status":       engine.statusCapacity,
		"validation":   engine.validationCapacity,
		"cancellation": engine.cancellationCapacity,
		"result":       engine.resultCapacity,
	} {
		if len(capacity) != 0 || cap(capacity) == 0 {
			t.Fatalf("%s pool capacity = %d/%d", name, len(capacity), cap(capacity))
		}
	}
}

func TestDueSelectionDoesNotLetCreatesHideStatusTasks(t *testing.T) {
	engine, db := newTestEngine(t)
	now := time.Now().Add(-time.Second)
	createTasks := make([]models.InferenceTask, engine.config.DueTaskBatchSize+1)
	for i := range createTasks {
		createTasks[i] = models.InferenceTask{
			TaskType:     models.TaskTypeLLM,
			TaskID:       "create",
			Status:       models.InferenceTaskPending,
			RelayChecked: true,
			RelayFound:   false,
			NextActionAt: now,
		}
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&createTasks).Error; err != nil {
		t.Fatal(err)
	}
	statusTask := models.InferenceTask{
		TaskType:     models.TaskTypeLLM,
		TaskID:       "status",
		Status:       models.InferenceTaskStarted,
		RelayChecked: true,
		RelayFound:   true,
		NextActionAt: now,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&statusTask).Error; err != nil {
		t.Fatal(err)
	}
	tasks, err := engine.loadDueTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foundStatus := false
	for i := range tasks {
		if tasks[i].ID == statusTask.ID {
			foundStatus = true
			break
		}
	}
	if !foundStatus {
		t.Fatal("status task was hidden by due create tasks")
	}
}

func TestOperationWriteRequiresRunningRowAndParentCanPersistTimeout(t *testing.T) {
	engine, db := newTestEngine(t)
	task := models.InferenceTask{TaskType: models.TaskTypeLLM, TaskID: "task"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := engine.retryOperationWrite(context.Background(), task.ID, map[string]any{
		"operation_status": models.TaskOperationStatusCompleted,
	}); !errors.Is(err, errOperationNotRunning) {
		t.Fatalf("non-running update error = %v", err)
	}
	if err := db.Model(&task).Updates(map[string]any{
		"operation_type":   models.TaskOperationCreate,
		"operation_status": models.TaskOperationStatusRunning,
	}).Error; err != nil {
		t.Fatal(err)
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := engine.retryOperationWrite(workerCtx, task.ID, map[string]any{
		"operation_status": models.TaskOperationStatusCompleted,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled worker update error = %v", err)
	}
	engine.writeBatchFailure(context.Background(), []operationDecision{{task: task}}, context.DeadlineExceeded, true)
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if task.OperationStatus != models.TaskOperationStatusFailed || !task.OperationUnknown {
		t.Fatalf("timeout state = %q, unknown=%v", task.OperationStatus, task.OperationUnknown)
	}
}
