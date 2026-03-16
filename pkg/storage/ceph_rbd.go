//go:build ceph
// +build ceph

package storage

import (
	"context"
	"fmt"

	"github.com/ceph/go-ceph/rados"
	"github.com/ceph/go-ceph/rbd"
)

// initCephConnection initializes connection to Ceph cluster (build tag: ceph)
func (c *CephClient) initCephConnection() error {
	conn, err := rados.NewConn()
	if err != nil {
		return fmt.Errorf("failed to create RADOS connection: %w", err)
	}

	// Read config file
	if err := conn.ReadConfigFile(c.confFile); err != nil {
		conn.Shutdown()
		return fmt.Errorf("failed to read Ceph config file %s: %w", c.confFile, err)
	}

	// Connect to cluster
	if err := conn.Connect(); err != nil {
		conn.Shutdown()
		return fmt.Errorf("failed to connect to Ceph cluster: %w", err)
	}

	// Open IO context for pool
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
	if c.ioctx != nil {
		c.ioctx.Destroy()
	}
	if c.conn != nil {
		c.conn.Shutdown()
	}
	return nil
}

// createVolumeRBD creates an RBD volume (build tag: ceph)
func (c *CephClient) createVolumeRBD(ctx context.Context, volumeID string, sizeGB int) error {
	if c.ioctx == nil {
		return fmt.Errorf("Ceph connection not initialized")
	}

	imageName := "volume-" + volumeID
	sizeBytes := uint64(sizeGB) * 1024 * 1024 * 1024

	// Create RBD image
	_, err := rbd.Create(c.ioctx, imageName, sizeBytes, rbd.RbdFeatureLayering)
	if err != nil {
		return fmt.Errorf("failed to create RBD image %s: %w", imageName, err)
	}

	return nil
}

// deleteVolumeRBD deletes an actual RBD volume (build tag: ceph)
func (c *CephClient) deleteVolumeRBD(ctx context.Context, volumeID string) error {
	if c.ioctx == nil {
		return fmt.Errorf("Ceph connection not initialized")
	}

	imageName := "volume-" + volumeID

	// Remove RBD image
	err := rbd.RemoveImage(c.ioctx, imageName)
	if err != nil {
		return fmt.Errorf("failed to remove RBD image %s: %w", imageName, err)
	}

	return nil
}

// volumeExistsRBD checks if an actual RBD volume exists (build tag: ceph)
func (c *CephClient) volumeExistsRBD(ctx context.Context, volumeID string) (bool, error) {
	if c.ioctx == nil {
		return false, fmt.Errorf("Ceph connection not initialized")
	}

	imageName := "volume-" + volumeID

	// List images and check if our image exists
	imageNames, err := rbd.GetImageNames(c.ioctx)
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

// CreateSnapshot creates a snapshot of a volume (build tag: ceph)
func (c *CephClient) CreateSnapshotRBD(ctx context.Context, volumeID, snapshotID string) error {
	if c.ioctx == nil {
		return nil // Stub behavior for non-RBD modes
	}

	imageName := "volume-" + volumeID
	snapName := "snap-" + snapshotID

	// Open RBD image
	image, err := rbd.OpenImage(c.ioctx, imageName, "")
	if err != nil {
		return fmt.Errorf("failed to open RBD image %s: %w", imageName, err)
	}
	defer image.Close()

	// Create snapshot
	snapshot, err := image.CreateSnapshot(snapName)
	if err != nil {
		return fmt.Errorf("failed to create snapshot %s: %w", snapName, err)
	}
	snapshot.Release()

	return nil
}

// DeleteSnapshot deletes a snapshot (build tag: ceph)
func (c *CephClient) DeleteSnapshotRBD(ctx context.Context, volumeID, snapshotID string) error {
	if c.ioctx == nil {
		return nil // Stub behavior for non-RBD modes
	}

	imageName := "volume-" + volumeID
	snapName := "snap-" + snapshotID

	// Open RBD image
	image, err := rbd.OpenImage(c.ioctx, imageName, "")
	if err != nil {
		return fmt.Errorf("failed to open RBD image %s: %w", imageName, err)
	}
	defer image.Close()

	// Get snapshot
	snapshot := image.GetSnapshot(snapName)

	// Remove snapshot
	err = snapshot.Remove()
	if err != nil {
		return fmt.Errorf("failed to remove snapshot %s: %w", snapName, err)
	}

	return nil
}

// GetVolumeSizeRBD gets the size of a volume (build tag: ceph)
func (c *CephClient) GetVolumeSizeRBD(ctx context.Context, volumeID string) (int, error) {
	if c.ioctx == nil {
		return 0, nil // Stub behavior
	}

	imageName := "volume-" + volumeID

	// Open RBD image
	image, err := rbd.OpenImage(c.ioctx, imageName, "")
	if err != nil {
		return 0, fmt.Errorf("failed to open RBD image %s: %w", imageName, err)
	}
	defer image.Close()

	// Get image size
	size, err := image.GetSize()
	if err != nil {
		return 0, fmt.Errorf("failed to get image size: %w", err)
	}

	// Convert bytes to GB
	sizeGB := int(size / (1024 * 1024 * 1024))
	return sizeGB, nil
}

// HealthRBD checks if Ceph cluster is accessible (build tag: ceph)
func (c *CephClient) HealthRBD(ctx context.Context) error {
	if c.conn == nil {
		return fmt.Errorf("Ceph connection not initialized")
	}

	// Check connection status
	if err := c.conn.PingMonitor(""); err != nil {
		return fmt.Errorf("Ceph cluster health check failed: %w", err)
	}

	return nil
}
