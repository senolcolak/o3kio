package glance

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/cobaltcore-dev/o3k/internal/database"
)

// setupGlanceTestDB creates an in-memory-equivalent SQLite database with the
// minimum schema needed to exercise CreateImage / GetImage / UpdateImage
// against a real driver. We deliberately avoid the full migration chain so
// the test stays independent of unrelated schema churn.
func setupGlanceTestDB(t *testing.T) *database.SQLiteAdapter {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "glance_test.db")
	adapter, err := database.NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}
	t.Cleanup(adapter.Close)

	ctx := context.Background()
	_, err = adapter.Exec(ctx, `
		CREATE TABLE images (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			project_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			visibility TEXT NOT NULL DEFAULT 'private',
			disk_format TEXT,
			container_format TEXT,
			min_disk_gb INTEGER DEFAULT 0,
			min_ram_mb INTEGER DEFAULT 0,
			protected INTEGER DEFAULT 0,
			rbd_pool TEXT,
			rbd_image TEXT,
			size_bytes INTEGER,
			checksum TEXT,
			os_hash_algo TEXT,
			os_hash_value TEXT,
			properties TEXT DEFAULT '{}',
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`)
	if err != nil {
		t.Fatalf("create images table: %v", err)
	}
	_, err = adapter.Exec(ctx, `
		CREATE TABLE image_members (
			image_id TEXT NOT NULL,
			member_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			PRIMARY KEY(image_id, member_id)
		)`)
	if err != nil {
		t.Fatalf("create image_members table: %v", err)
	}
	return adapter
}

// callHandler runs a Gin handler against an HTTP request and returns the
// response recorder so the test can assert on status + body.
func callHandler(handler gin.HandlerFunc, method, target string, body []byte, projectID, imageID string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Set("project_id", projectID)
	if imageID != "" {
		c.Params = gin.Params{{Key: "id", Value: imageID}}
	}
	handler(c)
	return w
}

// TestSCSImageMetadataRoundTrip exercises the full create → read → update →
// read cycle against a real SQLite driver. This is the integration-level
// guarantee that SCS-0102 properties survive the wire format end to end.
func TestSCSImageMetadataRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupGlanceTestDB(t)
	svc := NewServiceWithDB(db, "stub", "", "", "", "", "", nil)

	// Create with a realistic SCS-0102 property bag.
	createBody := map[string]any{
		"name":              "ubuntu-24.04-scs",
		"disk_format":       "qcow2",
		"container_format":  "bare",
		"visibility":        "private",
		"os_distro":         "ubuntu",
		"os_version":        "24.04",
		"architecture":      "x86_64",
		"hw_disk_bus":       "scsi",
		"replace_frequency": "monthly",
		"provided_until":    "2029-04-30",
		"uuid_validity":     "last-2",
		"image_build_date":  "2026-01-15",
		"os_purpose":        "generic",
		"os_hash_algo":      "sha512",
		"hotfix_hours":      float64(72),
		"license_included":  false,
	}
	body, _ := json.Marshal(createBody)
	w := callHandler(svc.CreateImage, http.MethodPost, "/images", body, "proj-test", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("create: bad json: %v", err)
	}
	imageID, _ := created["id"].(string)
	if imageID == "" {
		t.Fatalf("create: missing id in response: %s", w.Body.String())
	}
	// SCS properties surface as top-level fields, not nested under properties.
	if got := created["os_distro"]; got != "ubuntu" {
		t.Errorf("create response: os_distro = %v, want ubuntu", got)
	}
	if got := created["replace_frequency"]; got != "monthly" {
		t.Errorf("create response: replace_frequency = %v, want monthly", got)
	}

	// GetImage should round-trip the bag from the DB.
	w = callHandler(svc.GetImage, http.MethodGet, "/images/"+imageID, nil, "proj-test", imageID)
	if w.Code != http.StatusOK {
		t.Fatalf("get: status = %d, body = %s", w.Code, w.Body.String())
	}
	var fetched map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("get: bad json: %v", err)
	}
	for _, key := range []string{"os_distro", "os_version", "architecture", "hw_disk_bus", "replace_frequency", "provided_until", "uuid_validity"} {
		if _, ok := fetched[key]; !ok {
			t.Errorf("get: missing top-level key %q in response: %s", key, w.Body.String())
		}
	}
	if got := fetched["os_distro"]; got != "ubuntu" {
		t.Errorf("get: os_distro = %v, want ubuntu", got)
	}

	// PATCH: replace os_version, add patchlevel, remove os_hash_algo.
	patch := []map[string]any{
		{"op": "replace", "path": "/os_version", "value": "24.04.1"},
		{"op": "add", "path": "/patchlevel", "value": "5"},
		{"op": "remove", "path": "/hotfix_hours"},
	}
	patchBody, _ := json.Marshal(patch)
	w = callHandler(svc.UpdateImage, http.MethodPatch, "/images/"+imageID, patchBody, "proj-test", imageID)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: status = %d, body = %s", w.Code, w.Body.String())
	}
	var patched map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &patched); err != nil {
		t.Fatalf("patch: bad json: %v", err)
	}
	if got := patched["os_version"]; got != "24.04.1" {
		t.Errorf("patch: os_version = %v, want 24.04.1", got)
	}
	if got := patched["patchlevel"]; got != "5" {
		t.Errorf("patch: patchlevel = %v, want 5", got)
	}
	if _, present := patched["hotfix_hours"]; present {
		t.Errorf("patch: hotfix_hours should have been removed, got %v", patched["hotfix_hours"])
	}
	// Untouched keys remain.
	if got := patched["os_distro"]; got != "ubuntu" {
		t.Errorf("patch: os_distro = %v, want ubuntu (untouched)", got)
	}
}

// TestSCSImageMetadataValidationRejected verifies that a create request with
// an out-of-vocabulary SCS value is rejected with 400.
func TestSCSImageMetadataValidationRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupGlanceTestDB(t)
	svc := NewServiceWithDB(db, "stub", "", "", "", "", "", nil)

	body, _ := json.Marshal(map[string]any{
		"name":              "bad-image",
		"disk_format":       "qcow2",
		"container_format":  "bare",
		"replace_frequency": "annually", // not in the closed enum
	})
	w := callHandler(svc.CreateImage, http.MethodPost, "/images", body, "proj-test", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid SCS value, got %d, body = %s", w.Code, w.Body.String())
	}
}

// TestSCSImageMetadataPatchValidationRejected verifies that a PATCH that
// would leave the bag in an invalid state is rejected.
func TestSCSImageMetadataPatchValidationRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupGlanceTestDB(t)
	svc := NewServiceWithDB(db, "stub", "", "", "", "", "", nil)

	// First create a valid image.
	body, _ := json.Marshal(map[string]any{
		"name":             "good-image",
		"disk_format":      "qcow2",
		"container_format": "bare",
	})
	w := callHandler(svc.CreateImage, http.MethodPost, "/images", body, "proj-test", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	imageID, _ := created["id"].(string)

	// Patch with an invalid value.
	patch := []map[string]any{
		{"op": "add", "path": "/os_purpose", "value": "database"}, // not in enum
	}
	patchBody, _ := json.Marshal(patch)
	w = callHandler(svc.UpdateImage, http.MethodPatch, "/images/"+imageID, patchBody, "proj-test", imageID)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid os_purpose, got %d, body = %s", w.Code, w.Body.String())
	}

	// License-pair cross-field rule.
	patch = []map[string]any{
		{"op": "add", "path": "/license_included", "value": true},
		{"op": "add", "path": "/license_required", "value": true},
	}
	patchBody, _ = json.Marshal(patch)
	w = callHandler(svc.UpdateImage, http.MethodPatch, "/images/"+imageID, patchBody, "proj-test", imageID)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for license-pair conflict, got %d, body = %s", w.Code, w.Body.String())
	}
}
