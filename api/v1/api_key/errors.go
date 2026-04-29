package apikey

import (
	"crynux_bridge/api/v1/response"
	"crynux_bridge/api/v1/tools"
	"errors"
)

func mapValidateAPIKeyError(err error) error {
	if errors.Is(err, tools.ErrAPIKeyExpired) {
		return response.NewValidationErrorResponse("api_key", "API key has expired")
	}
	if errors.Is(err, tools.ErrAPIKeyInvalid) {
		return response.NewValidationErrorResponse("api_key", "invalid API key")
	}
	return response.NewExceptionResponse(err)
}
