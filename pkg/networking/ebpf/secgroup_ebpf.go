package ebpf

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

// SecurityGroupMode defines the security group implementation mode
type SecurityGroupMode string

const (
	ModeIPTables SecurityGroupMode = "iptables"
	ModeEBPF     SecurityGroupMode = "ebpf"
	ModeStub     SecurityGroupMode = "stub"
)

// SecurityGroupRule represents a security group rule
type SecurityGroupRule struct {
	Protocol      uint8  // syscall.IPPROTO_TCP, IPPROTO_UDP, IPPROTO_ICMP, 0=any
	Direction     uint8  // 0=ingress, 1=egress
	PortMin       uint16 // Minimum port (host byte order)
	PortMax       uint16 // Maximum port (host byte order)
	RemoteIPCIDR  string // "0.0.0.0/0", "192.168.1.0/24", etc.
}

// SecurityGroupManager manages eBPF-based security groups
type SecurityGroupManager struct {
	coll     *ebpf.Collection
	prog     *ebpf.Program
	sgRules  *ebpf.Map
	sgStats  *ebpf.Map
	links    map[string]link.Link // interface name -> XDP link
}

// Statistics tracks packet counters
type Statistics struct {
	PacketsAllowed   uint64
	PacketsDenied    uint64
	PacketsProcessed uint64
}

// NewSecurityGroupManager creates an eBPF security group manager
func NewSecurityGroupManager(ebpfObjectPath string) (*SecurityGroupManager, error) {
	// Load compiled eBPF object file
	spec, err := ebpf.LoadCollectionSpec(ebpfObjectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load eBPF spec: %w", err)
	}

	// Create collection
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create eBPF collection: %w", err)
	}

	// Get program and maps
	prog := coll.Programs["xdp_security_group_filter"]
	if prog == nil {
		return nil, fmt.Errorf("eBPF program 'xdp_security_group_filter' not found")
	}

	sgRules := coll.Maps["sg_rules"]
	if sgRules == nil {
		return nil, fmt.Errorf("eBPF map 'sg_rules' not found")
	}

	sgStats := coll.Maps["sg_statistics"]
	if sgStats == nil {
		return nil, fmt.Errorf("eBPF map 'sg_statistics' not found")
	}

	return &SecurityGroupManager{
		coll:    coll,
		prog:    prog,
		sgRules: sgRules,
		sgStats: sgStats,
		links:   make(map[string]link.Link),
	}, nil
}

// AttachToInterface attaches the XDP program to a network interface
func (m *SecurityGroupManager) AttachToInterface(ifaceName string) error {
	// Get interface by name
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return fmt.Errorf("failed to get interface %s: %w", ifaceName, err)
	}

	// Check if already attached
	if _, exists := m.links[ifaceName]; exists {
		return fmt.Errorf("XDP program already attached to %s", ifaceName)
	}

	// Attach XDP program to interface
	l, err := link.AttachXDP(link.XDPOptions{
		Program:   m.prog,
		Interface: iface.Index,
		Flags:     link.XDPGenericMode, // Use generic mode (SKB-based, compatible with all drivers)
	})
	if err != nil {
		return fmt.Errorf("failed to attach XDP to %s: %w", ifaceName, err)
	}

	m.links[ifaceName] = l
	return nil
}

// DetachFromInterface detaches the XDP program from a network interface
func (m *SecurityGroupManager) DetachFromInterface(ifaceName string) error {
	l, exists := m.links[ifaceName]
	if !exists {
		return fmt.Errorf("XDP program not attached to %s", ifaceName)
	}

	if err := l.Close(); err != nil {
		return fmt.Errorf("failed to detach XDP from %s: %w", ifaceName, err)
	}

	delete(m.links, ifaceName)
	return nil
}

// UpdateSecurityGroup updates security group rules for a specific port
func (m *SecurityGroupManager) UpdateSecurityGroup(portID uint32, rules []SecurityGroupRule) error {
	// Validate rule count
	if len(rules) > 100 {
		return fmt.Errorf("too many rules: %d (max 100)", len(rules))
	}

	// Serialize rule set to binary format
	// Format: [rule_count:4 bytes][rule1:24 bytes][rule2:24 bytes]...
	buf := make([]byte, 4+len(rules)*24)

	// Write rule count
	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(rules)))

	// Write each rule
	offset := 4
	for i, rule := range rules {
		// Parse CIDR
		_, ipnet, err := net.ParseCIDR(rule.RemoteIPCIDR)
		if err != nil {
			return fmt.Errorf("invalid CIDR in rule %d: %w", i, err)
		}

		// Convert IP to uint32 (network byte order)
		ipBytes := ipnet.IP.To4()
		if ipBytes == nil {
			return fmt.Errorf("invalid IPv4 address in rule %d", i)
		}
		ipPrefix := binary.BigEndian.Uint32(ipBytes)

		// Convert mask to uint32 (network byte order)
		maskBytes := ipnet.Mask
		if len(maskBytes) != 4 {
			return fmt.Errorf("invalid netmask in rule %d", i)
		}
		ipMask := binary.BigEndian.Uint32(maskBytes)

		// Serialize rule (24 bytes total)
		ruleOffset := offset + i*24
		buf[ruleOffset] = rule.Protocol                           // 1 byte
		buf[ruleOffset+1] = rule.Direction                        // 1 byte
		binary.LittleEndian.PutUint16(buf[ruleOffset+2:], rule.PortMin)  // 2 bytes
		binary.LittleEndian.PutUint16(buf[ruleOffset+4:], rule.PortMax)  // 2 bytes
		binary.LittleEndian.PutUint32(buf[ruleOffset+6:], ipPrefix)      // 4 bytes
		binary.LittleEndian.PutUint32(buf[ruleOffset+10:], ipMask)       // 4 bytes
		// Padding: 10 bytes reserved for future use
	}

	// Update eBPF map
	if err := m.sgRules.Put(portID, buf); err != nil {
		return fmt.Errorf("failed to update eBPF map: %w", err)
	}

	return nil
}

// RemoveSecurityGroup removes security group rules for a specific port
func (m *SecurityGroupManager) RemoveSecurityGroup(portID uint32) error {
	if err := m.sgRules.Delete(portID); err != nil {
		return fmt.Errorf("failed to delete from eBPF map: %w", err)
	}
	return nil
}

// GetStatistics retrieves packet statistics
func (m *SecurityGroupManager) GetStatistics() (*Statistics, error) {
	var stats Statistics
	key := uint32(0)

	// Read statistics from map
	buf := make([]byte, 24) // 3 * uint64
	if err := m.sgStats.Lookup(key, &buf); err != nil {
		return nil, fmt.Errorf("failed to read statistics: %w", err)
	}

	stats.PacketsAllowed = binary.LittleEndian.Uint64(buf[0:8])
	stats.PacketsDenied = binary.LittleEndian.Uint64(buf[8:16])
	stats.PacketsProcessed = binary.LittleEndian.Uint64(buf[16:24])

	return &stats, nil
}

// Close detaches all XDP programs and releases resources
func (m *SecurityGroupManager) Close() error {
	// Detach from all interfaces
	for ifaceName := range m.links {
		if err := m.DetachFromInterface(ifaceName); err != nil {
			return err
		}
	}

	// Close collection (also closes maps and programs)
	m.coll.Close() // Note: Close() doesn't return error in cilium/ebpf v0.12+

	return nil
}

// MACToPortID converts MAC address to port ID hash
func MACToPortID(mac net.HardwareAddr) uint32 {
	var hash uint32
	for _, b := range mac {
		hash = hash*31 + uint32(b)
	}
	return hash
}

// ProtocolStringToInt converts protocol string to syscall constant
func ProtocolStringToInt(protocol string) uint8 {
	switch protocol {
	case "tcp":
		return syscall.IPPROTO_TCP
	case "udp":
		return syscall.IPPROTO_UDP
	case "icmp":
		return syscall.IPPROTO_ICMP
	default:
		return 0 // any protocol
	}
}
