package compat_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/compat"
	"github.com/stretchr/testify/assert"
)

func TestEmbeddedRouterKeystoneVersions(t *testing.T) {
	router, cleanup := compat.NewEmbeddedRouter()
	defer cleanup()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v3", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "version")
}

func TestEmbeddedRouterNovaFlavors(t *testing.T) {
	router, cleanup := compat.NewEmbeddedRouter()
	defer cleanup()

	// Without a valid token, auth middleware should return 401.
	// This proves the route is wired and the auth middleware is active.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v2.1/flavors", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestEmbeddedServerRecordsAPICalls(t *testing.T) {
	ctx := context.Background()
	srv, err := compat.StartEmbeddedServer(ctx)
	assert.NoError(t, err)
	defer srv.Shutdown(ctx)

	resp, err := http.Get(fmt.Sprintf("http://%s/v3", srv.Addr()))
	assert.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	results := srv.Recorder.Results()
	assert.GreaterOrEqual(t, len(results), 1)
	assert.Equal(t, "GET", results[0].Method)
}

func TestEmbeddedRouterTokenIssuance(t *testing.T) {
	router, cleanup := compat.NewEmbeddedRouter()
	defer cleanup()

	w := httptest.NewRecorder()
	tokenBody := `{"auth":{"identity":{"methods":["password"],"password":{"user":{"name":"admin","domain":{"id":"default"},"password":"secret"}}},"scope":{"project":{"name":"default","domain":{"id":"default"}}}}}`
	req, _ := http.NewRequest("POST", "/v3/auth/tokens", strings.NewReader(tokenBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, "token issuance failed: %s", w.Body.String())
	token := w.Header().Get("X-Subject-Token")
	assert.NotEmpty(t, token, "should return a JWT token")

	// Use token for authenticated endpoint
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/v2.1/flavors", nil)
	req2.Header.Set("X-Auth-Token", token)
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code, "authenticated request failed: %s", w2.Body.String())
	assert.Contains(t, w2.Body.String(), "flavors")
}
