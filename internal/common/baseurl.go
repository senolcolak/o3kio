package common

import (
	"fmt"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// trustedHosts holds the parsed list of allowed Host header values.
// Populated from the ALLOWED_HOSTS environment variable (comma-separated).
var trustedHosts []string

func init() {
	if hosts := os.Getenv("ALLOWED_HOSTS"); hosts != "" {
		for host := range strings.SplitSeq(hosts, ",") {
			h := strings.TrimSpace(host)
			if h != "" {
				trustedHosts = append(trustedHosts, h)
			}
		}
	}
}

// BaseURL returns the external base URL for self-links.
func BaseURL(c *gin.Context, defaultPort int) string {
	if host := os.Getenv("O3K_ENDPOINT_HOST"); host != "" {
		return fmt.Sprintf("http://%s:%d", host, defaultPort)
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}

	requestHost := c.Request.Host

	// Validate the Host header against trusted hosts if configured
	if len(trustedHosts) > 0 {
		// Strip port from request host for comparison
		hostOnly := requestHost
		if idx := strings.LastIndex(requestHost, ":"); idx != -1 {
			hostOnly = requestHost[:idx]
		}

		trusted := false
		for _, h := range trustedHosts {
			// Strip port from trusted host entry for comparison
			trustedHostOnly := h
			if idx := strings.LastIndex(h, ":"); idx != -1 {
				trustedHostOnly = h[:idx]
			}
			if strings.EqualFold(hostOnly, trustedHostOnly) {
				trusted = true
				break
			}
		}
		if !trusted {
			// Fall back to the first trusted host
			requestHost = trustedHosts[0]
		}
	}

	return fmt.Sprintf("%s://%s", scheme, requestHost)
}
