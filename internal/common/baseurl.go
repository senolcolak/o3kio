package common

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
)

// BaseURL returns the external base URL for self-links.
func BaseURL(c *gin.Context, defaultPort int) string {
	if host := os.Getenv("O3K_ENDPOINT_HOST"); host != "" {
		return fmt.Sprintf("http://%s:%d", host, defaultPort)
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, c.Request.Host)
}
