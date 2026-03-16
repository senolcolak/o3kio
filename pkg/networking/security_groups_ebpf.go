package networking

import (
	"fmt"
	"net"

	"github.com/cobaltcore-dev/o3k/pkg/networking/ebpf"
)

// ApplySecurityGroupToPort applies security group rules to a specific port (eBPF-specific)
func (m *SecurityGroupManager) ApplySecurityGroupToPort(portID string, portMAC net.HardwareAddr, rules []SecurityGroupRule) error {
	if m.mode != "ebpf" {
		return fmt.Errorf("ApplySecurityGroupToPort only supported in eBPF mode")
	}

	// Convert port MAC address to port ID hash
	portIDHash := ebpf.MACToPortID(portMAC)

	// Convert Neutron security group rules to eBPF format
	ebpfRules := make([]ebpf.SecurityGroupRule, 0, len(rules))
	for _, rule := range rules {
		// Skip egress rules (eBPF XDP only handles ingress)
		if rule.Direction == "egress" {
			continue
		}

		// Convert protocol string to int
		protocol := ebpf.ProtocolStringToInt(rule.Protocol)

		// Use default CIDR if not specified
		remoteIP := rule.RemoteIPPrefix
		if remoteIP == "" {
			remoteIP = "0.0.0.0/0" // Allow from any
		}

		// Convert port range (Neutron uses -1 for "any")
		portMin := uint16(rule.PortRangeMin)
		portMax := uint16(rule.PortRangeMax)
		if rule.PortRangeMin == -1 {
			portMin = 0
			portMax = 65535
		}

		ebpfRules = append(ebpfRules, ebpf.SecurityGroupRule{
			Protocol:     protocol,
			Direction:    0, // ingress
			PortMin:      portMin,
			PortMax:      portMax,
			RemoteIPCIDR: remoteIP,
		})
	}

	// Update eBPF map
	return m.ebpfMgr.UpdateSecurityGroup(portIDHash, ebpfRules)
}

// RemoveSecurityGroupFromPort removes security group rules from a specific port (eBPF-specific)
func (m *SecurityGroupManager) RemoveSecurityGroupFromPort(portID string, portMAC net.HardwareAddr) error {
	if m.mode != "ebpf" {
		return fmt.Errorf("RemoveSecurityGroupFromPort only supported in eBPF mode")
	}

	portIDHash := ebpf.MACToPortID(portMAC)
	return m.ebpfMgr.RemoveSecurityGroup(portIDHash)
}

// GetStatistics returns eBPF packet filter statistics
func (m *SecurityGroupManager) GetStatistics() (*ebpf.Statistics, error) {
	if m.mode != "ebpf" {
		return nil, fmt.Errorf("GetStatistics only supported in eBPF mode")
	}

	return m.ebpfMgr.GetStatistics()
}

// Close releases all resources (eBPF-specific)
func (m *SecurityGroupManager) Close() error {
	if m.mode == "ebpf" && m.ebpfMgr != nil {
		return m.ebpfMgr.Close()
	}
	return nil
}
