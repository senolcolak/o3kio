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

// httpFaultName maps an HTTP status code to the OpenStack fault envelope key.
// See https://docs.openstack.org/api-guide/compute/faults.html
func httpFaultName(code int) string {
	switch code {
	case http.StatusBadRequest:
		return "badRequest"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "itemNotFound"
	case http.StatusMethodNotAllowed:
		return "badMethod"
	case http.StatusConflict:
		return "conflictingRequest"
	case http.StatusRequestEntityTooLarge, http.StatusTooManyRequests:
		return "overLimit"
	case http.StatusInternalServerError:
		return "computeFault"
	case http.StatusNotImplemented:
		return "notImplemented"
	case http.StatusServiceUnavailable:
		return "serviceUnavailable"
	default:
		return "computeFault"
	}
}

// ToJSON converts the error to OpenStack-compatible JSON format using named
// fault keys for every status code, e.g.:
//
//	{"itemNotFound": {"code": 404, "message": "..."}}
//	{"badRequest":   {"code": 400, "message": "..."}}
//
// The Code field on the struct is used as the envelope key when set; otherwise
// httpFaultName derives one from the status code.
func (e *OpenStackError) ToJSON() gin.H {
	return e.toJSONWithRequestID("")
}

func (e *OpenStackError) toJSONWithRequestID(requestID string) gin.H {
	errorBody := gin.H{
		"message": e.Message,
		"code":    e.StatusCode,
		"title":   http.StatusText(e.StatusCode),
	}

	if e.Details != "" {
		errorBody["details"] = e.Details
	}

	if requestID != "" {
		errorBody["request_id"] = requestID
	}

	key := e.Code
	if key == "" {
		key = httpFaultName(e.StatusCode)
	}

	return gin.H{key: errorBody}
}

// SendError sends an OpenStack-formatted error response. It reads the
// request_id set by the logging middleware and includes it in the body.
// For 429 and 503 responses a Retry-After: 60 header is added automatically.
func SendError(c *gin.Context, err *OpenStackError) {
	if err.StatusCode == http.StatusTooManyRequests || err.StatusCode == http.StatusServiceUnavailable {
		c.Header("Retry-After", "60")
	}
	requestID := c.GetString("request_id")
	c.JSON(err.StatusCode, err.toJSONWithRequestID(requestID))
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
		Message:    fmt.Sprintf("database error during %s operation on %s", operation, resourceType),
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

func NewNotImplementedError(message string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusNotImplemented,
		Code:       "notImplemented",
		Message:    message,
	}
}

func NewMethodNotAllowedError(method string) *OpenStackError {
	return &OpenStackError{
		StatusCode: http.StatusMethodNotAllowed,
		Code:       "badMethod",
		Message:    fmt.Sprintf("The method %s is not allowed for this resource.", method),
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

