package llm

import (
	"crynux_bridge/api/v1/response"
	"errors"
	"fmt"
	"strings"
)

const (
	llmErrorTypeInvalidAPIKey          = "invalid_api_key"
	llmErrorTypeExpiredAPIKey          = "expired_api_key"
	llmErrorTypeInsufficientQuota      = "insufficient_quota"
	llmErrorTypeInsufficientPermission = "insufficient_permissions"
	llmErrorTypeRateLimitExceeded      = "rate_limit_exceeded"
)

func mapLLMAuthorizationError(err error) error {
	var validationErr *response.ValidationErrorResponse
	if !errors.As(err, &validationErr) {
		return err
	}

	if validationErr.GetFieldName() != "Authorization" {
		return err
	}

	message := validationErr.GetFieldMessage()
	switch message {
	case "Authorization header must start with 'Bearer '":
		apiKeyErr := response.NewValidationErrorResponse("Authorization", "invalid Authorization header, expected 'Bearer <api_key>'")
		apiKeyErr.SetErrorType(llmErrorTypeInvalidAPIKey)
		return apiKeyErr
	case "invalid API key":
		apiKeyErr := response.NewValidationErrorResponse("Authorization", "request rejected: invalid API key")
		apiKeyErr.SetErrorType(llmErrorTypeInvalidAPIKey)
		return apiKeyErr
	case "API key has expired":
		expiredErr := response.NewValidationErrorResponse("Authorization", "request rejected: API key has expired")
		expiredErr.SetErrorType(llmErrorTypeExpiredAPIKey)
		return expiredErr
	case "API key does not have required role: admin or chat":
		permErr := response.NewValidationErrorResponse("Authorization", "request rejected: API key is missing required role (admin or chat)")
		permErr.SetErrorType(llmErrorTypeInsufficientPermission)
		return permErr
	case "API key quota exceeded (use limit reached)", "use limit exceeded":
		quotaErr := response.NewValidationErrorResponse(
			"quota",
			"request rejected: API key quota exceeded",
		)
		quotaErr.SetErrorType(llmErrorTypeInsufficientQuota)
		return quotaErr
	}

	return err
}

func newRateLimitExceededError(waitTime float64) *response.ValidationErrorResponse {
	rateLimitErr := response.NewValidationErrorResponse(
		"rate_limit",
		fmt.Sprintf("request rejected: rate limit exceeded, retry after %.2f seconds", waitTime),
	)
	rateLimitErr.SetErrorType(llmErrorTypeRateLimitExceeded)
	return rateLimitErr
}

func mapLLMTaskProcessingError(err error) error {
	var validationErr *response.ValidationErrorResponse
	if !errors.As(err, &validationErr) {
		return err
	}

	if validationErr.GetFieldName() == "task_args" {
		if mappedErr := mapTaskArgsValidationError(validationErr.GetFieldMessage()); mappedErr != nil {
			return mappedErr
		}
		return response.NewExceptionResponse(errors.New("internal server error while preparing task payload"))
	}

	return err
}

func mapTaskArgsValidationError(detail string) error {
	cleanDetail := strings.TrimSpace(strings.TrimPrefix(detail, "invalid task_args: "))
	normalizedDetail := strings.ToLower(cleanDetail)

	if strings.Contains(normalizedDetail, "messages[") ||
		strings.Contains(normalizedDetail, "/messages/") {
		return response.NewValidationErrorResponse("messages", cleanDetail)
	}

	if strings.Contains(normalizedDetail, "model is missing") ||
		strings.Contains(normalizedDetail, "model is not") ||
		strings.Contains(normalizedDetail, "/model") {
		return response.NewValidationErrorResponse("model", cleanDetail)
	}

	if strings.Contains(normalizedDetail, "task_args must be valid json") {
		return nil
	}

	return nil
}
