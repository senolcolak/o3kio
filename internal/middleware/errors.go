package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/cobaltcore-dev/o3k/internal/common"
)

// ErrorHandlingMiddleware catches panics and converts them to proper error responses
func ErrorHandlingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic
				logger.Error().
					Interface("error", err).
					Str("request_id", getRequestID(c)).
					Str("path", c.Request.URL.Path).
					Msg("Panic recovered in error handling middleware")

				// Send internal server error
				osErr := common.NewInternalServerError(fmt.Sprintf("Internal server error: %v", err))
				common.SendError(c, osErr)
			}
		}()

		c.Next()

		// Check if there are any errors set by handlers
		if len(c.Errors) > 0 {
			// Get the last error
			lastErr := c.Errors.Last()

			// If it's already an OpenStack error, use it
			if osErr, ok := lastErr.Err.(*common.OpenStackError); ok {
				common.SendError(c, osErr)
				return
			}

			// Otherwise, wrap it as internal server error
			osErr := common.NewInternalServerError(lastErr.Error())
			common.SendError(c, osErr)
		}
	}
}

// getRequestID extracts request ID from context
func getRequestID(c *gin.Context) string {
	if reqID, exists := c.Get("request_id"); exists {
		if s, ok := reqID.(string); ok {
			return s
		}
	}
	return "unknown"
}

// NotFoundHandler returns a 404 error for undefined routes
func NotFoundHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		osErr := common.NewNotFoundError("Resource")
		common.SendError(c, osErr)
	}
}

// MethodNotAllowedHandler returns a 405 error for wrong HTTP methods
func MethodNotAllowedHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		osErr := common.NewMethodNotAllowedError(c.Request.Method)
		common.SendError(c, osErr)
	}
}
