package relay

import (
	"bytes"
	"context"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type BatchOutcome string

const (
	BatchOutcomeCreated          BatchOutcome = "created"
	BatchOutcomeAlreadyExists    BatchOutcome = "already_exists"
	BatchOutcomeValidated        BatchOutcome = "validated"
	BatchOutcomeAlreadyApplied   BatchOutcome = "already_applied"
	BatchOutcomeCancelled        BatchOutcome = "cancelled"
	BatchOutcomeAlreadyCancelled BatchOutcome = "already_cancelled"
	BatchOutcomeNotCancellable   BatchOutcome = "not_cancellable"
	BatchOutcomeNotFound         BatchOutcome = "not_found"
	BatchOutcomePermanentError   BatchOutcome = "permanent_error"
	BatchOutcomeTemporaryError   BatchOutcome = "temporary_error"
	BatchOutcomeConflict         BatchOutcome = "commitment_conflict"
)

type BatchCreateItem = CreateTaskInput

type BatchCreateResult struct {
	TaskIDCommitment string                 `json:"task_id_commitment"`
	Outcome          BatchOutcome           `json:"outcome"`
	Error            string                 `json:"error,omitempty"`
	Status           models.ChainTaskStatus `json:"status"`
	Sequence         uint64                 `json:"sequence"`
	SamplingSeed     string                 `json:"sampling_seed"`
}

type BatchStatusItem struct {
	TaskIDCommitment               string                 `json:"task_id_commitment"`
	Found                          bool                   `json:"found"`
	Status                         models.ChainTaskStatus `json:"status"`
	AbortReason                    models.TaskAbortReason `json:"abort_reason"`
	TaskError                      models.TaskError       `json:"task_error"`
	Sequence                       uint64                 `json:"sequence"`
	SamplingSeed                   string                 `json:"sampling_seed"`
	SelectedExecutionGPU           string                 `json:"execution_gpu"`
	SelectedExecutionGPUVram       uint64                 `json:"execution_gpu_vram"`
	EstimatedExecutionCompletionAt *time.Time             `json:"estimated_completion_time,omitempty"`
	ResultAvailable                bool                   `json:"result_available"`
}

type BatchValidationItem struct {
	PublicKey         string   `json:"public_key"`
	TaskID            string   `json:"task_id"`
	TaskIDCommitments []string `json:"task_id_commitments"`
	VrfProof          string   `json:"vrf_proof"`
}

type BatchMutationResult struct {
	TaskIDCommitments []string     `json:"task_id_commitments,omitempty"`
	TaskIDCommitment  string       `json:"task_id_commitment,omitempty"`
	Outcome           BatchOutcome `json:"outcome"`
	Error             string       `json:"error,omitempty"`
}

type signedBatchRequest[T any] struct {
	Items     []T    `json:"items"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Signature string `json:"signature,omitempty"`
}

func BatchCreateTasks(ctx context.Context, tasks []*models.InferenceTask) ([]BatchCreateResult, error) {
	items := make([]BatchCreateItem, len(tasks))
	for i, task := range tasks {
		items[i] = *buildCreateTaskInput(task, task.TaskFee)
	}
	var result []BatchCreateResult
	err := doSignedBatch(ctx, "/v1/inference_tasks/batch", items, &result)
	return result, err
}

func BatchGetTaskStatus(ctx context.Context, commitments []string) ([]BatchStatusItem, error) {
	type input struct {
		TaskIDCommitments []string `json:"task_id_commitments"`
	}
	type request struct {
		input
		Timestamp int64  `json:"timestamp,omitempty"`
		Signature string `json:"signature,omitempty"`
	}
	params := input{TaskIDCommitments: commitments}
	appConfig := config.GetConfig()
	timestamp, signature, err := SignData(&params, appConfig.Blockchain.Account.PrivateKey)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(request{input: params, Timestamp: timestamp, Signature: signature})
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(
		callCtx,
		http.MethodPost,
		appConfig.Relay.BaseURL+"/v1/inference_tasks/batch/status",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := processRelayResponse(resp); err != nil {
		return nil, err
	}
	var result []BatchStatusItem
	if err := decodeBatchResponse(resp.Body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func BatchValidateTasks(ctx context.Context, items []BatchValidationItem) ([]BatchMutationResult, error) {
	var result []BatchMutationResult
	err := doSignedBatch(ctx, "/v1/inference_tasks/batch/validate", items, &result)
	return result, err
}

func BatchAbortTasks(ctx context.Context, commitments []string) ([]BatchMutationResult, error) {
	type item struct {
		TaskIDCommitment string                 `json:"task_id_commitment"`
		AbortReason      models.TaskAbortReason `json:"abort_reason"`
	}
	items := make([]item, len(commitments))
	for i, commitment := range commitments {
		items[i] = item{TaskIDCommitment: commitment, AbortReason: models.TaskAbortCreatorCancelled}
	}
	var result []BatchMutationResult
	err := doSignedBatch(ctx, "/v1/inference_tasks/batch/abort", items, &result)
	return result, err
}

func doSignedBatch[T any, R any](ctx context.Context, endpoint string, items []T, result *[]R) error {
	request := signedBatchRequest[T]{Items: items}
	appConfig := config.GetConfig()
	timestamp, signature, err := SignData(&request, appConfig.Blockchain.Account.PrivateKey)
	if err != nil {
		return err
	}
	request.Timestamp = timestamp
	request.Signature = signature
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, appConfig.Relay.BaseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := processRelayResponse(resp); err != nil {
		return err
	}
	return decodeBatchResponse(resp.Body, result)
}

func decodeBatchResponse[R any](body io.Reader, result *[]R) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&envelope); err != nil {
		return err
	}
	if err := json.Unmarshal(envelope.Data, result); err == nil {
		return nil
	}
	var wrapped struct {
		Items   []R `json:"items"`
		Results []R `json:"results"`
	}
	if err := json.Unmarshal(envelope.Data, &wrapped); err != nil {
		return err
	}
	if wrapped.Items != nil {
		*result = wrapped.Items
	} else {
		*result = wrapped.Results
	}
	return nil
}

func DownloadWholeTaskResult(ctx context.Context, taskIDCommitment, filename string) error {
	appConfig := config.GetConfig()
	params := &GetTaskByCommitmentInput{TaskIDCommitment: taskIDCommitment}
	timestamp, signature, err := SignData(params, appConfig.Blockchain.Account.PrivateKey)
	if err != nil {
		return err
	}
	requestURL := appConfig.Relay.BaseURL + "/v1/inference_tasks/" + url.PathEscape(taskIDCommitment) + "/results"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	query := req.URL.Query()
	query.Set("timestamp", strconv.FormatInt(timestamp, 10))
	query.Set("signature", signature)
	req.URL.RawQuery = query.Encode()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download whole task result: %s", resp.Status)
	}
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	return err
}
