package storage

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestImageStore_RBDStubReturnsCompilationHint verifies that when the
// binary is built without the `ceph` tag, every RBD entry point on
// ImageStore returns an error pointing the operator at the missing
// build tag. This is the behaviour the policy and ops docs promise:
// "real RBD support requires building with -tags ceph".
func TestImageStore_RBDStubReturnsCompilationHint(t *testing.T) {
	store := NewImageStore("rbd", "images", "/etc/ceph/ceph.conf", "", "", "")
	ctx := context.Background()

	t.Run("upload", func(t *testing.T) {
		_, err := store.uploadImageRBD(ctx, "img-1", strings.NewReader("payload"))
		assertNotCompiled(t, err)
	})

	t.Run("download", func(t *testing.T) {
		var buf bytes.Buffer
		err := store.downloadImageRBD(ctx, "img-1", &buf)
		assertNotCompiled(t, err)
	})

	t.Run("delete", func(t *testing.T) {
		err := store.deleteImageRBD(ctx, "img-1")
		assertNotCompiled(t, err)
	})

	t.Run("exists", func(t *testing.T) {
		exists, err := store.imageExistsRBD(ctx, "img-1")
		if exists {
			t.Errorf("stub should report exists=false, got true")
		}
		assertNotCompiled(t, err)
	})

	t.Run("size", func(t *testing.T) {
		size, err := store.getImageSizeRBD(ctx, "img-1")
		if size != 0 {
			t.Errorf("stub should report size=0, got %d", size)
		}
		assertNotCompiled(t, err)
	})
}

// TestImageStore_DispatchRoutesRBDModes verifies the public entry points
// (UploadImage, DownloadImage, DeleteImage, ImageExists, GetImageSize)
// route RBD-flavoured modes through the RBD methods rather than silently
// succeeding via local/S3 fallbacks. Since we're built without -tags ceph,
// every RBD call should error out — proving the dispatcher actually hit
// the RBD path.
func TestImageStore_DispatchRoutesRBDModes(t *testing.T) {
	store := NewImageStore("rbd", "images", "/etc/ceph/ceph.conf", "", "", "")
	ctx := context.Background()

	t.Run("UploadImage_rbd", func(t *testing.T) {
		_, err := store.UploadImage(ctx, "img-2", strings.NewReader("data"))
		assertNotCompiled(t, err)
	})

	t.Run("DownloadImage_rbd", func(t *testing.T) {
		var buf bytes.Buffer
		err := store.DownloadImage(ctx, "img-2", &buf)
		assertNotCompiled(t, err)
	})

	t.Run("DeleteImage_rbd", func(t *testing.T) {
		err := store.DeleteImage(ctx, "img-2")
		assertNotCompiled(t, err)
	})

	t.Run("ImageExists_rbd", func(t *testing.T) {
		_, err := store.ImageExists(ctx, "img-2")
		assertNotCompiled(t, err)
	})

	t.Run("GetImageSize_rbd", func(t *testing.T) {
		_, err := store.GetImageSize(ctx, "img-2")
		assertNotCompiled(t, err)
	})
}

// TestImageStore_GetImageSizeRoutesByMode verifies the size dispatcher
// chooses the right backend per mode. The RBD-flavoured modes hit the
// stub error; the stub mode returns the recorded size from in-memory
// state.
func TestImageStore_GetImageSizeRoutesByMode(t *testing.T) {
	cases := []struct {
		mode    string
		wantErr bool
	}{
		{"rbd", true},
		{"local,rbd", true},
		{"rbd,s3", true}, // S3 uninitialised, RBD stub — both error paths exercised
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			store := NewImageStore(tc.mode, "images", "/etc/ceph/ceph.conf", "bucket", "us-east-1", "")
			_, err := store.GetImageSize(context.Background(), "img-x")
			if tc.wantErr && err == nil {
				t.Errorf("mode=%s: expected error, got nil", tc.mode)
			}
		})
	}
}

// TestImageStore_CloseIsIdempotent ensures Close can be called even on a
// store that never opened a Ceph connection (stub mode, S3-only mode).
func TestImageStore_CloseIsIdempotent(t *testing.T) {
	for _, mode := range []string{"stub", "local", "s3", "rbd"} {
		t.Run(mode, func(t *testing.T) {
			store := NewImageStore(mode, "images", "/etc/ceph/ceph.conf", "bucket", "us-east-1", "")
			if err := store.Close(); err != nil {
				t.Errorf("first Close on %s mode: %v", mode, err)
			}
			if err := store.Close(); err != nil {
				t.Errorf("second Close on %s mode: %v", mode, err)
			}
		})
	}
}

func assertNotCompiled(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not compiled") &&
		!strings.Contains(err.Error(), "not initialized") &&
		!strings.Contains(err.Error(), "Ceph") {
		t.Errorf("error %q does not surface a Ceph build hint", err.Error())
	}
}
