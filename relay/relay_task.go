package relay

import (
	"bytes"
	"context"
	"crynux_bridge/config"
	"crynux_bridge/models"
	"crynux_bridge/utils"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type GetTaskByCommitmentInput struct {
	TaskIDCommitment string `json:"task_id_commitment"`
}

type GetSelectedNodeInfoInput struct {
	TaskIDCommitment string `json:"task_id_commitment"`
}

type CreateTaskInput struct {
	TaskIDCommitment string   `json:"task_id_commitment"`
	MinVram          uint64   `json:"min_vram"`
	Nonce            string   `json:"nonce"`
	RequiredGpu      string   `json:"required_gpu"`
	RequiredGpuVram  uint64   `json:"required_gpu_vram"`
	TaskArgs         string   `json:"task_args"`
	TaskModelIds     []string `json:"task_model_ids"`
	TaskSize         uint64   `json:"task_size"`
	TaskType         int      `json:"task_type"`
	TaskVersion      string   `json:"task_version"`
	Timeout          uint64   `json:"timeout"`
	TaskFee          string   `json:"task_fee"`
}

type ValidateTaskInput struct {
	PublicKey         string   `json:"public_key"`
	TaskID            string   `json:"task_id"`
	TaskIDCommitments []string `json:"task_id_commitments"`
	VrfProof          string   `json:"vrf_proof"`
	Timestamp         int64    `json:"timestamp,omitempty"`
	Signature         string   `json:"signature,omitempty"`
}

type CancelTaskInput struct {
	TaskIDCommitment string `json:"task_id_commitment"`
	AbortReason      int    `json:"abort_reason"`
	Timestamp        int64  `json:"timestamp,omitempty"`
	Signature        string `json:"signature,omitempty"`
}

type CheckBalanceInput struct {
	Address string `json:"address"`
}

// Parse the "data" field of relay response, and store it in parsedData
func parseRelayResponseData(resp *http.Response, parsedData any) error {
	if resp.StatusCode != 200 {
		return errors.New("response status code is not 200")
	}

	// read response body
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// parse JSON
	content := make(map[string]interface{})
	if err := json.Unmarshal(respBytes, &content); err != nil {
		return err
	}

	// get "data" field
	data, ok := content["data"]
	if !ok {
		return errors.New("data field is missing")
	}

	// check parsedData, make sure it is a non-nil pointer
	val := reflect.ValueOf(parsedData)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return errors.New("parsedData must be a non-nil pointer")
	}

	// parse "data" field according to its type
	switch v := data.(type) {
	case string:
		if strPtr, ok := parsedData.(*string); ok {
			*strPtr = v
		} else {
			return errors.New("parsedData must be *string for string data")
		}

	case float64:
		if strPtr, ok := parsedData.(*string); ok {
			*strPtr = fmt.Sprintf("%.0f", v)
		} else {
			return errors.New("parsedData must be *string for numeric data")
		}

	case map[string]interface{}:
		// target type must be a pointer to a struct
		jsonBytes, err := json.Marshal(v) // convert map to JSON
		if err != nil {
			return err
		}
		if err := json.Unmarshal(jsonBytes, parsedData); err != nil {
			return err
		}

	default:
		return errors.New("data field has an unsupported type")
	}

	return nil
}

func GetTaskByCommitment(ctx context.Context, taskIDCommitment string) (*models.RelayTask, error) {
	appConfig := config.GetConfig()

	params := &GetTaskByCommitmentInput{
		TaskIDCommitment: taskIDCommitment,
	}

	timestamp, signature, err := SignData(params, appConfig.Blockchain.Account.PrivateKey)
	if err != nil {
		return nil, err
	}

	reqUrl := appConfig.Relay.BaseURL + "/v1/inference_tasks/" + taskIDCommitment

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(timeoutCtx, "GET", reqUrl, nil)
	query := req.URL.Query()
	query.Add("timestamp", strconv.FormatInt(timestamp, 10))
	query.Add("signature", signature)
	req.URL.RawQuery = query.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := processRelayResponse(resp); err != nil {
		log.Errorf("Relay: get task by taskIDCommitment %s error: %v", taskIDCommitment, err)
		return nil, err
	}

	relayTask := new(models.RelayTask)
	err = parseRelayResponseData(resp, relayTask)
	if err != nil {
		log.Errorf("Relay: get task by taskIDCommitment %s error: %v", taskIDCommitment, err)
		return nil, err
	}

	log.Debugf("Relay: get task %s", taskIDCommitment)
	return relayTask, nil
}

func CreateTask(ctx context.Context, task *models.InferenceTask) error {
	appConfig := config.GetConfig()

	taskFee := utils.GweiToWei(big.NewInt(int64(task.TaskFee)))

	var timeout uint64
	if task.Timeout != 0 {
		timeout = task.Timeout
	} else if task.TaskType == models.TaskTypeSDFTLora {
		timeout = appConfig.Task.SDFinetuneTimeout * 60
	} else {
		timeout = appConfig.Task.DefaultTimeout * 60
	}

	params := &CreateTaskInput{
		TaskIDCommitment: task.TaskIDCommitment,
		MinVram:          task.MinVram,
		Nonce:            task.Nonce,
		RequiredGpu:      task.RequiredGPU,
		RequiredGpuVram:  task.RequiredGPUVram,
		TaskArgs:         task.TaskArgs,
		TaskModelIds:     task.TaskModelIDs,
		TaskSize:         task.TaskSize,
		TaskType:         int(task.TaskType),
		TaskVersion:      task.TaskVersion,
		TaskFee:          taskFee.String(),
		Timeout:          timeout,
	}

	timestamp, signature, err := SignData(params, appConfig.Blockchain.Account.PrivateKey)
	if err != nil {
		return err
	}

	var checkpointFilePath string
	if task.TaskType == models.TaskTypeSDFTLora {
		checkpoint, err := models.GetSDFTTaskConfigCheckpoint(task.TaskArgs)
		if err != nil {
			return err
		}
		if checkpoint != "" {
			checkpointFilePath = checkpoint
		}
	}

	var body io.Reader
	var contentType string
	var multipartWriter *multipart.Writer

	if checkpointFilePath == "" {
		form := url.Values{}
		form.Add("min_vram", strconv.FormatUint(task.MinVram, 10))
		form.Add("nonce", task.Nonce)
		form.Add("required_gpu", task.RequiredGPU)
		form.Add("required_gpu_vram", strconv.FormatUint(task.RequiredGPUVram, 10))
		form.Add("task_args", task.TaskArgs)
		form.Add("task_size", strconv.FormatUint(task.TaskSize, 10))
		form.Add("task_type", strconv.Itoa(int(task.TaskType)))
		form.Add("task_version", task.TaskVersion)
		form.Add("task_fee", taskFee.String())
		form.Add("timestamp", strconv.FormatInt(timestamp, 10))
		form.Add("signature", signature)
		form.Add("timeout", strconv.FormatUint(timeout, 10))
		for _, modelID := range task.TaskModelIDs {
			form.Add("task_model_ids", modelID)
		}
		body = strings.NewReader(form.Encode())
		contentType = "application/x-www-form-urlencoded"
	} else {
		pr, pw := io.Pipe()
		multipartWriter = multipart.NewWriter(pw)

		go func() {
			defer pw.Close()
			defer multipartWriter.Close()

			multipartWriter.WriteField("min_vram", strconv.FormatUint(task.MinVram, 10))
			multipartWriter.WriteField("nonce", task.Nonce)
			multipartWriter.WriteField("required_gpu", task.RequiredGPU)
			multipartWriter.WriteField("required_gpu_vram", strconv.FormatUint(task.RequiredGPUVram, 10))
			multipartWriter.WriteField("task_args", task.TaskArgs)
			multipartWriter.WriteField("task_size", strconv.FormatUint(task.TaskSize, 10))
			multipartWriter.WriteField("task_type", strconv.Itoa(int(task.TaskType)))
			multipartWriter.WriteField("task_version", task.TaskVersion)
			multipartWriter.WriteField("task_fee", taskFee.String())
			multipartWriter.WriteField("timestamp", strconv.FormatInt(timestamp, 10))
			multipartWriter.WriteField("signature", signature)
			multipartWriter.WriteField("timeout", strconv.FormatUint(timeout, 10))
			for _, modelID := range task.TaskModelIDs {
				multipartWriter.WriteField("task_model_ids", modelID)
			}

			file, err := os.Open(checkpointFilePath)
			if err != nil {
				pw.CloseWithError(fmt.Errorf("failed to open checkpoint file %s: %v", checkpointFilePath, err))
				return
			}
			defer file.Close()

			part, err := multipartWriter.CreateFormFile("checkpoint", filepath.Base(checkpointFilePath))
			if err != nil {
				pw.CloseWithError(fmt.Errorf("failed to create form file: %v", err))
				return
			}

			_, err = io.Copy(part, file)
			if err != nil {
				pw.CloseWithError(fmt.Errorf("failed to copy file content: %v", err))
				return
			}
		}()

		body = pr
		contentType = multipartWriter.FormDataContentType()
	}

	taskIDCommitment := task.TaskIDCommitment
	reqUrl := appConfig.Relay.BaseURL + "/v1/inference_tasks/" + taskIDCommitment

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(timeoutCtx, "POST", reqUrl, body)
	req.Header.Add("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := processRelayResponse(resp); err != nil {
		log.Errorf("Relay: create task %s error: %v", taskIDCommitment, err)
		return err
	}

	log.Debugf("Relay: create task %s", taskIDCommitment)
	return nil
}

// ValidateTask validates tasks (single task or task group)
func ValidateTask(ctx context.Context, tasks []*models.InferenceTask) error {
	appConfig := config.GetConfig()

	taskIDCommitments := make([]string, len(tasks))
	taskID := tasks[0].TaskID
	vrfProof := tasks[0].VRFProof
	for i, task := range tasks {
		taskIDCommitments[i] = task.TaskIDCommitment
		if task.TaskID != taskID {
			return errors.New("task ID mismatch")
		}
		if task.VRFProof != vrfProof {
			return errors.New("VRF proof mismatch")
		}
	}

	privateKey := appConfig.Blockchain.Account.PrivateKey
	publicKey, err := utils.GetPubKeyFromPrivKey(privateKey)
	if err != nil {
		log.Errorf("Relay: validate task %s error: %v", taskIDCommitments, err)
		return err
	}

	params := &ValidateTaskInput{
		PublicKey:         publicKey,
		TaskID:            taskID,
		TaskIDCommitments: taskIDCommitments,
		VrfProof:          vrfProof,
	}

	timestamp, signature, err := SignData(params, appConfig.Blockchain.Account.PrivateKey)
	if err != nil {
		return err
	}
	params.Timestamp = timestamp
	params.Signature = signature

	bs, err := json.Marshal(params)
	if err != nil {
		return err
	}
	body := bytes.NewReader(bs)

	reqUrl := appConfig.Relay.BaseURL + "/v1/inference_tasks/validate"

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(timeoutCtx, "POST", reqUrl, body)
	req.Header.Add("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := processRelayResponse(resp); err != nil {
		log.Errorf("Relay: validate task %s error: %v", taskIDCommitments, err)
		return err
	}

	log.Debugf("Relay: validate task %s success", taskIDCommitments)
	return nil
}

func CancelTask(ctx context.Context, task *models.InferenceTask, abortReason models.TaskAbortReason) error {

	appConfig := config.GetConfig()

	taskIDCommitment := task.TaskIDCommitment

	params := &CancelTaskInput{
		TaskIDCommitment: taskIDCommitment,
		AbortReason:      int(abortReason),
	}

	timestamp, signature, err := SignData(params, appConfig.Blockchain.Account.PrivateKey)
	if err != nil {
		return err
	}

	params.Timestamp = timestamp
	params.Signature = signature
	bs, err := json.Marshal(params)
	if err != nil {
		return err
	}
	body := bytes.NewReader(bs)

	reqUrl := appConfig.Relay.BaseURL + fmt.Sprintf("/v1/inference_tasks/%s/abort_reason", taskIDCommitment)

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(timeoutCtx, "POST", reqUrl, body)
	req.Header.Add("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := processRelayResponse(resp); err != nil {
		log.Errorf("Relay: cancel task %s error: %v", taskIDCommitment, err)
		return err
	}

	log.Debugf("Relay: cancel task %s success", taskIDCommitment)
	return nil
}

func CheckQuotaForTaskCreator(ctx context.Context) error {

	appConfig := config.GetConfig()

	address := appConfig.Blockchain.Account.Address

	reqUrl := appConfig.Relay.BaseURL + fmt.Sprintf("/v1/task_quota/%s", address)

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(timeoutCtx, "GET", reqUrl, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := processRelayResponse(resp); err != nil {
		log.Errorf("Relay: CheckQuotaForTaskCreator error: %v", err)
		return err
	}

	// get and check balance
	quotaStr := new(string)
	err = parseRelayResponseData(resp, quotaStr)
	if err != nil {
		log.Errorf("Relay: CheckQuotaForTaskCreator error: %v", err)
	}
	log.Debugf("Relay: CheckQuotaForTaskCreator, balance: %s", *quotaStr)

	quota, ok := big.NewInt(0).SetString(*quotaStr, 10)
	if !ok {
		return errors.New("failed to convert quota string to big.Int")
	}

	ethThreshold := utils.EtherToWei(big.NewInt(500))
	if quota.Cmp(ethThreshold) != 1 {
		return errors.New("not enough quota left")
	}

	return nil
}

func GetTaskSelectedNodeInfo(ctx context.Context, taskIDCommitment string) (*models.SelectedNodeInfo, error) {
	appConfig := config.GetConfig()

	params := &GetSelectedNodeInfoInput{
		TaskIDCommitment: taskIDCommitment,
	}

	timestamp, signature, err := SignData(params, appConfig.Blockchain.Account.PrivateKey)
	if err != nil {
		return nil, err
	}

	reqUrl := appConfig.Relay.BaseURL + "/v1/inference_tasks/" + taskIDCommitment + "/selected_node"

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(timeoutCtx, "GET", reqUrl, nil)
	query := req.URL.Query()
	query.Add("timestamp", strconv.FormatInt(timestamp, 10))
	query.Add("signature", signature)
	req.URL.RawQuery = query.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := processRelayResponse(resp); err != nil {
		log.Errorf("Relay: get selected node info by taskIDCommitment %s error: %v", taskIDCommitment, err)
		return nil, err
	}

	node := new(models.SelectedNodeInfo)
	err = parseRelayResponseData(resp, node)
	if err != nil {
		log.Errorf("Relay: get selected node info by taskIDCommitment %s error: %v", taskIDCommitment, err)
		return nil, err
	}

	log.Debugf("Relay: get task %s", taskIDCommitment)
	return node, nil
}