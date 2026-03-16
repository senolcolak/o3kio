//go:build !ceph
// +build !ceph

package storage

import (
	"context"
	"fmt"
)

// initCephConnection stub (no ceph build tag)
func (c *CephClient) initCephConnection() error {
	return fmt.Errorf("Ceph support not compiled (build with -tags ceph)")
}

// Close stub (no ceph build tag)
func (c *CephClient) Close() error {
	return nil
}

// createVolumeRBD stub (no ceph build tag)
func (c *CephClient) createVolumeRBD(ctx context.Context, volumeID string, sizeGB int) error {
	return fmt.Errorf("Ceph RBD support not compiled (build with -tags ceph)")
}

// deleteVolumeRBD stub (no ceph build tag)
func (c *CephClient) deleteVolumeRBD(ctx context.Context, volumeID string) error {
	return fmt.Errorf("Ceph RBD support not compiled (build with -tags ceph)")
}

// volumeExistsRBD stub (no ceph build tag)
func (c *CephClient) volumeExistsRBD(ctx context.Context, volumeID string) (bool, error) {
	return false, fmt.Errorf("Ceph RBD support not compiled (build with -tags ceph)")
}

// CreateSnapshotRBD stub (no ceph build tag)
func (c *CephClient) CreateSnapshotRBD(ctx context.Context, volumeID, snapshotID string) error {
	return fmt.Errorf("Ceph RBD support not compiled (build with -tags ceph)")
}

// DeleteSnapshotRBD stub (no ceph build tag)
func (c *CephClient) DeleteSnapshotRBD(ctx context.Context, volumeID, snapshotID string) error {
	return fmt.Errorf("Ceph RBD support not compiled (build with -tags ceph)")
}

// GetVolumeSizeRBD stub (no ceph build tag)
func (c *CephClient) GetVolumeSizeRBD(ctx context.Context, volumeID string) (int, error) {
	return 0, fmt.Errorf("Ceph RBD support not compiled (build with -tags ceph)")
}

// HealthRBD stub (no ceph build tag)
func (c *CephClient) HealthRBD(ctx context.Context) error {
	return fmt.Errorf("Ceph RBD support not compiled (build with -tags ceph)")
}
