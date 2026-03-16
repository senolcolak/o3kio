package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CephClient manages storage operations (RBD, local, or both)
type CephClient struct {
	mode     string // "stub", "rbd", "local", or "local,rbd"
	pool     string
	confFile string
	timeout  time.Duration
	mu       sync.Mutex
	stubVolumes map[string]*stubVolume // For stub mode
	localPath   string                 // For local mode
	conn        interface{}            // Ceph RADOS connection (type depends on build tags)
	ioctx       interface{}            // RADOS IO context for pool (type depends on build tags)
}

// stubVolume represents a simulated volume
type stubVolume struct {
	id        string
	sizeGB    int
	createdAt time.Time
}

// NewCephClient creates a new storage client
func NewCephClient(mode, pool, confFile string) *CephClient {
	// Use ~/.o3k/volumes for local storage (more portable than /var/lib)
	homeDir, _ := os.UserHomeDir()
	localPath := filepath.Join(homeDir, ".o3k", "volumes")

	client := &CephClient{
		mode:        mode,
		pool:        pool,
		confFile:    confFile,
		timeout:     1 * time.Second, // Fail-fast: 1 second timeout
		stubVolumes: make(map[string]*stubVolume),
		localPath:   localPath,
	}

	// Create local storage directory if needed
	if mode == "local" || mode == "local,rbd" {
		os.MkdirAll(client.localPath, 0755)
	}

	// Initialize Ceph connection if RBD mode
	if mode == "rbd" || mode == "local,rbd" {
		if err := client.initCephConnection(); err != nil {
			fmt.Printf("Warning: failed to initialize Ceph connection: %v\n", err)
			// Continue in degraded mode (will fallback to local if available)
		}
	}

	return client
}


// CreateVolume creates a storage volume
func (c *CephClient) CreateVolume(ctx context.Context, volumeID string, sizeGB int) error {
	switch c.mode {
	case "stub":
		return c.createVolumeStub(volumeID, sizeGB)
	case "local":
		return c.createVolumeLocal(ctx, volumeID, sizeGB)
	case "rbd":
		return c.createVolumeRBD(ctx, volumeID, sizeGB)
	case "local,rbd":
		// Create in local first, then RBD
		if err := c.createVolumeLocal(ctx, volumeID, sizeGB); err != nil {
			return fmt.Errorf("failed to create local volume: %w", err)
		}
		if err := c.createVolumeRBD(ctx, volumeID, sizeGB); err != nil {
			// Rollback local creation
			c.deleteVolumeLocal(ctx, volumeID)
			return fmt.Errorf("failed to create RBD volume: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported storage mode: %s", c.mode)
	}
}

// createVolumeStub simulates volume creation
func (c *CephClient) createVolumeStub(volumeID string, sizeGB int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.stubVolumes[volumeID]; exists {
		return fmt.Errorf("volume %s already exists", volumeID)
	}

	c.stubVolumes[volumeID] = &stubVolume{
		id:        volumeID,
		sizeGB:    sizeGB,
		createdAt: time.Now(),
	}

	return nil
}

// createVolumeLocal creates a local qcow2 volume
func (c *CephClient) createVolumeLocal(ctx context.Context, volumeID string, sizeGB int) error {
	volumePath := filepath.Join(c.localPath, "volume-"+volumeID+".qcow2")

	// Check if already exists
	if _, err := os.Stat(volumePath); err == nil {
		return fmt.Errorf("volume %s already exists", volumeID)
	}

	// Create qcow2 image using qemu-img
	// In real implementation: cmd := exec.CommandContext(ctx, "qemu-img", "create", "-f", "qcow2", volumePath, fmt.Sprintf("%dG", sizeGB))
	// For now, create empty file to simulate
	file, err := os.Create(volumePath)
	if err != nil {
		return fmt.Errorf("failed to create volume file: %w", err)
	}
	defer file.Close()

	// Write size metadata (in real implementation, qemu-img would handle this)
	// Truncate to reserve space (sparse file)
	if err := file.Truncate(int64(sizeGB) * 1024 * 1024 * 1024); err != nil {
		return fmt.Errorf("failed to allocate volume space: %w", err)
	}

	return nil
}


// deleteVolumeLocal deletes a local volume
func (c *CephClient) deleteVolumeLocal(ctx context.Context, volumeID string) error {
	volumePath := filepath.Join(c.localPath, "volume-"+volumeID+".qcow2")
	return os.Remove(volumePath)
}

// DeleteVolume deletes a storage volume
func (c *CephClient) DeleteVolume(ctx context.Context, volumeID string) error {
	switch c.mode {
	case "stub":
		return c.deleteVolumeStub(volumeID)
	case "local":
		return c.deleteVolumeLocal(ctx, volumeID)
	case "rbd":
		return c.deleteVolumeRBD(ctx, volumeID)
	case "local,rbd":
		// Delete both (best effort)
		localErr := c.deleteVolumeLocal(ctx, volumeID)
		rbdErr := c.deleteVolumeRBD(ctx, volumeID)
		if localErr != nil && rbdErr != nil {
			return fmt.Errorf("failed to delete: local=%v, rbd=%v", localErr, rbdErr)
		}
		return nil
	default:
		return fmt.Errorf("unsupported storage mode: %s", c.mode)
	}
}

// deleteVolumeStub simulates volume deletion
func (c *CephClient) deleteVolumeStub(volumeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.stubVolumes[volumeID]; !exists {
		return fmt.Errorf("volume %s not found", volumeID)
	}

	delete(c.stubVolumes, volumeID)
	return nil
}


// CreateSnapshot creates a snapshot of a volume
func (c *CephClient) CreateSnapshot(ctx context.Context, volumeID, snapshotID string) error {
	if c.mode == "rbd" || c.mode == "local,rbd" {
		return c.CreateSnapshotRBD(ctx, volumeID, snapshotID)
	}
	return nil // Stub behavior for non-RBD modes
}

// DeleteSnapshot deletes a snapshot
func (c *CephClient) DeleteSnapshot(ctx context.Context, volumeID, snapshotID string) error {
	if c.mode == "rbd" || c.mode == "local,rbd" {
		return c.DeleteSnapshotRBD(ctx, volumeID, snapshotID)
	}
	return nil // Stub behavior for non-RBD modes
}

// VolumeExists checks if a volume exists
func (c *CephClient) VolumeExists(ctx context.Context, volumeID string) (bool, error) {
	switch c.mode {
	case "stub":
		c.mu.Lock()
		defer c.mu.Unlock()
		_, exists := c.stubVolumes[volumeID]
		return exists, nil
	case "local":
		return c.volumeExistsLocal(volumeID)
	case "rbd":
		return c.volumeExistsRBD(ctx, volumeID)
	case "local,rbd":
		// Check local first (faster)
		if exists, _ := c.volumeExistsLocal(volumeID); exists {
			return true, nil
		}
		return c.volumeExistsRBD(ctx, volumeID)
	default:
		return false, fmt.Errorf("unsupported storage mode: %s", c.mode)
	}
}

// volumeExistsLocal checks if a local volume exists
func (c *CephClient) volumeExistsLocal(volumeID string) (bool, error) {
	volumePath := filepath.Join(c.localPath, "volume-"+volumeID+".qcow2")
	_, err := os.Stat(volumePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// GetVolumeSize gets the size of a volume
func (c *CephClient) GetVolumeSize(ctx context.Context, volumeID string) (int, error) {
	if c.mode == "rbd" || c.mode == "local,rbd" {
		return c.GetVolumeSizeRBD(ctx, volumeID)
	}
	return 0, nil // Stub behavior
}

// GetRBDPath returns the RBD path for a volume (for libvirt attachment)
func (c *CephClient) GetRBDPath(volumeID string) string {
	return fmt.Sprintf("rbd:%s/volume-%s", c.pool, volumeID)
}

// Health checks if Ceph cluster is accessible
func (c *CephClient) Health(ctx context.Context) error {
	if c.mode == "rbd" || c.mode == "local,rbd" {
		return c.HealthRBD(ctx)
	}
	return nil // Stub behavior
}
