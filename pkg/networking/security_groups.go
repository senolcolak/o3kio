package networking

import (
	"fmt"
	"sync"

	"github.com/coreos/go-iptables/iptables"
)

// SecurityGroupManager manages security groups with iptables or eBPF
type SecurityGroupManager struct {
	mode       string // "stub", "iptables", or "ebpf"
	ipt        *iptables.IPTables
	mu         sync.Mutex
	stubChains map[string]bool                // For stub mode
	stubRules  map[string][]SecurityGroupRule // For stub mode
	ebpfProgs  map[string]interface{}         // For eBPF mode (placeholder)
}

// NewSecurityGroupManager creates a new security group manager
func NewSecurityGroupManager(mode string) (*SecurityGroupManager, error) {
	mgr := &SecurityGroupManager{
		mode:       mode,
		stubChains: make(map[string]bool),
		stubRules:  make(map[string][]SecurityGroupRule),
		ebpfProgs:  make(map[string]interface{}),
	}

	if mode == "iptables" {
		ipt, err := iptables.New()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize iptables: %w", err)
		}
		mgr.ipt = ipt
	} else if mode == "ebpf" {
		// eBPF initialization will be added here
		// For now, we'll implement the interface structure
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
	chainName := "O3K-SG-" + securityGroupID[:8]
	m.stubChains[chainName] = true
	return nil
}

// createSecurityGroupChainIPTables creates an iptables chain
func (m *SecurityGroupManager) createSecurityGroupChainIPTables(securityGroupID string) error {
	chainName := "O3K-SG-" + securityGroupID[:8]

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
	m.mu.Lock()
	defer m.mu.Unlock()

	progName := "O3K-SG-" + securityGroupID[:8]

	// TODO: Implement eBPF program loading
	// This will involve:
	// 1. Loading eBPF bytecode
	// 2. Attaching to TC (traffic control) or XDP hook
	// 3. Managing BPF maps for rules
	//
	// For now, we'll create a placeholder that simulates the program
	m.ebpfProgs[progName] = map[string]interface{}{
		"type":   "xdp", // or "tc"
		"action": "drop_by_default",
		"rules":  []SecurityGroupRule{},
	}

	return nil
}

// DeleteSecurityGroupChain deletes an iptables chain
func (m *SecurityGroupManager) DeleteSecurityGroupChain(securityGroupID string) error {
	chainName := "O3K-SG-" + securityGroupID[:8]

	// Flush chain first
	m.ipt.ClearChain("filter", chainName)

	// Delete chain
	return m.ipt.DeleteChain("filter", chainName)
}

// AddRule adds a security group rule
func (m *SecurityGroupManager) AddRule(securityGroupID string, rule SecurityGroupRule) error {
	chainName := "O3K-SG-" + securityGroupID[:8]

	// Build iptables rule
	ruleSpec := m.buildRuleSpec(rule)

	// Insert rule before the final DROP rule
	if err := m.ipt.Insert("filter", chainName, 1, ruleSpec...); err != nil {
		return fmt.Errorf("failed to add rule to chain %s: %w", chainName, err)
	}

	return nil
}

// RemoveRule removes a security group rule
func (m *SecurityGroupManager) RemoveRule(securityGroupID string, rule SecurityGroupRule) error {
	chainName := "O3K-SG-" + securityGroupID[:8]

	ruleSpec := m.buildRuleSpec(rule)

	return m.ipt.Delete("filter", chainName, ruleSpec...)
}

// ApplyToInterface applies security group to a network interface
func (m *SecurityGroupManager) ApplyToInterface(interfaceName, securityGroupID string, direction string) error {
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

	// Add jump rule from INPUT/OUTPUT to security group chain
	ruleSpec := []string{ifaceFlag, interfaceName, "-j", chainName}

	if err := m.ipt.AppendUnique("filter", baseChain, ruleSpec...); err != nil {
		return fmt.Errorf("failed to apply security group to interface: %w", err)
	}

	return nil
}

// RemoveFromInterface removes security group from interface
func (m *SecurityGroupManager) RemoveFromInterface(interfaceName, securityGroupID string, direction string) error {
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
