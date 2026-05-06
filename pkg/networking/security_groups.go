package networking

import (
	"fmt"
	"sync"

	"github.com/cobaltcore-dev/o3k/pkg/networking/ebpf"
	"github.com/coreos/go-iptables/iptables"
)

// SecurityGroupManager manages security groups with iptables or eBPF
type SecurityGroupManager struct {
	mode       string // "stub", "iptables", or "ebpf"
	ipt        *iptables.IPTables
	ebpfMgr    *ebpf.SecurityGroupManager
	mu         sync.Mutex
	stubChains map[string]bool                // For stub mode
	stubRules  map[string][]SecurityGroupRule // For stub mode
}

func sgChainName(securityGroupID string) string {
	id := securityGroupID
	if len(id) > 8 {
		id = id[:8]
	}
	return "O3K-SG-" + id
}

// NewSecurityGroupManager creates a new security group manager
func NewSecurityGroupManager(mode string, ebpfObjectPath string) (*SecurityGroupManager, error) {
	mgr := &SecurityGroupManager{
		mode:       mode,
		stubChains: make(map[string]bool),
		stubRules:  make(map[string][]SecurityGroupRule),
	}

	if mode == "iptables" {
		ipt, err := iptables.New()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize iptables: %w", err)
		}
		mgr.ipt = ipt
	} else if mode == "ebpf" {
		// Initialize eBPF manager
		ebpfMgr, err := ebpf.NewSecurityGroupManager(ebpfObjectPath)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize eBPF manager: %w", err)
		}
		mgr.ebpfMgr = ebpfMgr
	}

	return mgr, nil
}

// SecurityGroupRule represents a security group rule
type SecurityGroupRule struct {
	ID             string
	Direction      string // "ingress" or "egress"
	EtherType      string // "IPv4" or "IPv6"
	Protocol       string // "tcp", "udp", "icmp", or ""
	PortRangeMin   int
	PortRangeMax   int
	RemoteIPPrefix string
	RemoteGroupID  string
}

// CreateSecurityGroupChain creates a chain/program for a security group
func (m *SecurityGroupManager) CreateSecurityGroupChain(securityGroupID string) error {
	switch m.mode {
	case "stub":
		return m.createSecurityGroupChainStub(securityGroupID)
	case "iptables":
		return m.createSecurityGroupChainIPTables(securityGroupID)
	case "ebpf":
		return m.createSecurityGroupChainEBPF(securityGroupID)
	default:
		return fmt.Errorf("unsupported security group mode: %s", m.mode)
	}
}

// createSecurityGroupChainStub simulates chain creation
func (m *SecurityGroupManager) createSecurityGroupChainStub(securityGroupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	chainName := sgChainName(securityGroupID)
	m.stubChains[chainName] = true
	return nil
}

// createSecurityGroupChainIPTables creates an iptables chain
func (m *SecurityGroupManager) createSecurityGroupChainIPTables(securityGroupID string) error {
	chainName := sgChainName(securityGroupID)

	// Create chain in filter table
	if err := m.ipt.NewChain("filter", chainName); err != nil {
		if err.(*iptables.Error).ExitStatus() != 1 { // 1 means chain already exists
			return fmt.Errorf("failed to create chain %s: %w", chainName, err)
		}
	}

	// Set default policy to DROP
	if err := m.ipt.Append("filter", chainName, "-j", "DROP"); err != nil {
		return fmt.Errorf("failed to set default DROP policy: %w", err)
	}

	return nil
}

// createSecurityGroupChainEBPF creates an eBPF program for security group
func (m *SecurityGroupManager) createSecurityGroupChainEBPF(securityGroupID string) error {
	// eBPF uses port-based rule sets (attached to interfaces)
	// No per-security-group chains needed
	return nil
}

// DeleteSecurityGroupChain deletes an iptables chain
func (m *SecurityGroupManager) DeleteSecurityGroupChain(securityGroupID string) error {
	switch m.mode {
	case "stub":
		return m.deleteSecurityGroupChainStub(securityGroupID)
	case "iptables":
		return m.deleteSecurityGroupChainIPTables(securityGroupID)
	case "ebpf":
		return m.deleteSecurityGroupChainEBPF(securityGroupID)
	default:
		return fmt.Errorf("unsupported security group mode: %s", m.mode)
	}
}

// deleteSecurityGroupChainStub simulates chain deletion
func (m *SecurityGroupManager) deleteSecurityGroupChainStub(securityGroupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	chainName := sgChainName(securityGroupID)
	delete(m.stubChains, chainName)
	delete(m.stubRules, chainName)
	return nil
}

// deleteSecurityGroupChainIPTables deletes an iptables chain
func (m *SecurityGroupManager) deleteSecurityGroupChainIPTables(securityGroupID string) error {
	chainName := sgChainName(securityGroupID)

	// Flush chain first
	_ = m.ipt.ClearChain("filter", chainName)

	// Delete chain
	return m.ipt.DeleteChain("filter", chainName)
}

// deleteSecurityGroupChainEBPF deletes an eBPF program
func (m *SecurityGroupManager) deleteSecurityGroupChainEBPF(securityGroupID string) error {
	// eBPF uses port-based rule sets
	// Security groups are removed by clearing port rules
	return nil
}

// AddRule adds a security group rule
func (m *SecurityGroupManager) AddRule(securityGroupID string, rule SecurityGroupRule) error {
	switch m.mode {
	case "stub":
		return m.addRuleStub(securityGroupID, rule)
	case "iptables":
		return m.addRuleIPTables(securityGroupID, rule)
	case "ebpf":
		return m.addRuleEBPF(securityGroupID, rule)
	default:
		return fmt.Errorf("unsupported security group mode: %s", m.mode)
	}
}

// addRuleStub simulates adding a rule
func (m *SecurityGroupManager) addRuleStub(securityGroupID string, rule SecurityGroupRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	chainName := sgChainName(securityGroupID)
	m.stubRules[chainName] = append(m.stubRules[chainName], rule)
	return nil
}

// addRuleIPTables adds a rule to iptables
func (m *SecurityGroupManager) addRuleIPTables(securityGroupID string, rule SecurityGroupRule) error {
	chainName := sgChainName(securityGroupID)

	// Build iptables rule
	ruleSpec := m.buildRuleSpec(rule)

	// Insert rule before the final DROP rule
	if err := m.ipt.Insert("filter", chainName, 1, ruleSpec...); err != nil {
		return fmt.Errorf("failed to add rule to chain %s: %w", chainName, err)
	}

	return nil
}

// addRuleEBPF adds a rule to eBPF program
func (m *SecurityGroupManager) addRuleEBPF(securityGroupID string, rule SecurityGroupRule) error {
	// eBPF rules are managed per-port, not per-security-group
	// This is handled by ApplySecurityGroupToPort
	return nil
}

// RemoveRule removes a security group rule
func (m *SecurityGroupManager) RemoveRule(securityGroupID string, rule SecurityGroupRule) error {
	switch m.mode {
	case "stub":
		return m.removeRuleStub(securityGroupID, rule)
	case "iptables":
		return m.removeRuleIPTables(securityGroupID, rule)
	case "ebpf":
		return m.removeRuleEBPF(securityGroupID, rule)
	default:
		return fmt.Errorf("unsupported security group mode: %s", m.mode)
	}
}

// removeRuleStub simulates removing a rule
func (m *SecurityGroupManager) removeRuleStub(securityGroupID string, rule SecurityGroupRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	chainName := sgChainName(securityGroupID)
	rules := m.stubRules[chainName]
	for i, r := range rules {
		if r.ID == rule.ID {
			m.stubRules[chainName] = append(rules[:i], rules[i+1:]...)
			break
		}
	}
	return nil
}

// removeRuleIPTables removes a rule from iptables
func (m *SecurityGroupManager) removeRuleIPTables(securityGroupID string, rule SecurityGroupRule) error {
	chainName := sgChainName(securityGroupID)

	ruleSpec := m.buildRuleSpec(rule)

	return m.ipt.Delete("filter", chainName, ruleSpec...)
}

// removeRuleEBPF removes a rule from eBPF program
func (m *SecurityGroupManager) removeRuleEBPF(securityGroupID string, rule SecurityGroupRule) error {
	// eBPF rules are managed per-port, not per-security-group
	// This is handled by ApplySecurityGroupToPort
	return nil
}

// ApplyToInterface applies security group to a network interface
func (m *SecurityGroupManager) ApplyToInterface(interfaceName, securityGroupID string, direction string) error {
	switch m.mode {
	case "stub":
		return m.applyToInterfaceStub(interfaceName, securityGroupID, direction)
	case "iptables":
		return m.applyToInterfaceIPTables(interfaceName, securityGroupID, direction)
	case "ebpf":
		return m.applyToInterfaceEBPF(interfaceName, securityGroupID, direction)
	default:
		return fmt.Errorf("unsupported security group mode: %s", m.mode)
	}
}

// applyToInterfaceStub simulates applying security group to interface
func (m *SecurityGroupManager) applyToInterfaceStub(interfaceName, securityGroupID string, direction string) error {
	return nil // Stub mode - no-op
}

// applyToInterfaceIPTables applies security group to interface using iptables
func (m *SecurityGroupManager) applyToInterfaceIPTables(interfaceName, securityGroupID string, direction string) error {
	chainName := sgChainName(securityGroupID)

	var ifaceFlag string
	if direction == "ingress" {
		ifaceFlag = "-i"
	} else {
		ifaceFlag = "-o"
	}

	ruleSpec := []string{ifaceFlag, interfaceName, "-j", chainName}

	if err := m.ipt.AppendUnique("filter", "FORWARD", ruleSpec...); err != nil {
		return fmt.Errorf("failed to apply security group to interface: %w", err)
	}

	return nil
}

// applyToInterfaceEBPF applies security group to interface using eBPF
func (m *SecurityGroupManager) applyToInterfaceEBPF(interfaceName, securityGroupID string, direction string) error {
	// Attach XDP program to interface if not already attached
	if err := m.ebpfMgr.AttachToInterface(interfaceName); err != nil {
		// Ignore "already attached" errors
		if err.Error() != fmt.Sprintf("XDP program already attached to %s", interfaceName) {
			return fmt.Errorf("failed to attach XDP to %s: %w", interfaceName, err)
		}
	}
	return nil
}

// RemoveFromInterface removes security group from interface
func (m *SecurityGroupManager) RemoveFromInterface(interfaceName, securityGroupID string, direction string) error {
	switch m.mode {
	case "stub":
		return m.removeFromInterfaceStub(interfaceName, securityGroupID, direction)
	case "iptables":
		return m.removeFromInterfaceIPTables(interfaceName, securityGroupID, direction)
	case "ebpf":
		return m.removeFromInterfaceEBPF(interfaceName, securityGroupID, direction)
	default:
		return fmt.Errorf("unsupported security group mode: %s", m.mode)
	}
}

// removeFromInterfaceStub simulates removing security group from interface
func (m *SecurityGroupManager) removeFromInterfaceStub(interfaceName, securityGroupID string, direction string) error {
	return nil // Stub mode - no-op
}

// removeFromInterfaceIPTables removes security group from interface using iptables
func (m *SecurityGroupManager) removeFromInterfaceIPTables(interfaceName, securityGroupID string, direction string) error {
	chainName := "O3K-SG-" + securityGroupID[:8]

	var baseChain string
	var ifaceFlag string

	if direction == "ingress" {
		baseChain = "INPUT"
		ifaceFlag = "-i"
	} else {
		baseChain = "OUTPUT"
		ifaceFlag = "-o"
	}

	ruleSpec := []string{ifaceFlag, interfaceName, "-j", chainName}

	return m.ipt.Delete("filter", baseChain, ruleSpec...)
}

// removeFromInterfaceEBPF removes security group from interface using eBPF
func (m *SecurityGroupManager) removeFromInterfaceEBPF(interfaceName, securityGroupID string, direction string) error {
	// eBPF programs remain attached to interface
	// Rules are removed by updating port rule sets
	return nil
}

// buildRuleSpec builds iptables rule specification
func (m *SecurityGroupManager) buildRuleSpec(rule SecurityGroupRule) []string {
	spec := []string{}

	// Protocol
	if rule.Protocol != "" {
		spec = append(spec, "-p", rule.Protocol)
	}

	// Port range
	if rule.PortRangeMin > 0 {
		if rule.Protocol == "tcp" || rule.Protocol == "udp" {
			if rule.PortRangeMin == rule.PortRangeMax {
				spec = append(spec, "--dport", fmt.Sprintf("%d", rule.PortRangeMin))
			} else {
				spec = append(spec, "--dport", fmt.Sprintf("%d:%d", rule.PortRangeMin, rule.PortRangeMax))
			}
		}
	}

	// Source/destination IP
	if rule.RemoteIPPrefix != "" {
		if rule.Direction == "ingress" {
			spec = append(spec, "-s", rule.RemoteIPPrefix)
		} else {
			spec = append(spec, "-d", rule.RemoteIPPrefix)
		}
	}

	// ICMP type (if applicable)
	if rule.Protocol == "icmp" && rule.PortRangeMin > 0 {
		spec = append(spec, "--icmp-type", fmt.Sprintf("%d", rule.PortRangeMin))
	}

	// Action: ACCEPT
	spec = append(spec, "-j", "ACCEPT")

	return spec
}

// ListRules lists all rules in a security group chain
func (m *SecurityGroupManager) ListRules(securityGroupID string) ([]string, error) {
	chainName := "O3K-SG-" + securityGroupID[:8]

	rules, err := m.ipt.List("filter", chainName)
	if err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}

	return rules, nil
}

// FlushRules removes all rules from a security group chain
func (m *SecurityGroupManager) FlushRules(securityGroupID string) error {
	chainName := "O3K-SG-" + securityGroupID[:8]

	return m.ipt.ClearChain("filter", chainName)
}
