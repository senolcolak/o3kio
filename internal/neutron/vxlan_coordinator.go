package neutron

import (
	"context"
	"fmt"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/compute"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/pkg/networking"
)

// VXLANCoordinator coordinates VXLAN network state across compute nodes
type VXLANCoordinator struct {
	vxlanManager  *networking.VXLANManager
	nodeRegistry  *compute.NodeRegistry
	nsManager     *networking.NetworkNamespaceManager
	db            database.DBIF
	pollInterval  time.Duration
	vniRangeStart int
	vniRangeEnd   int
	stopChan      chan struct{}
}

// NewVXLANCoordinator creates a new VXLAN coordinator
func NewVXLANCoordinator(
	vxlanManager *networking.VXLANManager,
	nodeRegistry *compute.NodeRegistry,
	nsManager *networking.NetworkNamespaceManager,
	pollInterval time.Duration,
	vniRangeStart, vniRangeEnd int,
) *VXLANCoordinator {
	if pollInterval == 0 {
		pollInterval = 1 * time.Second
	}

	return &VXLANCoordinator{
		vxlanManager:  vxlanManager,
		nodeRegistry:  nodeRegistry,
		nsManager:     nsManager,
		pollInterval:  pollInterval,
		vniRangeStart: vniRangeStart,
		vniRangeEnd:   vniRangeEnd,
		stopChan:      make(chan struct{}),
	}
}

// activeDB returns the injected DB or falls back to the global database.DB
func (vc *VXLANCoordinator) activeDB() database.DBIF {
	if vc.db != nil {
		return vc.db
	}
	return database.DB
}

// Start begins the coordination loop
func (vc *VXLANCoordinator) Start(ctx context.Context) {
	ticker := time.NewTicker(vc.pollInterval)
	defer ticker.Stop()

	// Do initial sync
	vc.syncNetworks(ctx)
	vc.syncPorts(ctx)

	for {
		select {
		case <-ticker.C:
			vc.syncNetworks(ctx)
			vc.syncPorts(ctx)
		case <-vc.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop stops the coordination loop
func (vc *VXLANCoordinator) Stop() {
	close(vc.stopChan)
}

// syncNetworks ensures all VXLAN networks exist on this node
func (vc *VXLANCoordinator) syncNetworks(ctx context.Context) {
	// Get all networks that need VXLAN
	rows, err := vc.activeDB().Query(ctx, `
		SELECT n.id, n.name, n.project_id, v.vni
		FROM networks n
		LEFT JOIN network_vni_allocations v ON n.id = v.network_id
		WHERE n.network_type = 'vxlan'
	`)

	if err != nil {
		fmt.Printf("Failed to query networks: %v\n", err)
		return
	}
	defer rows.Close()

	localIP := vc.nodeRegistry.GetTunnelIP()

	for rows.Next() {
		var networkID, name, projectID string
		var vni *int

		if err := rows.Scan(&networkID, &name, &projectID, &vni); err != nil {
			continue
		}

		// Allocate VNI if not already allocated
		if vni == nil {
			allocatedVNI, err := vc.allocateVNI(ctx, networkID)
			if err != nil {
				fmt.Printf("Failed to allocate VNI for network %s: %v\n", networkID, err)
				continue
			}
			vni = &allocatedVNI
		}

		// Create VXLAN interface
		if err := vc.vxlanManager.CreateVXLAN(networkID, *vni, localIP); err != nil {
			fmt.Printf("Failed to create VXLAN for network %s: %v\n", networkID, err)
			continue
		}

		// Attach to bridge
		bridgeName := "br-" + networkID[:8]
		nsName := vc.nsManager.GetNamespaceName(projectID)
		if err := vc.vxlanManager.AttachToBridge(networkID, bridgeName, true, nsName); err != nil {
			fmt.Printf("Failed to attach VXLAN to bridge for network %s: %v\n", networkID, err)
		}
	}
	if err := rows.Err(); err != nil {
		fmt.Printf("Failed to iterate network rows: %v\n", err)
	}
}

// syncPorts ensures all port FDB entries are configured
func (vc *VXLANCoordinator) syncPorts(ctx context.Context) {
	// Get all active compute nodes
	nodes, err := vc.nodeRegistry.ListActiveNodes(ctx)
	if err != nil {
		fmt.Printf("Failed to list active nodes: %v\n", err)
		return
	}

	// Get all FDB entries from database
	rows, err := vc.activeDB().Query(ctx, `
		SELECT network_id, mac_address, vtep_ip
		FROM vxlan_fdb_entries
	`)

	if err != nil {
		fmt.Printf("Failed to query FDB entries: %v\n", err)
		return
	}
	defer rows.Close()

	localIP := vc.nodeRegistry.GetTunnelIP()

	for rows.Next() {
		var networkID, macAddress, vtepIP string

		if err := rows.Scan(&networkID, &macAddress, &vtepIP); err != nil {
			continue
		}

		// Skip entries for this node's own ports
		if vtepIP == localIP {
			continue
		}

		// Verify the remote node is still active
		nodeActive := false
		for _, node := range nodes {
			if node.TunnelIP == vtepIP {
				nodeActive = true
				break
			}
		}

		if !nodeActive {
			// Remote node is down, remove FDB entry
			vc.vxlanManager.RemoveFDBEntry(networkID, macAddress)
			continue
		}

		// Add FDB entry
		if err := vc.vxlanManager.AddFDBEntry(networkID, macAddress, vtepIP); err != nil {
			fmt.Printf("Failed to add FDB entry for %s on network %s: %v\n", macAddress, networkID, err)
		}
	}
	if err := rows.Err(); err != nil {
		fmt.Printf("Failed to iterate FDB entry rows: %v\n", err)
	}
}

// allocateVNI allocates a VNI for a network
func (vc *VXLANCoordinator) allocateVNI(ctx context.Context, networkID string) (int, error) {
	// Try to allocate a VNI atomically using ON CONFLICT
	for vni := vc.vniRangeStart; vni <= vc.vniRangeEnd; vni++ {
		result, err := vc.activeDB().Exec(ctx, `
			INSERT INTO network_vni_allocations (network_id, vni)
			VALUES ($1, $2)
			ON CONFLICT (vni) DO NOTHING
		`, networkID, vni)

		if err != nil {
			continue
		}

		rowsAffected := result.RowsAffected()
		if rowsAffected > 0 {
			return vni, nil
		}
	}

	return 0, fmt.Errorf("no available VNI in range %d-%d", vc.vniRangeStart, vc.vniRangeEnd)
}

// DistributeFDBEntry adds an FDB entry to the database for distribution
// This should be called when a new port is created
func (vc *VXLANCoordinator) DistributeFDBEntry(ctx context.Context, networkID, portID, macAddress string) error {
	localIP := vc.nodeRegistry.GetTunnelIP()
	now := time.Now()

	_, err := vc.activeDB().Exec(ctx, `
		INSERT INTO vxlan_fdb_entries (network_id, mac_address, vtep_ip, port_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (network_id, mac_address)
		DO UPDATE SET
			vtep_ip = EXCLUDED.vtep_ip,
			port_id = EXCLUDED.port_id,
			updated_at = EXCLUDED.updated_at
	`, networkID, macAddress, localIP, portID, now, now)

	return err
}

// RemoveFDBEntry removes an FDB entry from the database
// This should be called when a port is deleted
func (vc *VXLANCoordinator) RemoveFDBEntry(ctx context.Context, portID string) error {
	_, err := vc.activeDB().Exec(ctx, `
		DELETE FROM vxlan_fdb_entries
		WHERE port_id = $1
	`, portID)

	return err
}

// GetVNI returns the VNI for a network (allocating if necessary)
func (vc *VXLANCoordinator) GetVNI(ctx context.Context, networkID string) (int, error) {
	// Try to get existing VNI
	var vni int
	err := vc.activeDB().QueryRow(ctx, `
		SELECT vni FROM network_vni_allocations WHERE network_id = $1
	`, networkID).Scan(&vni)

	if err == nil {
		return vni, nil
	}

	// Allocate new VNI
	return vc.allocateVNI(ctx, networkID)
}
