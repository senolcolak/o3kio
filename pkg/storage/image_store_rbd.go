//go:build ceph
// +build ceph

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ceph/go-ceph/rados"
	"github.com/ceph/go-ceph/rbd"
)

// rbdChunkSize controls how much data we stream per Read/Write call against
// an RBD image. 4 MiB matches the default RBD object size and keeps the
// per-call overhead amortised across syscalls.
const rbdChunkSize = 4 * 1024 * 1024

// initCephConnection opens a RADOS connection and IO context for the image
// pool. Mirrors the pattern used by ceph_rbd.go on the volume side.
func (s *ImageStore) initCephConnection() error {
	conn, err := rados.NewConn()
	if err != nil {
		return fmt.Errorf("failed to create RADOS connection: %w", err)
	}

	if err := conn.ReadConfigFile(s.cephConf); err != nil {
		conn.Shutdown()
		return fmt.Errorf("failed to read Ceph config file %s: %w", s.cephConf, err)
	}

	if err := conn.Connect(); err != nil {
		conn.Shutdown()
		return fmt.Errorf("failed to connect to Ceph cluster: %w", err)
	}

	ioctx, err := conn.OpenIOContext(s.cephPool)
	if err != nil {
		conn.Shutdown()
		return fmt.Errorf("failed to open pool %s: %w", s.cephPool, err)
	}

	s.cephConn = conn
	s.cephIoctx = ioctx
	return nil
}

// closeCephConnection releases the RADOS connection and IO context.
func (s *ImageStore) closeCephConnection() error {
	if ioctx, ok := s.cephIoctx.(*rados.IOContext); ok && ioctx != nil {
		ioctx.Destroy()
		s.cephIoctx = nil
	}
	if conn, ok := s.cephConn.(*rados.Conn); ok && conn != nil {
		conn.Shutdown()
		s.cephConn = nil
	}
	return nil
}

// rbdIOCtx returns the typed *rados.IOContext or an error if Ceph isn't
// initialised (e.g. NewImageStore failed to connect).
func (s *ImageStore) rbdIOCtx() (*rados.IOContext, error) {
	ioctx, ok := s.cephIoctx.(*rados.IOContext)
	if !ok || ioctx == nil {
		return nil, fmt.Errorf("Ceph connection not initialized for image store (pool=%s)", s.cephPool)
	}
	return ioctx, nil
}

// uploadImageRBD streams reader into an RBD image. The image is created with
// a generous initial size and resized down to the actual byte count once we
// know the final size — RBD requires the image to exist before Open can
// write to it, and the size can only be known after consuming the reader.
func (s *ImageStore) uploadImageRBD(ctx context.Context, imageID string, reader io.Reader) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	ioctx, err := s.rbdIOCtx()
	if err != nil {
		return 0, err
	}

	imageName := "image-" + imageID

	// Buffer the upload so we know the final size before creating the RBD
	// image. Glance uploads are typically <2 GiB and the alternative
	// (resize-during-write) requires either an upper-bound guess or two
	// passes — both worse than buffering for our typical image sizes.
	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, fmt.Errorf("failed to read image data: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return 0, err
	}

	size := uint64(len(data))
	if _, err := rbd.Create(ioctx, imageName, size, rbd.RbdFeatureLayering); err != nil {
		return 0, fmt.Errorf("failed to create RBD image %s/%s: %w", s.cephPool, imageName, err)
	}

	image, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		// Best-effort cleanup of the half-created image.
		_ = rbd.RemoveImage(ioctx, imageName)
		return 0, fmt.Errorf("failed to open RBD image %s/%s: %w", s.cephPool, imageName, err)
	}
	defer image.Close()

	written, err := image.Write(data)
	if err != nil {
		_ = image.Close()
		_ = rbd.RemoveImage(ioctx, imageName)
		return 0, fmt.Errorf("failed to write RBD image %s/%s: %w", s.cephPool, imageName, err)
	}
	if written != len(data) {
		_ = image.Close()
		_ = rbd.RemoveImage(ioctx, imageName)
		return 0, fmt.Errorf("short write to RBD image %s/%s: wrote %d of %d bytes", s.cephPool, imageName, written, len(data))
	}

	return int64(written), nil
}

// downloadImageRBD streams an RBD image to writer in fixed-size chunks.
func (s *ImageStore) downloadImageRBD(ctx context.Context, imageID string, writer io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	ioctx, err := s.rbdIOCtx()
	if err != nil {
		return err
	}

	imageName := "image-" + imageID

	image, err := rbd.OpenImageReadOnly(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("failed to open RBD image %s/%s: %w", s.cephPool, imageName, err)
	}
	defer image.Close()

	total, err := image.GetSize()
	if err != nil {
		return fmt.Errorf("failed to get RBD image size %s/%s: %w", s.cephPool, imageName, err)
	}

	buf := make([]byte, rbdChunkSize)
	var read uint64
	for read < total {
		if err := ctx.Err(); err != nil {
			return err
		}
		remaining := total - read
		if remaining < uint64(len(buf)) {
			buf = buf[:remaining]
		}
		n, err := image.Read(buf)
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read RBD image %s/%s at offset %d: %w", s.cephPool, imageName, read, err)
		}
		if n == 0 {
			break
		}
		if _, werr := writer.Write(buf[:n]); werr != nil {
			return fmt.Errorf("failed to write image data: %w", werr)
		}
		read += uint64(n)
	}

	return nil
}

// deleteImageRBD removes an RBD image. Returns nil for missing images so
// delete is idempotent (matches the local and S3 backends).
func (s *ImageStore) deleteImageRBD(ctx context.Context, imageID string) error {
	_, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	ioctx, err := s.rbdIOCtx()
	if err != nil {
		return err
	}

	imageName := "image-" + imageID

	if err := rbd.RemoveImage(ioctx, imageName); err != nil {
		// go-ceph wraps ENOENT as rbd.ErrNotFound / rados.ErrNotFound; use
		// errors.Is so a wrapped error still trips idempotency.
		if errors.Is(err, rbd.ErrNotFound) || errors.Is(err, rados.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("failed to remove RBD image %s/%s: %w", s.cephPool, imageName, err)
	}
	return nil
}

// imageExistsRBD checks whether an RBD image is present in the pool.
func (s *ImageStore) imageExistsRBD(ctx context.Context, imageID string) (bool, error) {
	_, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	ioctx, err := s.rbdIOCtx()
	if err != nil {
		return false, err
	}

	imageName := "image-" + imageID

	names, err := rbd.GetImageNames(ioctx)
	if err != nil {
		return false, fmt.Errorf("failed to list RBD images in %s: %w", s.cephPool, err)
	}
	for _, name := range names {
		if name == imageName {
			return true, nil
		}
	}
	return false, nil
}

// getImageSizeRBD reports the size of an RBD image in bytes.
func (s *ImageStore) getImageSizeRBD(ctx context.Context, imageID string) (int64, error) {
	_, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	ioctx, err := s.rbdIOCtx()
	if err != nil {
		return 0, err
	}

	imageName := "image-" + imageID

	image, err := rbd.OpenImageReadOnly(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return 0, fmt.Errorf("failed to open RBD image %s/%s: %w", s.cephPool, imageName, err)
	}
	defer image.Close()

	size, err := image.GetSize()
	if err != nil {
		return 0, fmt.Errorf("failed to get RBD image size %s/%s: %w", s.cephPool, imageName, err)
	}
	return int64(size), nil
}
