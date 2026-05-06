package networking

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/vishvananda/netlink"
)

// NetworkNamespaceManager manages Linux network namespaces
type NetworkNamespaceManager struct {
	nsPrefix   string
	mode       string // "stub", "iptables", or "ebpf"
	mu         sync.Mutex
	stubNS     map[string]bool // For stub mode
}

// NewNetworkNamespaceManager creates a new namespace manager
func NewNetworkNamespaceManager(mode string) *NetworkNamespaceManager {
	// Ensure /var/run/netns directory exists for real mode
	if mode != "stub" {
		if err := os.MkdirAll("/var/run/netns", 0755); err != nil {
			log.Printf("Warning: Failed to create /var/run/netns directory: %v", err)
		}
	}

	return &NetworkNamespaceManager{
		nsPrefix: "light-ns-",
		mode:     mode,
		stubNS:   make(map[string]bool),
	}
}

// CreateNamespace creates a network namespace for a project
func (m *NetworkNamespaceManager) CreateNamespace(projectID string) error {
	if m.mode == "stub" {
		return m.createNamespaceStub(projectID)
	}
	// Both iptables and eBPF use real namespaces
	return m.createNamespaceReal(projectID)
}

// createNamespaceStub simulates namespace creation
func (m *NetworkNamespaceManager) createNamespaceStub(projectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nsName := m.nsPrefix + projectID
	m.stubNS[nsName] = true
	return nil
}

// createNamespaceReal creates an actual namespace
func (m *NetworkNamespaceManager) createNamespaceReal(projectID string) error {
	nsName := m.nsPrefix + projectID

	// Check if namespace already exists
	if m.NamespaceExists(nsName) {
		return nil // Already exists
	}

	// Create namespace using ip netns add
	cmd := exec.Command("ip", "netns", "add", nsName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", nsName, err)
	}

	return nil
}

// DeleteNamespace deletes a network namespace
func (m *NetworkNamespaceManager) DeleteNamespace(projectID string) error {
	if m.mode == "stub" {
		return m.deleteNamespaceStub(projectID)
	}
	// Both iptables and eBPF use real namespaces
	return m.deleteNamespaceReal(projectID)
}

// deleteNamespaceStub simulates namespace deletion
func (m *NetworkNamespaceManager) deleteNamespaceStub(projectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nsName := m.nsPrefix + projectID
	delete(m.stubNS, nsName)
	return nil
}

// deleteNamespaceReal deletes an actual namespace
func (m *NetworkNamespaceManager) deleteNamespaceReal(projectID string) error {
	nsName := m.nsPrefix + projectID

	if !m.NamespaceExists(nsName) {
		return nil // Already deleted
	}

	cmd := exec.Command("ip", "netns", "delete", nsName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete namespace %s: %w", nsName, err)
	}

	return nil
}

// NamespaceExists checks if a namespace exists
func (m *NetworkNamespaceManager) NamespaceExists(nsName string) bool {
	if m.mode == "stub" {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.stubNS[nsName]
	}

	cmd := exec.Command("ip", "netns", "list")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("DEBUG: NamespaceExists check failed for %s, error running 'ip netns list': %v", nsName, err)
		return false
	}

	log.Printf("DEBUG: Checking if namespace %s exists, output: %q", nsName, string(output))
	namespaces := strings.Split(string(output), "\n")
	for _, ns := range namespaces {
		fields := strings.Fields(ns)
		if len(fields) > 0 && fields[0] == nsName {
			log.Printf("DEBUG: Namespace %s found in list", nsName)
			return true
		}
	}

	log.Printf("DEBUG: Namespace %s not found in list", nsName)
	return false
}

// GetNamespaceName returns the namespace name for a project
func (m *NetworkNamespaceManager) GetNamespaceName(projectID string) string {
	return m.nsPrefix + projectID
}

// EnsureNamespaceExists checks if namespace exists and creates it if not
func (m *NetworkNamespaceManager) EnsureNamespaceExists(projectID string) error {
	nsName := m.GetNamespaceName(projectID)

	if m.mode == "stub" {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.stubNS[nsName] = true
		return nil
	}

	log.Printf("DEBUG: EnsureNamespaceExists called for project %s, namespace %s", projectID, nsName)

	// Check if namespace exists
	if m.NamespaceExists(nsName) {
		log.Printf("DEBUG: Namespace %s already exists", nsName)
		return nil
	}

	log.Printf("DEBUG: Namespace %s does not exist, creating it", nsName)
	// Create namespace if it doesn't exist
	err := m.CreateNamespace(projectID)
	if err != nil {
		log.Printf("ERROR: Failed to create namespace %s: %v", nsName, err)
		return err
	}
	log.Printf("DEBUG: Successfully created namespace %s", nsName)
	return nil
}

// ExecInNamespace executes a command in a namespace
func (m *NetworkNamespaceManager) ExecInNamespace(projectID string, args ...string) error {
	if m.mode == "stub" {
		// In stub mode, just check if namespace exists
		nsName := m.GetNamespaceName(projectID)
		if !m.NamespaceExists(nsName) {
			return fmt.Errorf("namespace %s does not exist", nsName)
		}
		return nil
	}

	nsName := m.GetNamespaceName(projectID)
	fullArgs := append([]string{"netns", "exec", nsName}, args...)
	cmd := exec.Command("ip", fullArgs...)
	return cmd.Run()
}

// BridgeManager manages Linux bridges
type BridgeManager struct{
	mode      string // "stub", "iptables", or "ebpf"
	mu        sync.Mutex
	stubBridges map[string]bool // For stub mode
}

// NewBridgeManager creates a new bridge manager
func NewBridgeManager(mode string) *BridgeManager {
	return &BridgeManager{
		mode:        mode,
		stubBridges: make(map[string]bool),
	}
}

// CreateBridge creates a bridge in the default or specified namespace
func (m *BridgeManager) CreateBridge(bridgeName string, inNamespace bool, nsName string) error {
	if m.mode == "stub" {
		return m.createBridgeStub(bridgeName)
	}
	// Both iptables and eBPF use real bridges
	return m.createBridgeReal(bridgeName, inNamespace, nsName)
}

// createBridgeStub simulates bridge creation
func (m *BridgeManager) createBridgeStub(bridgeName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stubBridges[bridgeName] = true
	return nil
}

// createBridgeReal creates an actual bridge
func (m *BridgeManager) createBridgeReal(bridgeName string, inNamespace bool, nsName string) error {
	if inNamespace {
		// Create bridge in namespace using ip command
		cmd := exec.Command("ip", "netns", "exec", nsName, "ip", "link", "add", bridgeName, "type", "bridge")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create bridge %s in namespace %s: %w", bridgeName, nsName, err)
		}

		// Bring bridge up
		cmd = exec.Command("ip", "netns", "exec", nsName, "ip", "link", "set", bridgeName, "up")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to bring up bridge %s: %w", bridgeName, err)
		}
	} else {
		// Create bridge in default namespace using netlink
		bridge := &netlink.Bridge{
			LinkAttrs: netlink.LinkAttrs{
				Name: bridgeName,
			},
		}

		if err := netlink.LinkAdd(bridge); err != nil {
			if !strings.Contains(err.Error(), "exists") {
				return fmt.Errorf("failed to create bridge %s: %w", bridgeName, err)
			}
		}

		// Bring bridge up
		link, err := netlink.LinkByName(bridgeName)
		if err != nil {
			return fmt.Errorf("failed to find bridge %s: %w", bridgeName, err)
		}

		if err := netlink.LinkSetUp(link); err != nil {
			return fmt.Errorf("failed to bring up bridge %s: %w", bridgeName, err)
		}
	}

	return nil
}

// BridgeExists reports whether a bridge with the given name exists.
func (m *BridgeManager) BridgeExists(name string) bool {
	if m.mode == "stub" {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.stubBridges[name]
	}
	_, err := netlink.LinkByName(name)
	return err == nil
}

// DeleteBridge deletes a bridge
func (m *BridgeManager) DeleteBridge(bridgeName string, inNamespace bool, nsName string) error {
	if m.mode == "stub" {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.stubBridges, bridgeName)
		return nil
	}

	if inNamespace {
		cmd := exec.Command("ip", "netns", "exec", nsName, "ip", "link", "delete", bridgeName)
		return cmd.Run()
	}

	link, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return nil // Already deleted
	}

	return netlink.LinkDel(link)
}

// AttachToBridge attaches an interface to a bridge
func (m *BridgeManager) AttachToBridge(ifName, bridgeName string, inNamespace bool, nsName string) error {
	if m.mode == "stub" {
		// Just verify bridge exists in stub mode
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.stubBridges[bridgeName] {
			return fmt.Errorf("bridge %s does not exist", bridgeName)
		}
		return nil
	}

	if inNamespace {
		cmd := exec.Command("ip", "netns", "exec", nsName, "ip", "link", "set", ifName, "master", bridgeName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to attach %s to bridge %s in namespace %s: %w, output: %s", ifName, bridgeName, nsName, err, string(output))
		}
		return nil
	}

	iface, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to find interface %s: %w", ifName, err)
	}

	bridge, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return fmt.Errorf("failed to find bridge %s: %w", bridgeName, err)
	}

	return netlink.LinkSetMaster(iface, bridge.(*netlink.Bridge))
}

// TAPDeviceManager manages TAP devices
type TAPDeviceManager struct{
	mode    string // "stub", "iptables", or "ebpf"
	mu      sync.Mutex
	stubTAPs map[string]bool // For stub mode
}

// NewTAPDeviceManager creates a new TAP device manager
func NewTAPDeviceManager(mode string) *TAPDeviceManager {
	return &TAPDeviceManager{
		mode:     mode,
		stubTAPs: make(map[string]bool),
	}
}

// CreateTAPDevice creates a TAP device
func (m *TAPDeviceManager) CreateTAPDevice(tapName string, inNamespace bool, nsName string) error {
	if m.mode == "stub" {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.stubTAPs[tapName] = true
		return nil
	}

	// Both iptables and eBPF use real TAP devices
	if inNamespace {
		cmd := exec.Command("ip", "netns", "exec", nsName, "ip", "tuntap", "add", tapName, "mode", "tap")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to create TAP device %s in namespace %s: %w, output: %s", tapName, nsName, err, string(output))
		}

		// Bring TAP device up
		cmd = exec.Command("ip", "netns", "exec", nsName, "ip", "link", "set", tapName, "up")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to bring up TAP device %s in namespace %s: %w, output: %s", tapName, nsName, err, string(output))
		}
		return nil
	}

	// Create TAP device in default namespace
	cmd := exec.Command("ip", "tuntap", "add", tapName, "mode", "tap")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create TAP device %s: %w", tapName, err)
	}

	// Bring TAP device up
	cmd = exec.Command("ip", "link", "set", tapName, "up")
	return cmd.Run()
}

// DeleteTAPDevice deletes a TAP device
func (m *TAPDeviceManager) DeleteTAPDevice(tapName string, inNamespace bool, nsName string) error {
	if m.mode == "stub" {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.stubTAPs, tapName)
		return nil
	}

	if inNamespace {
		cmd := exec.Command("ip", "netns", "exec", nsName, "ip", "link", "delete", tapName)
		return cmd.Run()
	}

	cmd := exec.Command("ip", "link", "delete", tapName)
	return cmd.Run()
}

// MoveTAPToNamespace moves a TAP device to a namespace
func (m *TAPDeviceManager) MoveTAPToNamespace(tapName, nsName string) error {
	if m.mode == "stub" {
		// Just verify TAP exists in stub mode
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.stubTAPs[tapName] {
			return fmt.Errorf("TAP device %s does not exist", tapName)
		}
		return nil
	}

	cmd := exec.Command("ip", "link", "set", tapName, "netns", nsName)
	return cmd.Run()
}
