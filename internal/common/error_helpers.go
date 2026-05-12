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

// HandleValidationError sends a bad request error with validation details
func HandleValidationError(c *gin.Context, field string, message string) {
	err := NewValidationError(field, message, "")
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

// AbortWithError sends the error response and aborts the request chain
func AbortWithError(c *gin.Context, err *OpenStackError) {
	SendError(c, err)
	c.Abort()
}
