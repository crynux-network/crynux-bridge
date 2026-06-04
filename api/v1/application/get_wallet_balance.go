package application

import (
	"crynux_bridge/api/v1/response"
	"crynux_bridge/config"
	"crynux_bridge/relay"
	"math/big"

	"github.com/gin-gonic/gin"
)

type WalletBalance struct {
	Address string   `json:"address"`
	Balance *big.Int `json:"balance"`
}

type GetWalletBalanceResponse struct {
	response.Response
	Data *WalletBalance `json:"data"`
}

func GetWalletBalance(c *gin.Context) (*GetWalletBalanceResponse, error) {
	appConfig := config.GetConfig()

	balance, err := relay.GetBalance(c.Request.Context(), appConfig.Blockchain.Account.Address)
	if err != nil {
		return nil, response.NewExceptionResponse(err)
	}

	return &GetWalletBalanceResponse{
		Data: &WalletBalance{
			Address: appConfig.Blockchain.Account.Address,
			Balance: balance,
		},
	}, nil
}
