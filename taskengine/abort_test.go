package taskengine

import (
	"context"
	"errors"
	"testing"

	"crynux_bridge/models"
	"crynux_bridge/relay"

	"gorm.io/gorm"
)

func TestAbortUnexpandedClientTask(t *testing.T) {
	engine, db := newTestEngine(t)
	clientTask, err := engine.CreateTask(context.Background(), "client", nil, testSubmission())
	if err != nil {
		t.Fatal(err)
	}
	aborted, err := engine.AbortClientTask(context.Background(), "client", clientTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if aborted.Status != models.ClientTaskStatusFailed || !aborted.CreatorCancelled {
		t.Fatalf("unexpected aborted task: %#v", aborted)
	}
	if aborted.Phase != ClientTaskPhaseFailed || aborted.ErrorMessage != ClientTaskAbortErrorMessage {
		t.Fatalf("unexpected phase: %#v", aborted)
	}
	again, err := engine.AbortClientTask(context.Background(), "client", clientTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != models.ClientTaskStatusFailed || !again.CreatorCancelled {
		t.Fatalf("idempotent abort failed: %#v", again)
	}
	if err := engine.expandDueClientTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	var inferenceCount int64
	if err := db.Model(&models.InferenceTask{}).Count(&inferenceCount).Error; err != nil {
		t.Fatal(err)
	}
	if inferenceCount != 0 {
		t.Fatalf("expanded after abort: count=%d", inferenceCount)
	}
}

func TestAbortFailsWhenMemberStartedWithoutCancellingSiblings(t *testing.T) {
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
	started := models.InferenceTask{
		ClientID:         client.ID,
		ClientTaskID:     clientTask.ID,
		TaskID:           "task-a",
		TaskIDCommitment: "0x1",
		TaskType:         models.TaskTypeLLM,
		Status:           models.InferenceTaskStarted,
	}
	queued := models.InferenceTask{
		ClientID:         client.ID,
		ClientTaskID:     clientTask.ID,
		TaskID:           "task-a",
		TaskIDCommitment: "0x2",
		TaskType:         models.TaskTypeLLM,
		Status:           models.InferenceTaskCreated,
	}
	otherQueued := models.InferenceTask{
		ClientID:         client.ID,
		ClientTaskID:     clientTask.ID,
		TaskID:           "task-b",
		TaskIDCommitment: "0x3",
		TaskType:         models.TaskTypeLLM,
		Status:           models.InferenceTaskPending,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&started).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&queued).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&otherQueued).Error; err != nil {
		t.Fatal(err)
	}

	calledStatus := false
	batchGetTaskStatus = func(context.Context, []string) ([]relay.BatchStatusItem, error) {
		calledStatus = true
		return nil, errors.New("should not preflight")
	}
	batchAbortTasks = func(context.Context, []string) ([]relay.BatchMutationResult, error) {
		t.Fatal("should not abort")
		return nil, nil
	}
	t.Cleanup(func() {
		batchGetTaskStatus = relay.BatchGetTaskStatus
		batchAbortTasks = relay.BatchAbortTasks
	})

	_, err := engine.AbortClientTask(context.Background(), "client", clientTask.ID)
	if !errors.Is(err, ErrAbortNotCancellable) {
		t.Fatalf("err=%v, want not_cancellable", err)
	}
	if calledStatus {
		t.Fatal("preflight should not run when local group check fails")
	}
	var pending models.InferenceTask
	if err := db.First(&pending, otherQueued.ID).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Status != models.InferenceTaskPending {
		t.Fatalf("pending sibling status=%v, want pending", pending.Status)
	}
	var stillQueued models.InferenceTask
	if err := db.First(&stillQueued, queued.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stillQueued.Status != models.InferenceTaskCreated {
		t.Fatalf("queued VSS member status=%v, want created", stillQueued.Status)
	}
}

func TestAbortQueuedCommitmentsSuccess(t *testing.T) {
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
		{
			ClientID: client.ID, ClientTaskID: clientTask.ID, TaskID: "a",
			TaskIDCommitment: "0xa", TaskType: models.TaskTypeLLM, Status: models.InferenceTaskCreated,
		},
		{
			ClientID: client.ID, ClientTaskID: clientTask.ID, TaskID: "b",
			TaskIDCommitment: "0xb", TaskType: models.TaskTypeLLM, Status: models.InferenceTaskCreated,
		},
	}
	for i := range tasks {
		if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&tasks[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	batchGetTaskStatus = func(_ context.Context, commitments []string) ([]relay.BatchStatusItem, error) {
		items := make([]relay.BatchStatusItem, 0, len(commitments))
		for _, commitment := range commitments {
			items = append(items, relay.BatchStatusItem{
				TaskIDCommitment: commitment,
				Found:            true,
				Status:           models.ChainTaskQueued,
			})
		}
		return items, nil
	}
	batchAbortTasks = func(_ context.Context, commitments []string) ([]relay.BatchMutationResult, error) {
		results := make([]relay.BatchMutationResult, 0, len(commitments))
		for _, commitment := range commitments {
			results = append(results, relay.BatchMutationResult{
				TaskIDCommitment: commitment,
				Outcome:          relay.BatchOutcomeCancelled,
			})
		}
		return results, nil
	}
	t.Cleanup(func() {
		batchGetTaskStatus = relay.BatchGetTaskStatus
		batchAbortTasks = relay.BatchAbortTasks
	})

	aborted, err := engine.AbortClientTask(context.Background(), "client", clientTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if aborted.Status != models.ClientTaskStatusFailed || !aborted.CreatorCancelled {
		t.Fatalf("unexpected client task: %#v", aborted)
	}
	var members []models.InferenceTask
	if err := db.Find(&members).Error; err != nil {
		t.Fatal(err)
	}
	for _, member := range members {
		if member.Status != models.InferenceTaskEndAborted || member.AbortReason != models.TaskAbortCreatorCancelled {
			t.Fatalf("unexpected member: %#v", member)
		}
	}
}

func TestAbortPartialRelayRaceKeepsCancelledMembers(t *testing.T) {
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
	first := models.InferenceTask{
		ClientID: client.ID, ClientTaskID: clientTask.ID, TaskID: "a",
		TaskIDCommitment: "0xa", TaskType: models.TaskTypeLLM, Status: models.InferenceTaskCreated,
	}
	second := models.InferenceTask{
		ClientID: client.ID, ClientTaskID: clientTask.ID, TaskID: "b",
		TaskIDCommitment: "0xb", TaskType: models.TaskTypeLLM, Status: models.InferenceTaskCreated,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&second).Error; err != nil {
		t.Fatal(err)
	}

	batchGetTaskStatus = func(_ context.Context, commitments []string) ([]relay.BatchStatusItem, error) {
		items := make([]relay.BatchStatusItem, 0, len(commitments))
		for _, commitment := range commitments {
			items = append(items, relay.BatchStatusItem{
				TaskIDCommitment: commitment,
				Found:            true,
				Status:           models.ChainTaskQueued,
			})
		}
		return items, nil
	}
	batchAbortTasks = func(_ context.Context, commitments []string) ([]relay.BatchMutationResult, error) {
		return []relay.BatchMutationResult{
			{TaskIDCommitment: commitments[0], Outcome: relay.BatchOutcomeCancelled},
			{TaskIDCommitment: commitments[1], Outcome: relay.BatchOutcomeNotCancellable},
		}, nil
	}
	t.Cleanup(func() {
		batchGetTaskStatus = relay.BatchGetTaskStatus
		batchAbortTasks = relay.BatchAbortTasks
	})

	_, err := engine.AbortClientTask(context.Background(), "client", clientTask.ID)
	if !errors.Is(err, ErrAbortNotCancellable) {
		t.Fatalf("err=%v, want not_cancellable", err)
	}
	var updatedClientTask models.ClientTask
	if err := db.First(&updatedClientTask, clientTask.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedClientTask.Status != models.ClientTaskStatusRunning || updatedClientTask.CreatorCancelled {
		t.Fatalf("client task should stay running: %#v", updatedClientTask)
	}
	var updatedFirst models.InferenceTask
	if err := db.First(&updatedFirst, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedFirst.Status != models.InferenceTaskEndAborted {
		t.Fatalf("first member status=%v, want end aborted", updatedFirst.Status)
	}
	var updatedSecond models.InferenceTask
	if err := db.First(&updatedSecond, second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedSecond.Status != models.InferenceTaskCreated {
		t.Fatalf("second member status=%v, want created", updatedSecond.Status)
	}
}

func TestAbortBatchItemIndependence(t *testing.T) {
	engine, db := newTestEngine(t)
	client := models.Client{ClientId: "client"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	running := models.ClientTask{ClientID: client.ID, Status: models.ClientTaskStatusRunning}
	success := models.ClientTask{ClientID: client.ID, Status: models.ClientTaskStatusSuccess, RepeatExpanded: true}
	if err := db.Create(&running).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&success).Error; err != nil {
		t.Fatal(err)
	}

	results, err := engine.AbortClientTaskBatch(context.Background(), "client", []uint{running.ID, success.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("len=%d", len(results))
	}
	if results[0].Error != "" || results[0].ClientTask == nil || results[0].ClientTask.Status != models.ClientTaskStatusFailed {
		t.Fatalf("first item: %#v", results[0])
	}
	if results[1].Error != "not_cancellable" || results[1].ClientTask != nil {
		t.Fatalf("second item: %#v", results[1])
	}
}

func TestAbortRejectsResultDownloaded(t *testing.T) {
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
	member := models.InferenceTask{
		ClientID: client.ID, ClientTaskID: clientTask.ID, TaskID: "a",
		TaskType: models.TaskTypeLLM, Status: models.InferenceTaskResultDownloaded,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	_, err := engine.AbortClientTask(context.Background(), "client", clientTask.ID)
	if !errors.Is(err, ErrAbortNotCancellable) {
		t.Fatalf("err=%v", err)
	}
}
