package apikey

import (
	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type ChangeRateLimitInput struct {
	APIKey    string `path:"api_key" json:"api_key" description:"API key" validate:"required"`
	RateLimit int64  `json:"rate_limit" description:"Rate limit" validate:"required"`
}

type ChangeRateLimitInputWithSignature struct {
	ChangeRateLimitInput
	Timestamp int64  `form:"timestamp" json:"timestamp" description:"Signature timestamp" validate:"required"`
	Signature string `form:"signature" json:"signature" description:"Signature" validate:"required"`
}

func ChangeRateLimit(c *gin.Context, in *ChangeRateLimitInputWithSignature) (*response.Response, error) {
	match, address, err := tools.ValidateSignature(in.ChangeRateLimitInput, in.Timestamp, in.Signature)

	if err != nil || !match {

		if err != nil {
			log.Debugln("error in sig validate: " + err.Error())
		}

		validationErr := response.NewValidationErrorResponse("signature", "Invalid signature")
		return nil, validationErr
	}
	appConfig := config.GetConfig()
	if address != appConfig.Blockchain.Account.Address {
		validationErr := response.NewValidationErrorResponse("client_id", "Invalid signer")
		return nil, validationErr
	}
	apiKey, err := tools.ValidateAPIKey(c.Request.Context(), config.GetDB(), in.APIKey)
	if err != nil {
		return nil, mapValidateAPIKeyError(err)
	}

	if err := tools.ChangeRateLimit(c.Request.Context(), config.GetDB(), apiKey, in.RateLimit); err != nil {
		log.Debugln("error in change rate limit: " + err.Error())
		return nil, response.NewExceptionResponse(err)
	}

	return &response.Response{}, nil
}
