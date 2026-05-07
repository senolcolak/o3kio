package middleware

import (
	"net/http"
	"strings"

	"github.com/cobaltcore-dev/o3k/internal/keystone"
	"github.com/gin-gonic/gin"
)

// EnforceAccessRules checks if the request is allowed by the app credential's access rules.
// If no access rules are set (nil), all requests pass through.
// Must run AFTER AuthMiddleware.
func EnforceAccessRules(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		rulesRaw, exists := c.Get("access_rules")
		if !exists || rulesRaw == nil {
			c.Next()
			return
		}

		rules, ok := rulesRaw.([]keystone.AccessRule)
		if !ok || len(rules) == 0 {
			c.Next()
			return
		}

		requestPath := c.Request.URL.Path
		requestMethod := c.Request.Method

		for _, rule := range rules {
			if matchesRule(rule, requestMethod, requestPath, serviceName) {
				c.Next()
				return
			}
		}

		// No rule matched — deny
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"message": "access denied: request not permitted by application credential access rules",
				"code":    http.StatusForbidden,
			},
		})
	}
}

// matchesRule checks if a request matches an access rule.
func matchesRule(rule keystone.AccessRule, method, path, serviceName string) bool {
	// Service must match if specified in rule
	if rule.Service != "" && !strings.EqualFold(rule.Service, serviceName) {
		return false
	}

	// Method must match (case-insensitive)
	if !strings.EqualFold(rule.Method, method) {
		return false
	}

	// Path matching: exact or glob with trailing *
	// "/v2/servers/*" matches "/v2/servers/" and "/v2/servers/abc"
	if strings.HasSuffix(rule.Path, "/*") {
		prefix := strings.TrimSuffix(rule.Path, "*")
		return strings.HasPrefix(path, prefix) || path == strings.TrimSuffix(prefix, "/")
	}
	if strings.HasSuffix(rule.Path, "*") {
		prefix := strings.TrimSuffix(rule.Path, "*")
		return strings.HasPrefix(path, prefix)
	}

	return rule.Path == path
}
