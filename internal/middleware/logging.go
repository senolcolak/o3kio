package middleware

import (
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/cobaltcore-dev/o3k/internal/common"
)

var logger zerolog.Logger

// InitLogger initializes the global logger based on configuration
func InitLogger(config *common.LoggingConfig) {
	// Parse log level
	level, err := zerolog.ParseLevel(config.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Configure output format
	if config.Format == "json" {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	} else {
		// Pretty console output for development
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "2006-01-02 15:04:05",
		}
		logger = zerolog.New(output).With().Timestamp().Logger()
	}

	// Set as global logger
	log.Logger = logger
}

// LoggingMiddleware logs HTTP requests with structured logging
func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate request ID
		requestID := uuid.New().String()
		c.Set("request_id", requestID)

		// Start timer
		start := time.Now()

		// Log incoming request
		logger.Debug().
			Str("request_id", requestID).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Str("query", c.Request.URL.RawQuery).
			Str("remote_addr", c.ClientIP()).
			Str("user_agent", c.Request.UserAgent()).
			Msg("incoming request")

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Determine log level based on status code
		statusCode := c.Writer.Status()
		logEvent := logger.Info()
		if statusCode >= 500 {
			logEvent = logger.Error()
		} else if statusCode >= 400 {
			logEvent = logger.Warn()
		}

		// Log response
		logEvent.
			Str("request_id", requestID).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", statusCode).
			Dur("duration", duration).
			Int("response_size", c.Writer.Size()).
			Msg("request completed")

		// Log errors if any occurred
		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				logger.Error().
					Str("request_id", requestID).
					Uint("type", uint(err.Type)).
					Err(err.Err).
					Msg("request error")
			}
		}
	}
}

// GetLogger returns a logger with request context
func GetLogger(c *gin.Context) *zerolog.Logger {
	requestID, exists := c.Get("request_id")
	if !exists {
		return &logger
	}

	ctxLogger := logger.With().Str("request_id", requestID.(string)).Logger()
	return &ctxLogger
}

// RecoveryMiddleware handles panics
func RecoveryMiddleware() gin.HandlerFunc {
	return gin.RecoveryWithWriter(gin.DefaultWriter)
}

// CORSMiddleware adds CORS headers
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Auth-Token, X-Subject-Token")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func init() {
	// Use JSON binding by default
	binding.EnableDecoderUseNumber = true
}
