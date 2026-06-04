package network

import (
	"crynux_bridge/api/v1/response"
	"crynux_bridge/relay"

	"github.com/gin-gonic/gin"
)

type NodeStats struct {
	NumTotalNodes     uint64 `json:"num_total_nodes"`
	NumAvailableNodes uint64 `json:"num_available_nodes"`
}

type GetNodeStatsOutput struct {
	response.Response
	Data NodeStats `json:"data"`
}

func GetNodeStats(c *gin.Context) (*GetNodeStatsOutput, error) {
	stats, err := relay.GetNodeStats(c.Request.Context())
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}

	return &GetNodeStatsOutput{
		Data: NodeStats{
			NumAvailableNodes: stats.AvailableNodes,
			NumTotalNodes:     stats.TotalNodes,
		},
	}, nil
}
