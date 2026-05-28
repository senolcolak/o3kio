package glance

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestCreateImage_SCS0104_RejectsBadSource: an image whose name matches an
// SCS-0104 manifest entry but whose image_source doesn't start with any
// declared prefix must be rejected with 400. This is the conformance gate —
// operators can't accidentally claim a known SCS name with a wrong source.
func TestCreateImage_SCS0104_RejectsBadSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &Service{mode: "stub"}
	r := gin.New()
	r.POST("/images", func(c *gin.Context) {
		c.Set("project_id", "proj-test")
		svc.CreateImage(c)
	})

	body, _ := json.Marshal(map[string]interface{}{
		"name":         "Ubuntu 22.04",
		"image_source": "https://example.org/my-cooked-ubuntu.img",
	})
	req := httptest.NewRequest(http.MethodPost, "/images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for SCS-known name with bad source, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateImage_SCS0104_AllowsUnknownName: an image whose name doesn't appear
// in the SCS-0104 manifest passes the validator unconditionally. Operators
// remain free to publish arbitrary images. (We can't fully exercise the
// handler — activeDB() is nil in this minimal fixture and will panic at the
// INSERT — so we recover from that panic and assert the gate didn't fire
// before reaching the DB call.)
func TestCreateImage_SCS0104_AllowsUnknownName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &Service{mode: "stub"}
	r := gin.New()
	r.POST("/images", func(c *gin.Context) {
		c.Set("project_id", "proj-test")
		defer func() {
			// Swallow the nil-DB panic from the INSERT path; the only
			// thing this test cares about is whether the SCS gate fired
			// before that, which would set 400 on the response.
			_ = recover()
		}()
		svc.CreateImage(c)
	})

	body, _ := json.Marshal(map[string]interface{}{
		"name":         "my-custom-image",
		"image_source": "https://example.org/whatever.img",
	})
	req := httptest.NewRequest(http.MethodPost, "/images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest {
		t.Errorf("expected unknown image name to bypass SCS gate, got 400: %s", w.Body.String())
	}
}

// TestCreateImage_SCS0104_AcceptsKnownNameWithGoodSource: Ubuntu 22.04 + a
// jammy-prefixed source must pass the SCS gate. Same caveat as above: we
// recover from the nil-DB panic past the validator.
func TestCreateImage_SCS0104_AcceptsKnownNameWithGoodSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &Service{mode: "stub"}
	r := gin.New()
	r.POST("/images", func(c *gin.Context) {
		c.Set("project_id", "proj-test")
		defer func() { _ = recover() }()
		svc.CreateImage(c)
	})

	body, _ := json.Marshal(map[string]interface{}{
		"name":         "Ubuntu 22.04",
		"image_source": "https://cloud-images.ubuntu.com/releases/jammy/jammy-server-cloudimg-amd64.img",
	})
	req := httptest.NewRequest(http.MethodPost, "/images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest {
		t.Errorf("expected SCS-known name with valid source to pass gate, got 400: %s", w.Body.String())
	}
}
