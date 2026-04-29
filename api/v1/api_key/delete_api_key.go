package apikey

import (
	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"crynux_bridge/config"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type DeleteAPIKeyInput struct {
	APIKey string `path:"api_key" json:"api_key" description:"API key" validate:"required"`
}

type DeleteAPIKeyInputWithSignature struct {
	DeleteAPIKeyInput
	Timestamp int64  `query:"timestamp" json:"timestamp" description:"Signature timestamp" validate:"required"`
	Signature string `query:"signature" json:"signature" description:"Signature" validate:"required"`
}

func DeleteAPIKey(c *gin.Context, in *DeleteAPIKeyInputWithSignature) (*response.Response, error) {
	match, address, err := tools.ValidateSignature(in.DeleteAPIKeyInput, in.Timestamp, in.Signature)

	if err != nil || !match {

		if err != nil {
			log.Debugln("error in sig validate: " + err.Error())
		}

		validationErr := response.NewValidationErrorResponse("signature", "Invalid signature")
		return nil, validationErr
	}
	appConfig := config.GetConfig()
	if address != appConfig.Blockchain.Account.Address {
		validationErr := response.NewValidationErrorResponse("signature", "Invalid signer")
		return nil, validationErr
	}

	apiKey, err := tools.ValidateAPIKey(c.Request.Context(), config.GetDB(), in.APIKey)
	if err != nil {
		return nil, mapValidateAPIKeyError(err)
	}

	// delete the API key
	if err := tools.DeleteAPIKey(c.Request.Context(), config.GetDB(), apiKey); err != nil {
		return nil, response.NewExceptionResponse(err)
	}
	return &response.Response{}, nil
}
