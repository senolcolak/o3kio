package compat_test

import (
	"net/http"
	"net/http/httptest"
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
