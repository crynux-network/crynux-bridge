package relay

import (
	"context"
	"crynux_bridge/config"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

type QueuedTasksCountResponse struct {
	Message string `json:"message"`
	Data    int64  `json:"data"`
}

type NodeNumber struct {
	AllNodes    uint64 `json:"all_nodes"`
	BusyNodes   uint64 `json:"busy_nodes"`
	ActiveNodes uint64 `json:"active_nodes"`
}

type NodeNumberResponse struct {
	Message string     `json:"message"`
	Data    NodeNumber `json:"data"`
}

type NodeStats struct {
	TotalNodes     uint64
	AvailableNodes uint64
}

type BalanceResponse struct {
	Message string `json:"message"`
	Data    string `json:"data"`
}

func GetQueuedTasks(ctx context.Context) (int64, error) {
	appConfig := config.GetConfig()
	reqUrl := appConfig.Relay.BaseURL + "/v1/stats/queue/count"

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(timeoutCtx, "GET", reqUrl, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if err := processRelayResponse(resp); err != nil {
		log.Errorf("Relay: GetQueuedTasks error: %v", err)
		return 0, err
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	res := QueuedTasksCountResponse{}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return 0, err
	}
	return res.Data, nil
}

func GetNodeStats(ctx context.Context) (*NodeStats, error) {
	appConfig := config.GetConfig()
	reqUrl := appConfig.Relay.BaseURL + "/v1/network/nodes/number"

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(timeoutCtx, "GET", reqUrl, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := processRelayResponse(resp); err != nil {
		log.Errorf("Relay: GetNodeStats error: %v", err)
		return nil, err
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	res := NodeNumberResponse{}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, err
	}

	availableNodes := uint64(0)
	if res.Data.ActiveNodes > res.Data.BusyNodes {
		availableNodes = res.Data.ActiveNodes - res.Data.BusyNodes
	}

	return &NodeStats{
		TotalNodes:     res.Data.AllNodes,
		AvailableNodes: availableNodes,
	}, nil
}

func GetBalance(ctx context.Context, address string) (*big.Int, error) {
	appConfig := config.GetConfig()
	reqUrl := appConfig.Relay.BaseURL + "/v1/balance/" + address

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(timeoutCtx, "GET", reqUrl, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := processRelayResponse(resp); err != nil {
		log.Errorf("Relay: GetBalance error: %v", err)
		return nil, err
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	res := BalanceResponse{}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, err
	}

	balance, ok := big.NewInt(0).SetString(res.Data, 10)
	if !ok {
		return nil, errors.New("failed to convert balance string to big.Int")
	}
	return balance, nil
}
