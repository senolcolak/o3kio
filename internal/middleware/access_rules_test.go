package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/keystone"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestEnforceAccessRules_NoRulesSet(t *testing.T) {
	r := gin.New()
	r.Use(EnforceAccessRules("compute"))
	r.GET("/v2.1/servers", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v2.1/servers", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d (no rules = pass through)", w.Code, http.StatusOK)
	}
}

func TestEnforceAccessRules_NilRules(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("access_rules", nil)
		c.Next()
	})
	r.Use(EnforceAccessRules("compute"))
	r.GET("/v2.1/servers", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v2.1/servers", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d (nil rules = pass through)", w.Code, http.StatusOK)
	}
}

func TestEnforceAccessRules_EmptyRules(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("access_rules", []keystone.AccessRule{})
		c.Next()
	})
	r.Use(EnforceAccessRules("compute"))
	r.GET("/v2.1/servers", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v2.1/servers", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d (empty rules = pass through)", w.Code, http.StatusOK)
	}
}

func TestEnforceAccessRules_MatchingRule(t *testing.T) {
	rules := []keystone.AccessRule{
		{Method: "GET", Path: "/v2.1/servers", Service: "compute"},
	}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("access_rules", rules)
		c.Next()
	})
	r.Use(EnforceAccessRules("compute"))
	r.GET("/v2.1/servers", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v2.1/servers", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got %d, want %d (matching rule = allow)", w.Code, http.StatusOK)
	}
}

func TestEnforceAccessRules_NoMatchingRule(t *testing.T) {
	rules := []keystone.AccessRule{
		{Method: "GET", Path: "/v2.1/servers", Service: "compute"},
	}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("access_rules", rules)
		c.Next()
	})
	r.Use(EnforceAccessRules("compute"))
	r.POST("/v2.1/servers", func(c *gin.Context) { c.Status(http.StatusCreated) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v2.1/servers", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("got %d, want %d (no matching rule = deny)", w.Code, http.StatusForbidden)
	}
}

func TestMatchesRule(t *testing.T) {
	tests := []struct {
		name        string
		rule        keystone.AccessRule
		method      string
		path        string
		serviceName string
		want        bool
	}{
		{
			name:        "exact match",
			rule:        keystone.AccessRule{Method: "GET", Path: "/v2.1/servers"},
			method:      "GET", path: "/v2.1/servers",
			serviceName: "",
			want:        true,
		},
		{
			name:        "method mismatch",
			rule:        keystone.AccessRule{Method: "GET", Path: "/v2.1/servers"},
			method:      "POST", path: "/v2.1/servers",
			serviceName: "",
			want:        false,
		},
		{
			name:        "method case-insensitive",
			rule:        keystone.AccessRule{Method: "get", Path: "/v2.1/servers"},
			method:      "GET", path: "/v2.1/servers",
			serviceName: "",
			want:        true,
		},
		{
			name:        "path mismatch",
			rule:        keystone.AccessRule{Method: "GET", Path: "/v2.1/servers"},
			method:      "GET", path: "/v2.1/flavors",
			serviceName: "",
			want:        false,
		},
		{
			name:        "glob wildcard matches subpath",
			rule:        keystone.AccessRule{Method: "GET", Path: "/v2.1/servers/*"},
			method:      "GET", path: "/v2.1/servers/abc-123",
			serviceName: "",
			want:        true,
		},
		{
			name:        "glob wildcard matches root",
			rule:        keystone.AccessRule{Method: "GET", Path: "/v2.1/servers/*"},
			method:      "GET", path: "/v2.1/servers",
			serviceName: "",
			want:        true,
		},
		{
			name:        "glob wildcard no trailing slash root",
			rule:        keystone.AccessRule{Method: "DELETE", Path: "/v2.1/servers/*"},
			method:      "DELETE", path: "/v2.1/servers/",
			serviceName: "",
			want:        true,
		},
		{
			name:        "bare wildcard prefix",
			rule:        keystone.AccessRule{Method: "GET", Path: "/v2*"},
			method:      "GET", path: "/v2.1/servers",
			serviceName: "",
			want:        true,
		},
		{
			name:        "bare wildcard no match",
			rule:        keystone.AccessRule{Method: "GET", Path: "/v3*"},
			method:      "GET", path: "/v2.1/servers",
			serviceName: "",
			want:        false,
		},
		{
			name:        "service match",
			rule:        keystone.AccessRule{Method: "GET", Path: "/v2.1/servers", Service: "compute"},
			method:      "GET", path: "/v2.1/servers",
			serviceName: "compute",
			want:        true,
		},
		{
			name:        "service mismatch",
			rule:        keystone.AccessRule{Method: "GET", Path: "/v2.1/servers", Service: "compute"},
			method:      "GET", path: "/v2.1/servers",
			serviceName: "identity",
			want:        false,
		},
		{
			name:        "service case-insensitive",
			rule:        keystone.AccessRule{Method: "GET", Path: "/v2.1/servers", Service: "Compute"},
			method:      "GET", path: "/v2.1/servers",
			serviceName: "compute",
			want:        true,
		},
		{
			name:        "empty rule service matches any service name",
			rule:        keystone.AccessRule{Method: "GET", Path: "/v2.1/servers", Service: ""},
			method:      "GET", path: "/v2.1/servers",
			serviceName: "compute",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesRule(tt.rule, tt.method, tt.path, tt.serviceName)
			if got != tt.want {
				t.Errorf("matchesRule(%+v, %q, %q, %q) = %v, want %v",
					tt.rule, tt.method, tt.path, tt.serviceName, got, tt.want)
			}
		})
	}
}
