package networking

import (
	"fmt"
	"os/exec"
	"strings"
)

// RouterManager handles L3 router namespace operations
type RouterManager struct {
	mode string // "stub" or "real"
}

// NewRouterManager creates a new router manager
func NewRouterManager(mode string) *RouterManager {
	return &RouterManager{
		mode: mode,
	}
}

// CreateRouterNamespace creates a dedicated network namespace for a router
func (rm *RouterManager) CreateRouterNamespace(routerID string) error {
	if rm.mode == "stub" {
		return nil // No-op in stub mode
	}

	nsName := rm.GetRouterNamespaceName(routerID)

	// Check if namespace already exists
	if rm.namespaceExists(nsName) {
		return nil
	}

	// Create namespace
	cmd := exec.Command("ip", "netns", "add", nsName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create router namespace %s: %w, output: %s", nsName, err, output)
	}

	// Enable IP forwarding in the namespace
	if err := rm.enableIPForwarding(nsName); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}

	return nil
}

// DeleteRouterNamespace deletes a router's network namespace
func (rm *RouterManager) DeleteRouterNamespace(routerID string) error {
	if rm.mode == "stub" {
		return nil
	}

	nsName := rm.GetRouterNamespaceName(routerID)

	if !rm.namespaceExists(nsName) {
		return nil
	}

	cmd := exec.Command("ip", "netns", "del", nsName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete router namespace %s: %w, output: %s", nsName, err, output)
	}

	return nil
}

// GetRouterNamespaceName returns the namespace name for a router
func (rm *RouterManager) GetRouterNamespaceName(routerID string) string {
	id := routerID
	if len(id) > 11 {
		id = id[:11]
	}
	return "qrouter-" + id
}

// AttachInterfaceToRouter creates a veth pair and attaches one end to the router namespace
func (rm *RouterManager) AttachInterfaceToRouter(routerID, interfaceName, ipAddress, cidr string) error {
	if rm.mode == "stub" {
		return nil
	}

	nsName := rm.GetRouterNamespaceName(routerID)
	ifName := interfaceName
	if len(ifName) > 9 {
		ifName = ifName[:9]
	}
	vethPeer := "qr-" + ifName

	// Create veth pair
	cmd := exec.Command("ip", "link", "add", interfaceName, "type", "veth", "peer", "name", vethPeer)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create veth pair: %w, output: %s", err, output)
	}

	// Move router-side interface to router namespace
	cmd = exec.Command("ip", "link", "set", vethPeer, "netns", nsName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to move interface to namespace: %w, output: %s", err, output)
	}

	// Configure IP address in router namespace
	cmd = exec.Command("ip", "netns", "exec", nsName, "ip", "addr", "add", ipAddress+"/"+cidr, "dev", vethPeer)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add IP to router interface: %w, output: %s", err, output)
	}

	// Bring up interfaces
	cmd = exec.Command("ip", "link", "set", interfaceName, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up interface: %w, output: %s", err, output)
	}

	cmd = exec.Command("ip", "netns", "exec", nsName, "ip", "link", "set", vethPeer, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up router interface: %w, output: %s", err, output)
	}

	return nil
}

// DetachInterfaceFromRouter removes an interface from the router namespace
func (rm *RouterManager) DetachInterfaceFromRouter(routerID, interfaceName string) error {
	if rm.mode == "stub" {
		return nil
	}

	// Delete the veth pair (deleting one end deletes both)
	cmd := exec.Command("ip", "link", "del", interfaceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Interface might not exist, which is fine
		if !strings.Contains(string(output), "Cannot find device") {
			return fmt.Errorf("failed to delete interface: %w, output: %s", err, output)
		}
	}

	return nil
}

// AddRoute adds a static route in the router namespace
func (rm *RouterManager) AddRoute(routerID, destination, nexthop string) error {
	if rm.mode == "stub" {
		return nil
	}

	nsName := rm.GetRouterNamespaceName(routerID)

	cmd := exec.Command("ip", "netns", "exec", nsName, "ip", "route", "add", destination, "via", nexthop)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Route might already exist
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("failed to add route: %w, output: %s", err, output)
		}
	}

	return nil
}

// DeleteRoute removes a static route from the router namespace
func (rm *RouterManager) DeleteRoute(routerID, destination string) error {
	if rm.mode == "stub" {
		return nil
	}

	nsName := rm.GetRouterNamespaceName(routerID)

	cmd := exec.Command("ip", "netns", "exec", nsName, "ip", "route", "del", destination)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Route might not exist
		if !strings.Contains(string(output), "No such process") {
			return fmt.Errorf("failed to delete route: %w, output: %s", err, output)
		}
	}

	return nil
}

// SetDefaultGateway sets the default gateway in the router namespace
func (rm *RouterManager) SetDefaultGateway(routerID, gatewayIP string) error {
	if rm.mode == "stub" {
		return nil
	}

	nsName := rm.GetRouterNamespaceName(routerID)

	// Delete existing default route if any
	cmd := exec.Command("ip", "netns", "exec", nsName, "ip", "route", "del", "default")
	_ = cmd.Run() // Ignore error - route might not exist

	// Add new default route
	cmd = exec.Command("ip", "netns", "exec", nsName, "ip", "route", "add", "default", "via", gatewayIP)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set default gateway: %w, output: %s", err, output)
	}

	return nil
}

// EnableSNAT enables source NAT (masquerading) for outbound traffic from internal subnets
func (rm *RouterManager) EnableSNAT(routerID, externalInterface, internalCIDR string) error {
	if rm.mode == "stub" {
		return nil
	}

	nsName := rm.GetRouterNamespaceName(routerID)

	// Add SNAT rule using iptables MASQUERADE
	cmd := exec.Command("ip", "netns", "exec", nsName,
		"iptables", "-t", "nat", "-A", "POSTROUTING",
		"-s", internalCIDR,
		"-o", externalInterface,
		"-j", "MASQUERADE")

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to enable SNAT: %w, output: %s", err, output)
	}

	return nil
}

// DisableSNAT removes source NAT rules
func (rm *RouterManager) DisableSNAT(routerID, externalInterface, internalCIDR string) error {
	if rm.mode == "stub" {
		return nil
	}

	nsName := rm.GetRouterNamespaceName(routerID)

	cmd := exec.Command("ip", "netns", "exec", nsName,
		"iptables", "-t", "nat", "-D", "POSTROUTING",
		"-s", internalCIDR,
		"-o", externalInterface,
		"-j", "MASQUERADE")

	if output, err := cmd.CombinedOutput(); err != nil {
		// Rule might not exist
		if !strings.Contains(string(output), "No chain/target/match") {
			return fmt.Errorf("failed to disable SNAT: %w, output: %s", err, output)
		}
	}

	return nil
}

// AddFloatingIP adds a DNAT rule for a floating IP
func (rm *RouterManager) AddFloatingIP(routerID, floatingIP, fixedIP, externalInterface string) error {
	if rm.mode == "stub" {
		return nil
	}

	nsName := rm.GetRouterNamespaceName(routerID)

	// DNAT: Incoming traffic to floating IP -> fixed IP
	cmd := exec.Command("ip", "netns", "exec", nsName,
		"iptables", "-t", "nat", "-A", "PREROUTING",
		"-d", floatingIP,
		"-i", externalInterface,
		"-j", "DNAT", "--to-destination", fixedIP)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add DNAT rule: %w, output: %s", err, output)
	}

	// SNAT: Outgoing traffic from fixed IP -> floating IP (for return traffic)
	cmd = exec.Command("ip", "netns", "exec", nsName,
		"iptables", "-t", "nat", "-A", "POSTROUTING",
		"-s", fixedIP,
		"-o", externalInterface,
		"-j", "SNAT", "--to-source", floatingIP)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add SNAT rule for floating IP: %w, output: %s", err, output)
	}

	return nil
}

// RemoveFloatingIP removes DNAT/SNAT rules for a floating IP
func (rm *RouterManager) RemoveFloatingIP(routerID, floatingIP, fixedIP, externalInterface string) error {
	if rm.mode == "stub" {
		return nil
	}

	nsName := rm.GetRouterNamespaceName(routerID)

	// Remove DNAT rule
	cmd := exec.Command("ip", "netns", "exec", nsName,
		"iptables", "-t", "nat", "-D", "PREROUTING",
		"-d", floatingIP,
		"-i", externalInterface,
		"-j", "DNAT", "--to-destination", fixedIP)
	_ = cmd.Run() // Ignore error

	// Remove SNAT rule
	cmd = exec.Command("ip", "netns", "exec", nsName,
		"iptables", "-t", "nat", "-D", "POSTROUTING",
		"-s", fixedIP,
		"-o", externalInterface,
		"-j", "SNAT", "--to-source", floatingIP)
	_ = cmd.Run() // Ignore error

	return nil
}

// AddPortForwarding adds a DNAT rule for a specific port forwarding
// External traffic to floatingIP:externalPort is forwarded to fixedIP:internalPort
func (rm *RouterManager) AddPortForwarding(routerID, floatingIP string, externalPort int,
	fixedIP string, internalPort int, protocol, externalInterface string) error {

	if rm.mode == "stub" {
		return nil // No-op in stub mode
	}

	nsName := rm.GetRouterNamespaceName(routerID)

	// DNAT: Incoming traffic to floatingIP:externalPort -> fixedIP:internalPort
	cmd := exec.Command("ip", "netns", "exec", nsName,
		"iptables", "-t", "nat", "-A", "PREROUTING",
		"-d", floatingIP,
		"-i", externalInterface,
		"-p", protocol,
		"--dport", fmt.Sprintf("%d", externalPort),
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("%s:%d", fixedIP, internalPort))

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add port forwarding DNAT rule: %w, output: %s", err, output)
	}

	return nil
}

// RemovePortForwarding removes a DNAT rule for a specific port forwarding
func (rm *RouterManager) RemovePortForwarding(routerID, floatingIP string, externalPort int,
	fixedIP string, internalPort int, protocol, externalInterface string) error {

	if rm.mode == "stub" {
		return nil
	}

	nsName := rm.GetRouterNamespaceName(routerID)

	// Remove DNAT rule
	cmd := exec.Command("ip", "netns", "exec", nsName,
		"iptables", "-t", "nat", "-D", "PREROUTING",
		"-d", floatingIP,
		"-i", externalInterface,
		"-p", protocol,
		"--dport", fmt.Sprintf("%d", externalPort),
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("%s:%d", fixedIP, internalPort))

	_ = cmd.Run() // Ignore error (rule may not exist)

	return nil
}

// Helper functions

func (rm *RouterManager) namespaceExists(nsName string) bool {
	cmd := exec.Command("ip", "netns", "list")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == nsName {
			return true
		}
	}
	return false
}

func (rm *RouterManager) enableIPForwarding(nsName string) error {
	// Enable IPv4 forwarding
	cmd := exec.Command("ip", "netns", "exec", nsName,
		"sysctl", "-w", "net.ipv4.ip_forward=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to enable IPv4 forwarding: %w, output: %s", err, output)
	}

	// Disable reverse path filtering (required for NAT)
	cmd = exec.Command("ip", "netns", "exec", nsName,
		"sysctl", "-w", "net.ipv4.conf.all.rp_filter=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to disable rp_filter: %w, output: %s", err, output)
	}

	return nil
}
