package networking

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"

	"github.com/vishvananda/netlink"
)

// VXLANManager manages VXLAN interfaces and forwarding database entries
type VXLANManager struct {
	mode       string // "stub" or real mode (iptables/ebpf)
	vxlanPort  int
	mu         sync.Mutex
	stubVXLANs map[string]bool              // For stub mode: networkID -> exists
	stubFDBs   map[string]map[string]string // For stub mode: networkID -> MAC -> remoteIP
}

// NewVXLANManager creates a new VXLAN manager
func NewVXLANManager(mode string, vxlanPort int) *VXLANManager {
	if vxlanPort == 0 {
		vxlanPort = 4789 // Default VXLAN port
	}

	return &VXLANManager{
		mode:       mode,
		vxlanPort:  vxlanPort,
		stubVXLANs: make(map[string]bool),
		stubFDBs:   make(map[string]map[string]string),
	}
}

// CreateVXLAN creates a VXLAN interface for a network
// networkID: Network UUID
// vni: VXLAN Network Identifier (1000-16777215)
// localIP: Local tunnel endpoint IP
func (m *VXLANManager) CreateVXLAN(networkID string, vni int, localIP string) error {
	if m.mode == "stub" {
		return m.createVXLANStub(networkID, vni)
	}
	return m.createVXLANReal(networkID, vni, localIP)
}

// createVXLANStub simulates VXLAN creation
func (m *VXLANManager) createVXLANStub(networkID string, vni int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stubVXLANs[networkID] = true
	m.stubFDBs[networkID] = make(map[string]string)
	return nil
}

// createVXLANReal creates an actual VXLAN interface
func (m *VXLANManager) createVXLANReal(networkID string, vni int, localIP string) error {
	vxlanName := m.GetVXLANName(networkID)

	// Check if VXLAN interface already exists
	if _, err := netlink.LinkByName(vxlanName); err == nil {
		return nil // Already exists
	}

	// Parse local IP
	localAddr := net.ParseIP(localIP)
	if localAddr == nil {
		return fmt.Errorf("invalid local IP: %s", localIP)
	}

	// Create VXLAN link using netlink
	vxlan := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: vxlanName,
			MTU:  1450, // Standard MTU minus VXLAN overhead (50 bytes)
		},
		VxlanId:  vni,
		Port:     m.vxlanPort,
		Learning: true,  // Enable MAC learning
		SrcAddr:  localAddr,
		L2miss:   true,  // Notify on L2 miss
		L3miss:   true,  // Notify on L3 miss
		NoAge:    false, // Allow FDB entry aging
		GBP:      false, // No Group-Based Policy
		Age:      300,   // FDB entry age timeout (seconds)
	}

	if err := netlink.LinkAdd(vxlan); err != nil {
		if !strings.Contains(err.Error(), "exists") {
			return fmt.Errorf("failed to create VXLAN interface %s: %w", vxlanName, err)
		}
	}

	// Bring the VXLAN interface up
	link, err := netlink.LinkByName(vxlanName)
	if err != nil {
		return fmt.Errorf("failed to find VXLAN interface %s: %w", vxlanName, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to bring up VXLAN interface %s: %w", vxlanName, err)
	}

	return nil
}

// DeleteVXLAN deletes a VXLAN interface
func (m *VXLANManager) DeleteVXLAN(networkID string) error {
	if m.mode == "stub" {
		return m.deleteVXLANStub(networkID)
	}
	return m.deleteVXLANReal(networkID)
}

// deleteVXLANStub simulates VXLAN deletion
func (m *VXLANManager) deleteVXLANStub(networkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.stubVXLANs, networkID)
	delete(m.stubFDBs, networkID)
	return nil
}

// deleteVXLANReal deletes an actual VXLAN interface
func (m *VXLANManager) deleteVXLANReal(networkID string) error {
	vxlanName := m.GetVXLANName(networkID)

	link, err := netlink.LinkByName(vxlanName)
	if err != nil {
		return nil // Already deleted
	}

	return netlink.LinkDel(link)
}

// AddFDBEntry adds a forwarding database entry (MAC -> remote VTEP IP mapping)
// This tells the kernel where to send packets for a specific MAC address
func (m *VXLANManager) AddFDBEntry(networkID, macAddress, remoteIP string) error {
	if m.mode == "stub" {
		return m.addFDBEntryStub(networkID, macAddress, remoteIP)
	}
	return m.addFDBEntryReal(networkID, macAddress, remoteIP)
}

// addFDBEntryStub simulates FDB entry addition
func (m *VXLANManager) addFDBEntryStub(networkID, macAddress, remoteIP string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stubFDBs[networkID] == nil {
		m.stubFDBs[networkID] = make(map[string]string)
	}
	m.stubFDBs[networkID][macAddress] = remoteIP
	return nil
}

// addFDBEntryReal adds an actual FDB entry using netlink
func (m *VXLANManager) addFDBEntryReal(networkID, macAddress, remoteIP string) error {
	vxlanName := m.GetVXLANName(networkID)

	link, err := netlink.LinkByName(vxlanName)
	if err != nil {
		return fmt.Errorf("failed to find VXLAN interface %s: %w", vxlanName, err)
	}

	// Parse MAC address
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return fmt.Errorf("invalid MAC address %s: %w", macAddress, err)
	}

	// Parse remote IP
	remoteAddr := net.ParseIP(remoteIP)
	if remoteAddr == nil {
		return fmt.Errorf("invalid remote IP: %s", remoteIP)
	}

	// Create FDB entry
	neigh := &netlink.Neigh{
		LinkIndex:    link.Attrs().Index,
		Family:       AF_BRIDGE,
		State:        NUD_PERMANENT,
		Flags:        NTF_SELF,
		IP:           remoteAddr,
		HardwareAddr: mac,
	}

	// Add or replace FDB entry
	if err := netlink.NeighSet(neigh); err != nil {
		return fmt.Errorf("failed to add FDB entry for %s: %w", macAddress, err)
	}

	return nil
}

// RemoveFDBEntry removes a forwarding database entry
func (m *VXLANManager) RemoveFDBEntry(networkID, macAddress string) error {
	if m.mode == "stub" {
		return m.removeFDBEntryStub(networkID, macAddress)
	}
	return m.removeFDBEntryReal(networkID, macAddress)
}

// removeFDBEntryStub simulates FDB entry removal
func (m *VXLANManager) removeFDBEntryStub(networkID, macAddress string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stubFDBs[networkID] != nil {
		delete(m.stubFDBs[networkID], macAddress)
	}
	return nil
}

// removeFDBEntryReal removes an actual FDB entry
func (m *VXLANManager) removeFDBEntryReal(networkID, macAddress string) error {
	vxlanName := m.GetVXLANName(networkID)

	link, err := netlink.LinkByName(vxlanName)
	if err != nil {
		return nil // Interface doesn't exist, entry already gone
	}

	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return fmt.Errorf("invalid MAC address %s: %w", macAddress, err)
	}

	neigh := &netlink.Neigh{
		LinkIndex:    link.Attrs().Index,
		Family:       AF_BRIDGE,
		HardwareAddr: mac,
		Flags:        NTF_SELF,
	}

	// Best effort deletion - ignore errors
	netlink.NeighDel(neigh)
	return nil
}

// AttachToBridge attaches VXLAN interface to a bridge
// This allows VMs on the bridge to communicate over the VXLAN tunnel
func (m *VXLANManager) AttachToBridge(networkID, bridgeName string, inNamespace bool, nsName string) error {
	if m.mode == "stub" {
		return nil // No-op in stub mode
	}
	return m.attachToBridgeReal(networkID, bridgeName, inNamespace, nsName)
}

// attachToBridgeReal attaches VXLAN interface to bridge
func (m *VXLANManager) attachToBridgeReal(networkID, bridgeName string, inNamespace bool, nsName string) error {
	vxlanName := m.GetVXLANName(networkID)

	if inNamespace {
		// First, move VXLAN interface to namespace if not already there
		cmd := exec.Command("ip", "link", "set", vxlanName, "netns", nsName)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Ignore error if already in namespace
			if !strings.Contains(string(output), "Cannot find device") {
				return fmt.Errorf("failed to move VXLAN to namespace: %w, output: %s", err, output)
			}
		}

		// Attach to bridge in namespace
		cmd = exec.Command("ip", "netns", "exec", nsName, "ip", "link", "set", vxlanName, "master", bridgeName)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to attach VXLAN to bridge: %w, output: %s", err, output)
		}

		// Ensure VXLAN interface is up in namespace
		cmd = exec.Command("ip", "netns", "exec", nsName, "ip", "link", "set", vxlanName, "up")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to bring up VXLAN in namespace: %w, output: %s", err, output)
		}
	} else {
		// Attach to bridge in default namespace
		vxlanLink, err := netlink.LinkByName(vxlanName)
		if err != nil {
			return fmt.Errorf("failed to find VXLAN interface: %w", err)
		}

		bridgeLink, err := netlink.LinkByName(bridgeName)
		if err != nil {
			return fmt.Errorf("failed to find bridge: %w", err)
		}

		if err := netlink.LinkSetMaster(vxlanLink, bridgeLink.(*netlink.Bridge)); err != nil {
			return fmt.Errorf("failed to attach VXLAN to bridge: %w", err)
		}
	}

	return nil
}

// GetVXLANName returns the VXLAN interface name for a network
// Format: vxlan-{first 8 chars of network UUID}
func (m *VXLANManager) GetVXLANName(networkID string) string {
	if len(networkID) > 8 {
		return "vxlan-" + networkID[:8]
	}
	return "vxlan-" + networkID
}

// ListFDBEntries lists all FDB entries for a network (for debugging)
func (m *VXLANManager) ListFDBEntries(networkID string) (map[string]string, error) {
	if m.mode == "stub" {
		m.mu.Lock()
		defer m.mu.Unlock()

		entries := make(map[string]string)
		if m.stubFDBs[networkID] != nil {
			for mac, ip := range m.stubFDBs[networkID] {
				entries[mac] = ip
			}
		}
		return entries, nil
	}

	// In real mode, query kernel FDB table
	vxlanName := m.GetVXLANName(networkID)
	link, err := netlink.LinkByName(vxlanName)
	if err != nil {
		return nil, fmt.Errorf("failed to find VXLAN interface: %w", err)
	}

	neighbors, err := netlink.NeighList(link.Attrs().Index, AF_BRIDGE)
	if err != nil {
		return nil, fmt.Errorf("failed to list FDB entries: %w", err)
	}

	entries := make(map[string]string)
	for _, neigh := range neighbors {
		if neigh.HardwareAddr != nil && neigh.IP != nil {
			entries[neigh.HardwareAddr.String()] = neigh.IP.String()
		}
	}

	return entries, nil
}
