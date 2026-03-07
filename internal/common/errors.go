package common

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// OpenStackError represents a standard OpenStack API error
type OpenStackError struct {
	StatusCode int
	Code       string
	Message    string
	Details    string // Optional additional details
}

func (e *OpenStackError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ToJSON converts the error to OpenStack-compatible JSON format
func (e *OpenStackError) ToJSON() gin.H {
	errorBody := gin.H{
		"message": e.Message,
		"code":    e.StatusCode,
	}

	if e.Details != "" {
		errorBody["details"] = e.Details
	}

	return gin.H{
		e.Code: errorBody,
	}
}

// SendError sends an OpenStack-formatted error response
func SendError(c *gin.Context, err *OpenStackError) {
	c.JSON(err.StatusCode, err.ToJSON())
}

// Common error constructors

func NewUnauthorizedError(message string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusUnauthorized,
		Code:       "unauthorized",
		Message:    message,
	}
}

func NewForbiddenError(message string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusForbidden,
		Code:       "forbidden",
		Message:    message,
	}
}

func NewNotFoundError(resource string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusNotFound,
		Code:       "itemNotFound",
		Message:    fmt.Sprintf("%s could not be found.", resource),
	}
}

func NewConflictError(message string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusConflict,
		Code:       "conflict",
		Message:    message,
	}
}

func NewBadRequestError(message string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusBadRequest,
		Code:       "badRequest",
		Message:    message,
	}
}

func NewBadRequestErrorWithDetails(message, details string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusBadRequest,
		Code:       "badRequest",
		Message:    message,
		Details:    details,
	}
}

func NewServiceUnavailableError(message string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusServiceUnavailable,
		Code:       "serviceUnavailable",
		Message:    message,
	}
}

func NewInternalServerError(message string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusInternalServerError,
		Code:       "computeFault",
		Message:    message,
	}
}

func NewQuotaExceededError(resource string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusRequestEntityTooLarge,
		Code:       "overLimit",
		Message:    fmt.Sprintf("Quota exceeded for resource: %s", resource),
	}
}

func NewMethodNotAllowedError(method string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusMethodNotAllowed,
		Code:       "badMethod",
		Message:    fmt.Sprintf("The method %s is not allowed for this resource.", method),
	}
}

func NewRateLimitError(message string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusTooManyRequests,
		Code:       "overLimit",
		Message:    message,
	}
}

func NewBuildInProgressError() *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusConflict,
		Code:       "conflictingRequest",
		Message:    "Cannot perform action, instance is building.",
	}
}

func NewInvalidStateError(message string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusConflict,
		Code:       "conflictingRequest",
		Message:    message,
	}
}

// WrapError wraps a Go error into an OpenStack error
func WrapError(err error, statusCode int, code string) *OpenStackError {
	return &OpenStackError{
		StatusCode: statusCode,
		Code:       code,
		Message:    err.Error(),
	}
}

// IsNotFound checks if an error is a "not found" error
func IsNotFound(err error) bool {
	if osErr, ok := err.(*OpenStackError); ok {
		return osErr.StatusCode == http.StatusNotFound
	}
	return false
}

// IsConflict checks if an error is a conflict error
func IsConflict(err error) bool {
	if osErr, ok := err.(*OpenStackError); ok {
		return osErr.StatusCode == http.StatusConflict
	}
	return false
}
