package cinder

import (
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/gin-gonic/gin"
)

// newFakeGinContext creates a minimal *gin.Context with the given query params.
func newFakeGinContext(params map[string]string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	c.Request, _ = http.NewRequest(http.MethodGet, "/?"+q.Encode(), nil)
	return c
}
