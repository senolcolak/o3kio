package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestRouter(allowedOrigins []string) *gin.Engine {
	r := gin.New()
	r.Use(CORSMiddlewareWithConfig(allowedOrigins))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.OPTIONS("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

// TestCORSMiddleware_DefaultOrigin verifies that an empty config falls back to
// "http://localhost" when the request carries that Origin.
func TestCORSMiddleware_DefaultOrigin(t *testing.T) {
	r := newTestRouter(nil) // nil → should default to ["http://localhost"]

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	got := w.Header().Get("Access-Control-Allow-Origin")
	if got != "http://localhost" {
		t.Errorf("expected Access-Control-Allow-Origin = %q, got %q", "http://localhost", got)
	}
}

// TestCORSMiddleware_ConfiguredOrigin verifies that a configured origin is
// reflected back in the response header.
func TestCORSMiddleware_ConfiguredOrigin(t *testing.T) {
	allowed := []string{"https://example.com", "http://localhost:3000"}
	r := newTestRouter(allowed)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	got := w.Header().Get("Access-Control-Allow-Origin")
	if got != "https://example.com" {
		t.Errorf("expected Access-Control-Allow-Origin = %q, got %q", "https://example.com", got)
	}
}

// TestCORSMiddleware_DisallowedOrigin verifies that an origin not in the allow
// list does not receive an Access-Control-Allow-Origin header.
func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	allowed := []string{"https://example.com"}
	r := newTestRouter(allowed)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	got := w.Header().Get("Access-Control-Allow-Origin")
	if got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header for disallowed origin, got %q", got)
	}
}

// TestCORSMiddleware_OptionsPreflightReturns204 verifies that an OPTIONS
// preflight from an allowed origin returns 204.
func TestCORSMiddleware_OptionsPreflightReturns204(t *testing.T) {
	allowed := []string{"https://example.com"}
	r := newTestRouter(allowed)

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}
}
