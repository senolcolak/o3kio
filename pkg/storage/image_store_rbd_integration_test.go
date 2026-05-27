//go:build ceph && rbd_integration
// +build ceph,rbd_integration

// RBD integration test against a real Ceph cluster. Requires:
//   - go-ceph build tag `ceph`
//   - additional tag `rbd_integration` to opt in
//   - librbd / librados system libraries
//   - reachable Ceph cluster with admin keyring
//
// Environment:
//   O3K_TEST_CEPH_POOL  pool name (default: rbd)
//   O3K_TEST_CEPH_CONF  ceph.conf path (default: /etc/ceph/ceph.conf)
//
// Run with (microceph in CI):
//   go test -tags 'ceph rbd_integration' ./pkg/storage/ -run TestRBD -v
//
// The test writes a small known-content image, downloads it back, and verifies
// the round-trip is byte-identical, then deletes idempotently.

package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func rbdTestEnv(t *testing.T) (pool, conf string) {
	t.Helper()
	pool = os.Getenv("O3K_TEST_CEPH_POOL")
	if pool == "" {
		pool = "rbd"
	}
	conf = os.Getenv("O3K_TEST_CEPH_CONF")
	if conf == "" {
		conf = "/etc/ceph/ceph.conf"
	}
	if _, err := os.Stat(conf); err != nil {
		t.Skipf("Ceph config %s not readable (%v); skipping integration test", conf, err)
	}
	return pool, conf
}

func newRBDStore(t *testing.T) *ImageStore {
	t.Helper()
	pool, conf := rbdTestEnv(t)
	store := NewImageStore("rbd", pool, conf, "", "", "")
	if store.cephIoctx == nil {
		t.Skipf("Ceph connection failed (pool=%s conf=%s); skipping. Check that the cluster is up and the keyring is readable.", pool, conf)
	}
	return store
}

func TestRBDIntegration_RoundTrip(t *testing.T) {
	store := newRBDStore(t)
	t.Cleanup(func() {
		_ = store.Close()
	})

	imageID := uuid.NewString()
	payload := make([]byte, 1024*1024) // 1 MiB
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Upload
	written, err := store.UploadImage(ctx, imageID, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("UploadImage: %v", err)
	}
	if written != int64(len(payload)) {
		t.Fatalf("UploadImage wrote %d bytes, want %d", written, len(payload))
	}

	t.Cleanup(func() {
		// Best-effort cleanup so the pool doesn't accumulate stale images
		// when an assertion fails partway through.
		_ = store.DeleteImage(context.Background(), imageID)
	})

	// Exists
	exists, err := store.ImageExists(ctx, imageID)
	if err != nil {
		t.Fatalf("ImageExists: %v", err)
	}
	if !exists {
		t.Fatal("ImageExists returned false immediately after upload")
	}

	// Size
	size, err := store.GetImageSize(ctx, imageID)
	if err != nil {
		t.Fatalf("GetImageSize: %v", err)
	}
	if size != int64(len(payload)) {
		t.Errorf("GetImageSize = %d, want %d", size, len(payload))
	}

	// Download + compare
	var buf bytes.Buffer
	if err := store.DownloadImage(ctx, imageID, &buf); err != nil {
		t.Fatalf("DownloadImage: %v", err)
	}
	got := buf.Bytes()
	// RBD reads return the full image extent; the trailing bytes past
	// our written length should be zero, but our payload covers the
	// whole image so the lengths should match exactly.
	if len(got) < len(payload) {
		t.Fatalf("DownloadImage returned %d bytes, want at least %d", len(got), len(payload))
	}
	if !bytes.Equal(got[:len(payload)], payload) {
		t.Fatal("DownloadImage payload differs from upload (first len(payload) bytes do not match)")
	}

	// Delete
	if err := store.DeleteImage(ctx, imageID); err != nil {
		t.Fatalf("DeleteImage: %v", err)
	}

	// Delete is idempotent
	if err := store.DeleteImage(ctx, imageID); err != nil {
		t.Errorf("DeleteImage second call (idempotent) returned: %v", err)
	}

	// Gone
	exists, err = store.ImageExists(ctx, imageID)
	if err != nil {
		t.Fatalf("ImageExists post-delete: %v", err)
	}
	if exists {
		t.Error("ImageExists returned true after DeleteImage")
	}
}

func TestRBDIntegration_DownloadMissingIsError(t *testing.T) {
	store := newRBDStore(t)
	t.Cleanup(func() {
		_ = store.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var buf bytes.Buffer
	err := store.DownloadImage(ctx, "does-not-exist-"+uuid.NewString(), &buf)
	if err == nil {
		t.Fatal("DownloadImage of missing image returned nil error, want error")
	}
}
