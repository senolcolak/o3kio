package common

import (
	"fmt"
	"net/http"
	"strings"

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

// NewResourceNotFoundError creates a detailed not found error with resource type and ID
func NewResourceNotFoundError(resourceType, resourceID string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusNotFound,
		Code:       "itemNotFound",
		Message:    fmt.Sprintf("%s %s could not be found.", resourceType, resourceID),
		Details:    fmt.Sprintf("The requested %s does not exist or has been deleted.", resourceType),
	}
}

func NewConflictError(message string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusConflict,
		Code:       "conflict",
		Message:    message,
	}
}

// NewResourceConflictError creates a detailed conflict error
func NewResourceConflictError(resourceType, resourceName, reason string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusConflict,
		Code:       "conflict",
		Message:    fmt.Sprintf("%s '%s' already exists", resourceType, resourceName),
		Details:    reason,
	}
}

// NewOperationConflictError creates an error for conflicting operations
func NewOperationConflictError(operation, reason string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusConflict,
		Code:       "conflictingRequest",
		Message:    fmt.Sprintf("Cannot %s", operation),
		Details:    reason,
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

// NewValidationError creates a detailed validation error
func NewValidationError(field, issue, suggestion string) *OpenStackError {
	details := fmt.Sprintf("Field '%s': %s", field, issue)
	if suggestion != "" {
		details = fmt.Sprintf("%s. %s", details, suggestion)
	}
	return &OpenStackError{
		StatusCode: http.StatusBadRequest,
		Code:       "badRequest",
		Message:    "Validation failed",
		Details:    details,
	}
}

// NewMissingFieldError creates an error for missing required fields
func NewMissingFieldError(fields ...string) *OpenStackError {
	var fieldList string
	if len(fields) == 1 {
		fieldList = fields[0]
	} else {
		fieldList = strings.Join(fields, ", ")
	}
	return &OpenStackError{
		StatusCode: http.StatusBadRequest,
		Code:       "badRequest",
		Message:    "Missing required field(s)",
		Details:    fmt.Sprintf("Required field(s) missing: %s", fieldList),
	}
}

// NewInvalidValueError creates an error for invalid field values
func NewInvalidValueError(field, value, allowedValues string) *OpenStackError {
	details := fmt.Sprintf("Field '%s' has invalid value '%s'", field, value)
	if allowedValues != "" {
		details = fmt.Sprintf("%s. Allowed values: %s", details, allowedValues)
	}
	return &OpenStackError{
		StatusCode: http.StatusBadRequest,
		Code:       "badRequest",
		Message:    "Invalid field value",
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

// NewDatabaseError creates an error for database failures
func NewDatabaseError(operation, resourceType string, err error) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusInternalServerError,
		Code:       "computeFault",
		Message:    fmt.Sprintf("Database error during %s operation", operation),
		Details:    fmt.Sprintf("Failed to %s %s: %v", operation, resourceType, err),
	}
}

// NewExternalServiceError creates an error for external service failures
func NewExternalServiceError(service, operation string, err error) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusServiceUnavailable,
		Code:       "serviceUnavailable",
		Message:    fmt.Sprintf("%s service error", service),
		Details:    fmt.Sprintf("Failed to %s: %v", operation, err),
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

// NewResourceStateError creates an error for invalid resource state operations
func NewResourceStateError(resourceType, resourceID, currentState, requiredState, operation string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusConflict,
		Code:       "conflictingRequest",
		Message:    fmt.Sprintf("Cannot %s %s in current state", operation, resourceType),
		Details:    fmt.Sprintf("%s %s is in state '%s' but operation requires state '%s'", resourceType, resourceID, currentState, requiredState),
	}
}

// NewPermissionDeniedError creates an error for permission issues
func NewPermissionDeniedError(operation, resourceType, requiredRole string) *OpenStackError {
	details := fmt.Sprintf("Operation '%s' on %s requires role '%s'", operation, resourceType, requiredRole)
	return &OpenStackError{
		StatusCode: http.StatusForbidden,
		Code:       "forbidden",
		Message:    "Permission denied",
		Details:    details,
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
