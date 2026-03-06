package networking

import (
	"fmt"

	"github.com/coreos/go-iptables/iptables"
)

// SecurityGroupManager manages iptables-based security groups
type SecurityGroupManager struct {
	ipt *iptables.IPTables
}

// NewSecurityGroupManager creates a new security group manager
func NewSecurityGroupManager() (*SecurityGroupManager, error) {
	ipt, err := iptables.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize iptables: %w", err)
	}

	return &SecurityGroupManager{
		ipt: ipt,
	}, nil
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

// CreateSecurityGroupChain creates an iptables chain for a security group
func (m *SecurityGroupManager) CreateSecurityGroupChain(securityGroupID string) error {
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
