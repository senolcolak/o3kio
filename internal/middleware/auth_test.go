package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/keystone"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const testJWTSecret = "test-secret-for-unit-tests"

// newAuthService returns an AuthService suitable for unit tests.
// It has no DB or cache, so loadRevokedTokens returns silently and
// ValidateToken only performs JWT signature/expiry checks.
func newAuthService() *keystone.AuthService {
	return keystone.NewAuthService(testJWTSecret, 1*time.Hour, nil)
}

// buildToken signs a TokenClaims with the shared test secret.
func buildToken(t *testing.T, claims *keystone.TokenClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

// newAuthRouter wires AuthMiddleware onto a single GET /resource handler that
// echoes back what was set in the gin context as JSON.
func newAuthRouter(svc *keystone.AuthService) *gin.Engine {
	r := gin.New()
	r.Use(AuthMiddleware(svc))
	r.GET("/resource", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id":    c.GetString("user_id"),
			"user_name":  c.GetString("user_name"),
			"project_id": c.GetString("project_id"),
			"is_admin":   c.GetBool("is_admin"),
		})
	})
	// Version discovery paths
	for _, path := range []string{"/v3", "/v2.1", "/v2.0", "/"} {
		p := path
		r.GET(p, func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
	}
	// Token issuance bypass
	r.POST("/v3/auth/tokens", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})
	return r
}

func TestAuthMiddleware_MissingToken_Returns401(t *testing.T) {
	r := newAuthRouter(newAuthService())

	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if _, ok := body["unauthorized"]; !ok {
		t.Errorf("expected 'unauthorized' error envelope, got: %v", body)
	}
}

func TestAuthMiddleware_ValidToken_SetsContext(t *testing.T) {
	svc := newAuthService()
	now := time.Now()
	claims := &keystone.TokenClaims{
		UserID:    "user-123",
		UserName:  "alice",
		ProjectID: "proj-456",
		Roles:     []string{"member"},
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
			Subject:   "user-123",
		},
	}
	token := buildToken(t, claims)

	r := newAuthRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req.Header.Set("X-Auth-Token", token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var body struct {
		UserID    string `json:"user_id"`
		UserName  string `json:"user_name"`
		ProjectID string `json:"project_id"`
		IsAdmin   bool   `json:"is_admin"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"user_id", body.UserID, "user-123"},
		{"user_name", body.UserName, "alice"},
		{"project_id", body.ProjectID, "proj-456"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.field, tt.got, tt.want)
		}
	}
	if body.IsAdmin {
		t.Error("is_admin should be false for a member token")
	}
}

func TestAuthMiddleware_AdminRole_SetsIsAdmin(t *testing.T) {
	svc := newAuthService()
	now := time.Now()
	claims := &keystone.TokenClaims{
		UserID:    "admin-user",
		UserName:  "admin",
		ProjectID: "proj-admin",
		Roles:     []string{"admin", "member"},
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
			Subject:   "admin-user",
		},
	}
	token := buildToken(t, claims)

	r := newAuthRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req.Header.Set("X-Auth-Token", token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body struct {
		IsAdmin bool `json:"is_admin"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body.IsAdmin {
		t.Error("is_admin should be true when roles include 'admin'")
	}
}

func TestAuthMiddleware_VersionDiscovery_NoAuthRequired(t *testing.T) {
	r := newAuthRouter(newAuthService())

	paths := []string{"/v3", "/v2.1", "/v2.0", "/"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			// deliberately no X-Auth-Token
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("path %s: expected 200 without token, got %d", path, w.Code)
			}
		})
	}
}

func TestAuthMiddleware_TokenIssuancePath_NoAuthRequired(t *testing.T) {
	r := newAuthRouter(newAuthService())

	req := httptest.NewRequest(http.MethodPost, "/v3/auth/tokens", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("POST /v3/auth/tokens: expected 201 without token, got %d", w.Code)
	}
}

func TestAuthMiddleware_ExpiredToken_Returns401WithMessage(t *testing.T) {
	svc := newAuthService()
	past := time.Now().Add(-2 * time.Hour)
	claims := &keystone.TokenClaims{
		UserID:    "user-expired",
		UserName:  "ghost",
		ProjectID: "proj-expired",
		Roles:     []string{"member"},
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(past.Add(-1 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(past),
			Subject:   "user-expired",
		},
	}
	token := buildToken(t, claims)

	r := newAuthRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req.Header.Set("X-Auth-Token", token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d", w.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	// ValidateToken maps jwt.ErrTokenExpired → NewUnauthorizedError("Token has expired")
	unauth, ok := body["unauthorized"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'unauthorized' envelope, got: %v", body)
	}
	msg, _ := unauth["message"].(string)
	if msg != "Token has expired" {
		t.Errorf("expected message %q, got %q", "Token has expired", msg)
	}
}

func TestAuthMiddleware_InvalidToken_Returns401(t *testing.T) {
	r := newAuthRouter(newAuthService())

	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req.Header.Set("X-Auth-Token", "not-a-valid-jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for malformed token, got %d", w.Code)
	}
}

func TestAuthMiddleware_WrongSecret_Returns401(t *testing.T) {
	// Sign with a different secret than the service uses.
	now := time.Now()
	claims := &keystone.TokenClaims{
		UserID:    "user-tampered",
		ProjectID: "proj-tampered",
		Roles:     []string{"member"},
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
			Subject:   "user-tampered",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString([]byte("wrong-secret"))

	r := newAuthRouter(newAuthService())
	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req.Header.Set("X-Auth-Token", signed)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong-secret token, got %d", w.Code)
	}
}
