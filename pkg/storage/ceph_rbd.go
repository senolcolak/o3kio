//go:build ceph
// +build ceph

package storage

import (
	"context"
	"fmt"

	"github.com/ceph/go-ceph/rados"
	"github.com/ceph/go-ceph/rbd"
)

// rbdIOCtx asserts c.ioctx back to *rados.IOContext. The struct field is
// declared as interface{} so the package builds without the `ceph` build tag;
// every real-mode method goes through this helper.
func (c *CephClient) rbdIOCtx() (*rados.IOContext, error) {
	if c.ioctx == nil {
		return nil, fmt.Errorf("Ceph connection not initialized")
	}
	ioctx, ok := c.ioctx.(*rados.IOContext)
	if !ok || ioctx == nil {
		return nil, fmt.Errorf("Ceph IO context has unexpected type")
	}
	return ioctx, nil
}

func (c *CephClient) rbdConn() (*rados.Conn, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("Ceph connection not initialized")
	}
	conn, ok := c.conn.(*rados.Conn)
	if !ok || conn == nil {
		return nil, fmt.Errorf("Ceph connection has unexpected type")
	}
	return conn, nil
}

// initCephConnection initializes connection to Ceph cluster (build tag: ceph)
func (c *CephClient) initCephConnection() error {
	conn, err := rados.NewConn()
	if err != nil {
		return fmt.Errorf("failed to create RADOS connection: %w", err)
	}

	if err := conn.ReadConfigFile(c.confFile); err != nil {
		conn.Shutdown()
		return fmt.Errorf("failed to read Ceph config file %s: %w", c.confFile, err)
	}

	if err := conn.Connect(); err != nil {
		conn.Shutdown()
		return fmt.Errorf("failed to connect to Ceph cluster: %w", err)
	}

	ioctx, err := conn.OpenIOContext(c.pool)
	if err != nil {
		conn.Shutdown()
		return fmt.Errorf("failed to open pool %s: %w", c.pool, err)
	}

	c.conn = conn
	c.ioctx = ioctx
	return nil
}

// Close closes Ceph connection (build tag: ceph)
func (c *CephClient) Close() error {
	if ioctx, ok := c.ioctx.(*rados.IOContext); ok && ioctx != nil {
		ioctx.Destroy()
		c.ioctx = nil
	}
	if conn, ok := c.conn.(*rados.Conn); ok && conn != nil {
		conn.Shutdown()
		c.conn = nil
	}
	return nil
}

// createVolumeRBD creates an RBD volume (build tag: ceph)
func (c *CephClient) createVolumeRBD(ctx context.Context, volumeID string, sizeGB int) error {
	ioctx, err := c.rbdIOCtx()
	if err != nil {
		return err
	}

	imageName := "volume-" + volumeID
	sizeBytes := uint64(sizeGB) * 1024 * 1024 * 1024

	// Create2: ioctx, name, size, features (uint64), order (int). order=0 = librbd default.
	if _, err := rbd.Create2(ioctx, imageName, sizeBytes, rbd.RbdFeatureLayering, 0); err != nil {
		return fmt.Errorf("failed to create RBD image %s: %w", imageName, err)
	}
	return nil
}

// deleteVolumeRBD deletes an actual RBD volume (build tag: ceph)
func (c *CephClient) deleteVolumeRBD(ctx context.Context, volumeID string) error {
	ioctx, err := c.rbdIOCtx()
	if err != nil {
		return err
	}

	imageName := "volume-" + volumeID
	if err := rbd.RemoveImage(ioctx, imageName); err != nil {
		return fmt.Errorf("failed to remove RBD image %s: %w", imageName, err)
	}
	return nil
}

// volumeExistsRBD checks if an actual RBD volume exists (build tag: ceph)
func (c *CephClient) volumeExistsRBD(ctx context.Context, volumeID string) (bool, error) {
	ioctx, err := c.rbdIOCtx()
	if err != nil {
		return false, err
	}

	imageName := "volume-" + volumeID
	imageNames, err := rbd.GetImageNames(ioctx)
	if err != nil {
		return false, fmt.Errorf("failed to list RBD images: %w", err)
	}
	for _, name := range imageNames {
		if name == imageName {
			return true, nil
		}
	}
	return false, nil
}

// CreateSnapshotRBD creates a snapshot of a volume (build tag: ceph)
func (c *CephClient) CreateSnapshotRBD(ctx context.Context, volumeID, snapshotID string) error {
	ioctx, err := c.rbdIOCtx()
	if err != nil {
		// Stub behavior for non-RBD modes — preserve old contract
		if c.ioctx == nil {
			return nil
		}
		return err
	}

	imageName := "volume-" + volumeID
	snapName := "snap-" + snapshotID

	image, err := rbd.OpenImage(ioctx, imageName, "")
	if err != nil {
		return fmt.Errorf("failed to open RBD image %s: %w", imageName, err)
	}
	defer image.Close()

	if _, err := image.CreateSnapshot(snapName); err != nil {
		return fmt.Errorf("failed to create snapshot %s: %w", snapName, err)
	}
	return nil
}

// DeleteSnapshotRBD deletes a snapshot (build tag: ceph)
func (c *CephClient) DeleteSnapshotRBD(ctx context.Context, volumeID, snapshotID string) error {
	ioctx, err := c.rbdIOCtx()
	if err != nil {
		if c.ioctx == nil {
			return nil
		}
		return err
	}

	imageName := "volume-" + volumeID
	snapName := "snap-" + snapshotID

	image, err := rbd.OpenImage(ioctx, imageName, "")
	if err != nil {
		return fmt.Errorf("failed to open RBD image %s: %w", imageName, err)
	}
	defer image.Close()

	snapshot := image.GetSnapshot(snapName)
	if err := snapshot.Remove(); err != nil {
		return fmt.Errorf("failed to remove snapshot %s: %w", snapName, err)
	}
	return nil
}

// GetVolumeSizeRBD gets the size of a volume (build tag: ceph)
func (c *CephClient) GetVolumeSizeRBD(ctx context.Context, volumeID string) (int, error) {
	ioctx, err := c.rbdIOCtx()
	if err != nil {
		if c.ioctx == nil {
			return 0, nil
		}
		return 0, err
	}

	imageName := "volume-" + volumeID
	image, err := rbd.OpenImage(ioctx, imageName, "")
	if err != nil {
		return 0, fmt.Errorf("failed to open RBD image %s: %w", imageName, err)
	}
	defer image.Close()

	size, err := image.GetSize()
	if err != nil {
		return 0, fmt.Errorf("failed to get image size: %w", err)
	}
	return int(size / (1024 * 1024 * 1024)), nil
}

// HealthRBD checks if Ceph cluster is accessible (build tag: ceph)
func (c *CephClient) HealthRBD(ctx context.Context) error {
	conn, err := c.rbdConn()
	if err != nil {
		return err
	}
	if _, err := conn.PingMonitor(""); err != nil {
		return fmt.Errorf("Ceph cluster health check failed: %w", err)
	}
	return nil
}
