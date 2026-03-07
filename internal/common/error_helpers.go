package common

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// HandleDatabaseError converts database errors to appropriate OpenStack errors
func HandleDatabaseError(c *gin.Context, err error, resource string) {
	if err == pgx.ErrNoRows {
		SendError(c, NewNotFoundError(resource))
		return
	}

	// Log the database error
	log.Error().
		Err(err).
		Str("resource", resource).
		Msg("Database error occurred")

	SendError(c, NewInternalServerError("Database error occurred"))
}

// HandleValidationError sends a bad request error with validation details
func HandleValidationError(c *gin.Context, field string, message string) {
	err := NewBadRequestErrorWithDetails(
		"Validation failed",
		fmt.Sprintf("Field '%s': %s", field, message),
	)
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
	c.Error(err)
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
