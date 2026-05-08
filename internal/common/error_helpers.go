package common

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/rs/zerolog/log"
)

// HandleDatabaseError converts database errors to appropriate OpenStack errors
func HandleDatabaseError(c *gin.Context, err error, resource string) {
	if errors.Is(err, database.ErrNoRows) {
		SendError(c, NewNotFoundError(resource))
		return
	}

	// Log the database error
	log.Error().
		Err(err).
		Str("resource", resource).
		Msg("Database error occurred")

	SendError(c, NewDatabaseError("query", resource, err))
}

// HandleDatabaseErrorWithOperation converts database errors with operation context
func HandleDatabaseErrorWithOperation(c *gin.Context, err error, operation, resourceType, resourceID string) {
	if errors.Is(err, database.ErrNoRows) {
		SendError(c, NewResourceNotFoundError(resourceType, resourceID))
		return
	}

	// Log the database error with context
	log.Error().
		Err(err).
		Str("operation", operation).
		Str("resource_type", resourceType).
		Str("resource_id", resourceID).
		Msg("Database error occurred")

	SendError(c, NewDatabaseError(operation, resourceType, err))
}

// HandleValidationError sends a bad request error with validation details
func HandleValidationError(c *gin.Context, field string, message string) {
	err := NewValidationError(field, message, "")
	SendError(c, err)
}

// HandleValidationErrorWithSuggestion sends a validation error with a helpful suggestion
func HandleValidationErrorWithSuggestion(c *gin.Context, field, issue, suggestion string) {
	err := NewValidationError(field, issue, suggestion)
	SendError(c, err)
}

// HandleBindingError handles JSON binding errors
func HandleBindingError(c *gin.Context, err error) {
	osErr := NewBadRequestErrorWithDetails(
		"Invalid request body",
		err.Error(),
	)
	SendError(c, osErr)
}

// AbortWithError sets an error and aborts the request chain
func AbortWithError(c *gin.Context, err *OpenStackError) {
	_ = c.Error(err)
	c.Abort()
	SendError(c, err)
}

// ErrorResponse creates a generic error response helper
type ErrorResponse struct {
	ctx *gin.Context
}

// NewErrorResponse creates a new error response helper
func NewErrorResponse(c *gin.Context) *ErrorResponse {
	return &ErrorResponse{ctx: c}
}

// NotFound sends a 404 error
func (e *ErrorResponse) NotFound(resource string) {
	SendError(e.ctx, NewNotFoundError(resource))
}

// BadRequest sends a 400 error
func (e *ErrorResponse) BadRequest(message string) {
	SendError(e.ctx, NewBadRequestError(message))
}

// Conflict sends a 409 error
func (e *ErrorResponse) Conflict(message string) {
	SendError(e.ctx, NewConflictError(message))
}

// InternalError sends a 500 error
func (e *ErrorResponse) InternalError(message string) {
	SendError(e.ctx, NewInternalServerError(message))
}

// Unauthorized sends a 401 error
func (e *ErrorResponse) Unauthorized(message string) {
	SendError(e.ctx, NewUnauthorizedError(message))
}

// Forbidden sends a 403 error
func (e *ErrorResponse) Forbidden(message string) {
	SendError(e.ctx, NewForbiddenError(message))
}

// QuotaExceeded sends a 413 error
func (e *ErrorResponse) QuotaExceeded(resource string) {
	SendError(e.ctx, NewQuotaExceededError(resource))
}

// ServiceUnavailable sends a 503 error
func (e *ErrorResponse) ServiceUnavailable(message string) {
	SendError(e.ctx, NewServiceUnavailableError(message))
}

// ExternalServiceError sends a service unavailable error for external services
func (e *ErrorResponse) ExternalServiceError(service, operation string, err error) {
	SendError(e.ctx, NewExternalServiceError(service, operation, err))
}

// ResourceNotFound sends a detailed not found error
func (e *ErrorResponse) ResourceNotFound(resourceType, resourceID string) {
	SendError(e.ctx, NewResourceNotFoundError(resourceType, resourceID))
}

// ResourceConflict sends a detailed conflict error
func (e *ErrorResponse) ResourceConflict(resourceType, resourceName, reason string) {
	SendError(e.ctx, NewResourceConflictError(resourceType, resourceName, reason))
}

// ResourceStateError sends a resource state error
func (e *ErrorResponse) ResourceStateError(resourceType, resourceID, currentState, requiredState, operation string) {
	SendError(e.ctx, NewResourceStateError(resourceType, resourceID, currentState, requiredState, operation))
}

// MissingFields sends a missing required fields error
func (e *ErrorResponse) MissingFields(fields ...string) {
	SendError(e.ctx, NewMissingFieldError(fields...))
}

// InvalidValue sends an invalid field value error
func (e *ErrorResponse) InvalidValue(field, value, allowedValues string) {
	SendError(e.ctx, NewInvalidValueError(field, value, allowedValues))
}

// PermissionDenied sends a permission denied error
func (e *ErrorResponse) PermissionDenied(operation, resourceType, requiredRole string) {
	SendError(e.ctx, NewPermissionDeniedError(operation, resourceType, requiredRole))
}
