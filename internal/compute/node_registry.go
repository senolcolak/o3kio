package compute

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/google/uuid"
)

// NodeRegistry manages compute node registration and heartbeats
type NodeRegistry struct {
	nodeID            string
	hostname          string
	tunnelIP          string
	heartbeatInterval time.Duration
	stopChan          chan struct{}
}

// NewNodeRegistry creates a new node registry
func NewNodeRegistry(nodeID, tunnelIP string, heartbeatInterval time.Duration) (*NodeRegistry, error) {
	// Auto-generate node ID if not provided
	if nodeID == "" || nodeID == "auto" {
		nodeID = uuid.New().String()
	}

	// Auto-detect tunnel IP if not provided
	if tunnelIP == "" || tunnelIP == "auto" {
		detectedIP, err := detectTunnelIP()
		if err != nil {
			return nil, fmt.Errorf("failed to auto-detect tunnel IP: %w", err)
		}
		tunnelIP = detectedIP
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	if heartbeatInterval == 0 {
		heartbeatInterval = 30 * time.Second
	}

	return &NodeRegistry{
		nodeID:            nodeID,
		hostname:          hostname,
		tunnelIP:          tunnelIP,
		heartbeatInterval: heartbeatInterval,
		stopChan:          make(chan struct{}),
	}, nil
}

// RegisterNode registers this node in the database
func (nr *NodeRegistry) RegisterNode(ctx context.Context) error {
	now := time.Now()

	// Upsert node registration
	_, err := database.DB.Exec(ctx, `
		INSERT INTO compute_nodes (id, hostname, tunnel_ip, status, last_heartbeat, created_at, updated_at)
		VALUES ($1, $2, $3, 'active', $4, $5, $6)
		ON CONFLICT (hostname)
		DO UPDATE SET
			tunnel_ip = EXCLUDED.tunnel_ip,
			status = 'active',
			last_heartbeat = EXCLUDED.last_heartbeat,
			updated_at = EXCLUDED.updated_at
	`, nr.nodeID, nr.hostname, nr.tunnelIP, now, now, now)

	if err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}

	return nil
}

// StartHeartbeat starts the heartbeat goroutine
func (nr *NodeRegistry) StartHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(nr.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := nr.sendHeartbeat(ctx); err != nil {
				// Log error but continue
				fmt.Printf("Heartbeat error: %v\n", err)
			}
		case <-nr.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// sendHeartbeat updates the last_heartbeat timestamp
func (nr *NodeRegistry) sendHeartbeat(ctx context.Context) error {
	_, err := database.DB.Exec(ctx, `
		UPDATE compute_nodes
		SET last_heartbeat = $1, updated_at = $1
		WHERE hostname = $2
	`, time.Now(), nr.hostname)

	return err
}

// StopHeartbeat stops the heartbeat goroutine
func (nr *NodeRegistry) StopHeartbeat() {
	close(nr.stopChan)
}

// ListActiveNodes returns all nodes with recent heartbeat (within 2x heartbeat interval)
func (nr *NodeRegistry) ListActiveNodes(ctx context.Context) ([]ComputeNode, error) {
	threshold := time.Now().Add(-2 * nr.heartbeatInterval)

	rows, err := database.DB.Query(ctx, `
		SELECT id, hostname, tunnel_ip, status, last_heartbeat, created_at
		FROM compute_nodes
		WHERE last_heartbeat > $1 AND status = 'active'
		ORDER BY hostname
	`, threshold)

	if err != nil {
		return nil, fmt.Errorf("failed to list active nodes: %w", err)
	}
	defer rows.Close()

	var nodes []ComputeNode
	for rows.Next() {
		var node ComputeNode
		if err := rows.Scan(&node.ID, &node.Hostname, &node.TunnelIP, &node.Status, &node.LastHeartbeat, &node.CreatedAt); err != nil {
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetTunnelIP returns the tunnel IP for this node
func (nr *NodeRegistry) GetTunnelIP() string {
	return nr.tunnelIP
}

// GetNodeID returns the node ID
func (nr *NodeRegistry) GetNodeID() string {
	return nr.nodeID
}

// GetHostname returns the hostname
func (nr *NodeRegistry) GetHostname() string {
	return nr.hostname
}

// ComputeNode represents a compute node
type ComputeNode struct {
	ID            string
	Hostname      string
	TunnelIP      string
	Status        string
	LastHeartbeat time.Time
	CreatedAt     time.Time
}

// detectTunnelIP auto-detects the primary interface IP
func detectTunnelIP() (string, error) {
	// Get all network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	// Look for the first non-loopback interface with an IPv4 address
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String(), nil
				}
			}
		}
	}

	return "", fmt.Errorf("no suitable network interface found")
}
