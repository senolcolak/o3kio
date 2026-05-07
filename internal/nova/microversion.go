package nova

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const novaMaxVersion = "2.93"

// MicroversionMiddleware sets Nova API microversion response headers on every response.
func MicroversionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Determine requested version from headers
		version := c.GetHeader("X-OpenStack-Nova-API-Version")
		if version == "" {
			// Check the generic OpenStack-API-Version header (format: "compute 2.X")
			generic := c.GetHeader("OpenStack-API-Version")
			if strings.HasPrefix(generic, "compute ") {
				version = strings.TrimPrefix(generic, "compute ")
			}
		}

		// Default to 2.1 if not specified; cap at novaMaxVersion numerically
		if version == "" {
			version = "2.1"
		} else {
			// Parse "2.X" format and compare minor version as integer
			parts := strings.Split(version, ".")
			if len(parts) == 2 {
				if minor, err := strconv.Atoi(parts[1]); err == nil && minor > 93 {
					version = novaMaxVersion
				}
			}
		}

		// Set response headers
		c.Header("X-OpenStack-Nova-API-Version", version)
		c.Header("OpenStack-API-Version", "compute "+version)
		c.Header("Vary", "X-OpenStack-Nova-API-Version")

		c.Next()
	}
}
