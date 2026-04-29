package response

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/loopfz/gadgeto/tonic"
)

type ResponseMessage interface {
	SetMessage(message string)
	GetMessage() string
}

type ErrorResponseMessage interface {
	GetErrorType() string
	SetErrorType(errType string)
}

type ExceptionResponseMessage interface {
	GetException() string
}

type Response struct {
	Message string `json:"message" description:"The response message. Will be 'success' or a detailed error type"`
}

func (r *Response) SetMessage(message string) {
	r.Message = message
}

func (r *Response) GetMessage() string {
	return r.Message
}

type ErrorResponse struct {
	Response
}

func (r *ErrorResponse) GetErrorType() string {
	return r.GetMessage()
}

func (r *ErrorResponse) SetErrorType(errType string) {
	r.SetMessage(errType)
}

func (r *ErrorResponse) Error() string {
	return r.GetErrorType()
}

type ExceptionResponse struct {
	Response
}

func (e *ExceptionResponse) Error() string {
	return e.Message
}

func (e *ExceptionResponse) GetException() string {
	return e.Message
}

func NewExceptionResponse(err error) *ExceptionResponse {
	r := &ExceptionResponse{}
	r.SetMessage(err.Error())
	return r
}

func TonicErrorResponse(ctx *gin.Context, err error) (int, interface{}) {
	var e tonic.BindError
	if errors.As(err, &e) {
		validationErrorResponse := NewValidationErrorResponse("", "")
		validationErr := e.ValidationErrors()
		// We return only the first error
		for _, err := range validationErr {
			validationErrorResponse.SetFieldName(err.Field())
			validationErrorResponse.SetFieldMessage(formatValidationFieldError(err))
			return 400, validationErrorResponse
		}
		validationErrorResponse.SetFieldName(e.GetField())
		validationErrorResponse.SetFieldMessage(formatBindErrorMessage(e.GetField(), e.GetMessage()))
		return 400, validationErrorResponse
	}

	if err, ok := err.(ErrorResponseMessage); ok {
		return 400, err
	}

	if err, ok := err.(ExceptionResponseMessage); ok {
		return 500, err
	}

	return 500, NewExceptionResponse(err)
}

func TonicRenderResponse(ctx *gin.Context, statusCode int, payload interface{}) {

	if payload, ok := payload.(ResponseMessage); ok {
		if payload.GetMessage() == "" {
			payload.SetMessage("success")
		}
	}

	tonic.DefaultRenderHook(ctx, statusCode, payload)
}

type validationFieldError interface {
	Field() string
	Tag() string
	Param() string
}

func formatValidationFieldError(err validationFieldError) string {
	fieldName := err.Field()
	if fieldName == "" {
		fieldName = "field"
	}

	switch err.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", fieldName)
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", fieldName, err.Param())
	case "min":
		return fmt.Sprintf("%s must be at least %s", fieldName, err.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s", fieldName, err.Param())
	case "len":
		return fmt.Sprintf("%s must be exactly %s characters", fieldName, err.Param())
	case "gt":
		return fmt.Sprintf("%s must be greater than %s", fieldName, err.Param())
	case "gte":
		return fmt.Sprintf("%s must be greater than or equal to %s", fieldName, err.Param())
	case "lt":
		return fmt.Sprintf("%s must be less than %s", fieldName, err.Param())
	case "lte":
		return fmt.Sprintf("%s must be less than or equal to %s", fieldName, err.Param())
	default:
		if err.Param() != "" {
			return fmt.Sprintf("%s failed %s validation (expected %s)", fieldName, err.Tag(), err.Param())
		}
		return fmt.Sprintf("%s failed %s validation", fieldName, err.Tag())
	}
}

func formatBindErrorMessage(field, message string) string {
	trimmedMessage := strings.TrimSpace(message)
	if trimmedMessage == "" {
		if strings.TrimSpace(field) == "" {
			return "invalid request"
		}
		return fmt.Sprintf("%s is invalid", field)
	}
	return trimmedMessage
}
