package tasktrace

import (
	"crynux_bridge/models"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	SourceOpenAIChatCompletions = "openai_chat_completions"
	SourceOpenAICompletions     = "openai_completions"
	SourceDirectInferenceTask   = "direct_inference_task"
	SourceImageGeneration       = "image_generation"
	SourceImageFinetune         = "image_finetune"
)

type StartTraceInput struct {
	Source      string
	Endpoint    string
	ClientID    string
	Model       string
	TaskType    *models.ChainTaskType
	Request     any
	RequestTime time.Time
	Tasks       []models.InferenceTask
}

type Trace struct {
	Source                  string        `json:"source"`
	Endpoint                string        `json:"endpoint"`
	ClientID                string        `json:"client_id"`
	Model                   string        `json:"model,omitempty"`
	TaskType                string        `json:"task_type,omitempty"`
	TaskTypeCode            *uint8        `json:"task_type_code,omitempty"`
	RequestTime             time.Time     `json:"request_time"`
	ResponseTime            *time.Time    `json:"response_time,omitempty"`
	DurationSeconds         *float64      `json:"duration_seconds,omitempty"`
	Request                 any           `json:"request,omitempty"`
	Response                any           `json:"response,omitempty"`
	Error                   string        `json:"error,omitempty"`
	PrimaryTaskIDCommitment string        `json:"primary_task_id_commitment"`
	FinalTaskIDCommitment   string        `json:"final_task_id_commitment,omitempty"`
	ParallelTasks           []TaskTrace   `json:"parallel_tasks"`
	ValidationGroups        []TaskGroup   `json:"validation_groups"`
	MissingData             []MissingData `json:"missing_data"`
}

type TaskTrace struct {
	LocalID          uint                 `json:"local_id"`
	ClientTaskID     uint                 `json:"client_task_id"`
	TaskID           string               `json:"task_id"`
	TaskIDCommitment string               `json:"task_id_commitment,omitempty"`
	Role             string               `json:"role"`
	TaskType         string               `json:"task_type"`
	TaskTypeCode     uint8                `json:"task_type_code"`
	Status           string               `json:"status"`
	StatusCode       int                  `json:"status_code"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
	Sequence         uint64               `json:"sequence,omitempty"`
	Events           []TaskLifecycleEvent `json:"events"`
}

type TaskLifecycleEvent struct {
	Name           string         `json:"name"`
	Timestamp      time.Time      `json:"timestamp"`
	TaskStatus     string         `json:"task_status"`
	TaskStatusCode int            `json:"task_status_code"`
	Details        map[string]any `json:"details,omitempty"`
}

type TaskGroup struct {
	TaskID string      `json:"task_id"`
	Tasks  []TaskTrace `json:"tasks"`
}

type MissingData struct {
	Step   string `json:"step"`
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

type store struct {
	mu          sync.Mutex
	maxTraces   int
	order       []string
	traces      map[string]*traceRecord
	taskIndex   map[string]string
	localIndex  map[uint]string
	clientIndex map[uint]string
}

type traceRecord struct {
	trace     Trace
	taskKeys  []string
	tasks     map[string]*TaskTrace
	eventKeys map[string]struct{}
}

var defaultStore = newStore()

func newStore() *store {
	return &store{
		traces:      make(map[string]*traceRecord),
		taskIndex:   make(map[string]string),
		localIndex:  make(map[uint]string),
		clientIndex: make(map[uint]string),
	}
}

func StartTrace(input StartTraceInput, maxTraces int) string {
	return defaultStore.startTrace(input, maxTraces)
}

func FinishTrace(primaryTaskIDCommitment string, response any, err error, finalTask *models.InferenceTask) {
	defaultStore.finishTrace(primaryTaskIDCommitment, response, err, finalTask)
}

func RegisterTasks(parentTask *models.InferenceTask, tasks []models.InferenceTask, role string) {
	defaultStore.registerRelatedTasks(parentTask, tasks, role)
}

func RegisterTask(parentTask *models.InferenceTask, task *models.InferenceTask, role string) {
	if task == nil {
		return
	}
	RegisterTasks(parentTask, []models.InferenceTask{*task}, role)
}

func RecordEvent(task *models.InferenceTask, name string, details map[string]any) {
	defaultStore.recordEvent(task, name, details)
}

func ListTraces(limit int, openAIOnly bool) []Trace {
	return defaultStore.listTraces(limit, openAIOnly)
}

func GetTrace(taskIDCommitment string) (Trace, bool) {
	return defaultStore.getTrace(taskIDCommitment)
}

func ResetForTest() {
	defaultStore = newStore()
}

func (s *store) startTrace(input StartTraceInput, maxTraces int) string {
	if maxTraces <= 0 || len(input.Tasks) == 0 {
		return ""
	}

	primary := input.Tasks[0].TaskIDCommitment
	if primary == "" {
		return ""
	}
	requestTime := input.RequestTime
	if requestTime.IsZero() {
		requestTime = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.maxTraces = maxTraces
	if existing, ok := s.traces[primary]; ok {
		existing.trace.Request = normalizePayload(input.Request)
		existing.trace.RequestTime = requestTime
		s.addTasks(existing, primary, input.Tasks, "primary")
		return primary
	}

	traceType, traceTypeCode := taskTypeFields(input.TaskType)
	record := &traceRecord{
		trace: Trace{
			Source:                  input.Source,
			Endpoint:                input.Endpoint,
			ClientID:                input.ClientID,
			Model:                   input.Model,
			TaskType:                traceType,
			TaskTypeCode:            traceTypeCode,
			RequestTime:             requestTime,
			Request:                 normalizePayload(input.Request),
			PrimaryTaskIDCommitment: primary,
		},
		tasks:     make(map[string]*TaskTrace),
		eventKeys: make(map[string]struct{}),
	}
	s.traces[primary] = record
	s.order = append(s.order, primary)
	s.addTasks(record, primary, input.Tasks, "primary")
	s.evictLocked()

	return primary
}

func (s *store) finishTrace(primaryTaskIDCommitment string, response any, err error, finalTask *models.InferenceTask) {
	if primaryTaskIDCommitment == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.traces[primaryTaskIDCommitment]
	if !ok {
		return
	}
	now := time.Now()
	duration := now.Sub(record.trace.RequestTime).Seconds()
	record.trace.ResponseTime = &now
	record.trace.DurationSeconds = &duration
	record.trace.Response = normalizePayload(response)
	if err != nil {
		record.trace.Error = err.Error()
	}
	if finalTask != nil {
		record.trace.FinalTaskIDCommitment = finalTask.TaskIDCommitment
		s.addTasks(record, primaryTaskIDCommitment, []models.InferenceTask{*finalTask}, "primary")
	}
}

func (s *store) registerRelatedTasks(parentTask *models.InferenceTask, tasks []models.InferenceTask, role string) {
	if parentTask == nil || len(tasks) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	primary := s.primaryForTaskLocked(parentTask)
	if primary == "" {
		return
	}
	record := s.traces[primary]
	s.addTasks(record, primary, tasks, role)
}

func (s *store) recordEvent(task *models.InferenceTask, name string, details map[string]any) {
	if task == nil || name == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	primary := s.primaryForTaskLocked(task)
	if primary == "" {
		return
	}
	record := s.traces[primary]
	s.addTasks(record, primary, []models.InferenceTask{*task}, inferTaskRole(record, task))

	taskKey := keyForTask(*task)
	taskTrace := record.tasks[taskKey]
	eventKey := fmt.Sprintf("%s:%s:%d", taskKey, name, task.Status)
	if _, exists := record.eventKeys[eventKey]; exists {
		return
	}
	record.eventKeys[eventKey] = struct{}{}
	taskTrace.Events = append(taskTrace.Events, TaskLifecycleEvent{
		Name:           name,
		Timestamp:      time.Now(),
		TaskStatus:     taskStatusName(task.Status),
		TaskStatusCode: int(task.Status),
		Details:        normalizeDetails(details),
	})
}

func (s *store) listTraces(limit int, openAIOnly bool) []Trace {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}
	result := make([]Trace, 0, limit)
	for i := len(s.order) - 1; i >= 0 && len(result) < limit; i-- {
		record := s.traces[s.order[i]]
		if record == nil {
			continue
		}
		if openAIOnly && !isOpenAISource(record.trace.Source) {
			continue
		}
		result = append(result, record.snapshot())
	}
	return result
}

func (s *store) getTrace(taskIDCommitment string) (Trace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	primary := taskIDCommitment
	if mapped, ok := s.taskIndex[taskIDCommitment]; ok {
		primary = mapped
	}
	record, ok := s.traces[primary]
	if !ok {
		return Trace{}, false
	}
	return record.snapshot(), true
}

func (s *store) addTasks(record *traceRecord, primary string, tasks []models.InferenceTask, role string) {
	for _, task := range tasks {
		taskKey := keyForTask(task)
		existing, ok := record.tasks[taskKey]
		if !ok {
			taskTrace := taskToTrace(task, role)
			record.tasks[taskKey] = &taskTrace
			record.taskKeys = append(record.taskKeys, taskKey)
		} else {
			updateTaskTrace(existing, task)
			if existing.Role == "" {
				existing.Role = role
			}
		}

		if task.TaskIDCommitment != "" {
			s.taskIndex[task.TaskIDCommitment] = primary
		}
		if task.ID != 0 {
			s.localIndex[task.ID] = primary
		}
		if task.ClientTaskID != 0 {
			s.clientIndex[task.ClientTaskID] = primary
		}
	}
	record.trace.ParallelTasks = record.orderedTasks()
	record.trace.ValidationGroups = buildValidationGroups(record.trace.ParallelTasks)
}

func (s *store) primaryForTaskLocked(task *models.InferenceTask) string {
	if task.TaskIDCommitment != "" {
		if primary, ok := s.taskIndex[task.TaskIDCommitment]; ok {
			return primary
		}
	}
	if task.ID != 0 {
		if primary, ok := s.localIndex[task.ID]; ok {
			return primary
		}
	}
	if task.ClientTaskID != 0 {
		if primary, ok := s.clientIndex[task.ClientTaskID]; ok {
			return primary
		}
	}
	return ""
}

func (s *store) evictLocked() {
	for s.maxTraces > 0 && len(s.order) > s.maxTraces {
		primary := s.order[0]
		s.order = s.order[1:]
		record := s.traces[primary]
		delete(s.traces, primary)
		if record == nil {
			continue
		}
		for _, task := range record.tasks {
			if task.TaskIDCommitment != "" {
				delete(s.taskIndex, task.TaskIDCommitment)
			}
			if task.LocalID != 0 {
				delete(s.localIndex, task.LocalID)
			}
			if task.ClientTaskID != 0 {
				delete(s.clientIndex, task.ClientTaskID)
			}
		}
	}
}

func (record *traceRecord) snapshot() Trace {
	trace := record.trace
	trace.ParallelTasks = record.orderedTasks()
	trace.ValidationGroups = buildValidationGroups(trace.ParallelTasks)
	trace.MissingData = buildMissingData(trace)
	return trace
}

func (record *traceRecord) orderedTasks() []TaskTrace {
	tasks := make([]TaskTrace, 0, len(record.taskKeys))
	for _, key := range record.taskKeys {
		if task := record.tasks[key]; task != nil {
			copyTask := *task
			copyTask.Events = append([]TaskLifecycleEvent(nil), task.Events...)
			tasks = append(tasks, copyTask)
		}
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].LocalID < tasks[j].LocalID
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	return tasks
}

func keyForTask(task models.InferenceTask) string {
	if task.ID != 0 {
		return fmt.Sprintf("local:%d", task.ID)
	}
	if task.TaskIDCommitment != "" {
		return "commitment:" + task.TaskIDCommitment
	}
	return "task:" + task.TaskID
}

func taskToTrace(task models.InferenceTask, role string) TaskTrace {
	return TaskTrace{
		LocalID:          task.ID,
		ClientTaskID:     task.ClientTaskID,
		TaskID:           task.TaskID,
		TaskIDCommitment: task.TaskIDCommitment,
		Role:             role,
		TaskType:         taskTypeName(task.TaskType),
		TaskTypeCode:     uint8(task.TaskType),
		Status:           taskStatusName(task.Status),
		StatusCode:       int(task.Status),
		CreatedAt:        task.CreatedAt,
		UpdatedAt:        task.UpdatedAt,
		Sequence:         task.Sequence,
	}
}

func updateTaskTrace(taskTrace *TaskTrace, task models.InferenceTask) {
	taskTrace.LocalID = task.ID
	taskTrace.ClientTaskID = task.ClientTaskID
	taskTrace.TaskID = task.TaskID
	taskTrace.TaskIDCommitment = task.TaskIDCommitment
	taskTrace.TaskType = taskTypeName(task.TaskType)
	taskTrace.TaskTypeCode = uint8(task.TaskType)
	taskTrace.Status = taskStatusName(task.Status)
	taskTrace.StatusCode = int(task.Status)
	taskTrace.UpdatedAt = task.UpdatedAt
	taskTrace.Sequence = task.Sequence
	if taskTrace.CreatedAt.IsZero() {
		taskTrace.CreatedAt = task.CreatedAt
	}
}

func inferTaskRole(record *traceRecord, task *models.InferenceTask) string {
	key := keyForTask(*task)
	if existing := record.tasks[key]; existing != nil && existing.Role != "" {
		return existing.Role
	}
	if task.SamplingSeed != "" {
		return "validation"
	}
	return "primary"
}

func normalizePayload(value any) any {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		return string(data)
	}
	return result
}

func normalizeDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	normalized := normalizePayload(details)
	if result, ok := normalized.(map[string]any); ok {
		return result
	}
	return map[string]any{"value": normalized}
}

func taskTypeFields(taskType *models.ChainTaskType) (string, *uint8) {
	if taskType == nil {
		return "", nil
	}
	code := uint8(*taskType)
	return taskTypeName(*taskType), &code
}

func taskTypeName(taskType models.ChainTaskType) string {
	switch taskType {
	case models.TaskTypeSD:
		return "sd"
	case models.TaskTypeLLM:
		return "llm"
	case models.TaskTypeSDFTLora:
		return "sd_finetune_lora"
	default:
		return fmt.Sprintf("unknown_%d", taskType)
	}
}

func taskStatusName(status models.TaskStatus) string {
	switch status {
	case models.InferenceTaskPending:
		return "pending"
	case models.InferenceTaskCreated:
		return "created"
	case models.InferenceTaskStarted:
		return "started"
	case models.InferenceTaskParamsUploaded:
		return "params_uploaded"
	case models.InferenceTaskScoreReady:
		return "score_ready"
	case models.InferenceTaskErrorReported:
		return "error_reported"
	case models.InferenceTaskValidated:
		return "validated"
	case models.InferenceTaskEndAborted:
		return "end_aborted"
	case models.InferenceTaskEndGroupRefund:
		return "end_group_refund"
	case models.InferenceTaskEndInvalidated:
		return "end_invalidated"
	case models.InferenceTaskEndSuccess:
		return "end_success"
	case models.InferenceTaskResultDownloaded:
		return "result_downloaded"
	case models.InferenceTaskNeedCancel:
		return "need_cancel"
	default:
		return fmt.Sprintf("unknown_%d", status)
	}
}

func buildValidationGroups(tasks []TaskTrace) []TaskGroup {
	groupsByID := make(map[string][]TaskTrace)
	groupIDs := make([]string, 0)
	for _, task := range tasks {
		if task.TaskID == "" {
			continue
		}
		if task.Role != "validation" && !strings.EqualFold(task.Role, "retry") {
			continue
		}
		if _, ok := groupsByID[task.TaskID]; !ok {
			groupIDs = append(groupIDs, task.TaskID)
		}
		groupsByID[task.TaskID] = append(groupsByID[task.TaskID], task)
	}

	groups := make([]TaskGroup, 0, len(groupIDs))
	for _, taskID := range groupIDs {
		groups = append(groups, TaskGroup{
			TaskID: taskID,
			Tasks:  groupsByID[taskID],
		})
	}
	return groups
}

func buildMissingData(trace Trace) []MissingData {
	missing := make([]MissingData, 0)
	if trace.ResponseTime == nil {
		missing = append(missing, MissingData{Step: "api_response", Field: "response_time", Reason: "api_call_not_finished"})
	}
	for _, task := range trace.ParallelTasks {
		if task.TaskIDCommitment == "" {
			missing = append(missing, MissingData{Step: "task", Field: "task_id_commitment", Reason: "task_not_submitted_to_relay"})
		}
		if len(task.Events) == 0 {
			missing = append(missing, MissingData{Step: "task_lifecycle", Field: "events", Reason: "no_worker_event_recorded"})
		}
	}
	return missing
}

func isOpenAISource(source string) bool {
	return source == SourceOpenAIChatCompletions || source == SourceOpenAICompletions
}
