package nova_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/nova"
	"github.com/gin-gonic/gin"
)

// TestCreateFlavor_SCS_Valid is the wire-up integration: a valid SCS-0100
// name parses, the row insert lands, AND the parsed CPU/disk type is
// mirrored into flavor_extra_specs so SCS-aware clients see the same shape
// as the SCS-0103 seed.
func TestCreateFlavor_SCS_Valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := database.NewMockDB()
	svc := nova.NewServiceWithDB(mock, "stub")

	body, _ := json.Marshal(map[string]any{
		"flavor": map[string]any{
			"name":  "SCS-2V-4-20s",
			"ram":   4096,
			"vcpus": 2,
			"disk":  20,
		},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/v2.1/flavors", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc.CreateFlavor(c)

	if w.Code != http.StatusOK {
		t.Fatalf("CreateFlavor: status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if !mock.ExecCalled("INSERT INTO flavors") {
		t.Error("expected INSERT INTO flavors")
	}
	if !mock.ExecCalled("INSERT INTO flavor_extra_specs") {
		t.Error("expected SCS extra-specs to be mirrored into flavor_extra_specs")
	}
}

// TestCreateFlavor_SCS_Malformed: an SCS-* name that doesn't parse must be
// rejected at the API surface with 400, not silently accepted.
func TestCreateFlavor_SCS_Malformed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := database.NewMockDB()
	svc := nova.NewServiceWithDB(mock, "stub")

	body, _ := json.Marshal(map[string]any{
		"flavor": map[string]any{
			"name":  "SCS-bogus",
			"ram":   4096,
			"vcpus": 2,
			"disk":  0,
		},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/v2.1/flavors", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc.CreateFlavor(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("CreateFlavor(SCS-bogus): status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	if mock.ExecCalled("INSERT INTO flavors") {
		t.Error("malformed SCS name should not have inserted a flavor row")
	}
}

// TestCreateFlavor_NonSCS_Passthrough: non-SCS names continue to work
// untouched — the validator is opt-in based on the `SCS-` prefix.
func TestCreateFlavor_NonSCS_Passthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := database.NewMockDB()
	svc := nova.NewServiceWithDB(mock, "stub")

	body, _ := json.Marshal(map[string]any{
		"flavor": map[string]any{
			"name":  "m1.tiny",
			"ram":   512,
			"vcpus": 1,
			"disk":  1,
		},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/v2.1/flavors", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc.CreateFlavor(c)

	if w.Code != http.StatusOK {
		t.Fatalf("CreateFlavor(m1.tiny): status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if !mock.ExecCalled("INSERT INTO flavors") {
		t.Error("expected INSERT INTO flavors for non-SCS name")
	}
	if mock.ExecCalled("INSERT INTO flavor_extra_specs") {
		t.Error("non-SCS name should not produce scs:* extra-specs")
	}
}
