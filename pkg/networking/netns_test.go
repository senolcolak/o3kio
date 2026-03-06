package networking

import (
	"testing"
)

// Test namespace manager in stub mode
func TestNamespaceManagerStubMode(t *testing.T) {
	mgr := NewNetworkNamespaceManager("stub")

	projectID := "test-project-123"

	// Create namespace
	if err := mgr.CreateNamespace(projectID); err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Verify namespace exists
	nsName := mgr.GetNamespaceName(projectID)
	if !mgr.NamespaceExists(nsName) {
		t.Error("Namespace should exist after creation")
	}

	// Delete namespace
	if err := mgr.DeleteNamespace(projectID); err != nil {
		t.Fatalf("Failed to delete namespace: %v", err)
	}

	// Verify namespace no longer exists
	if mgr.NamespaceExists(nsName) {
		t.Error("Namespace should not exist after deletion")
	}
}

// Test bridge manager in stub mode
func TestBridgeManagerStubMode(t *testing.T) {
	mgr := NewBridgeManager("stub")

	bridgeName := "br-test123"

	// Create bridge
	if err := mgr.CreateBridge(bridgeName, false, ""); err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	// Verify bridge exists
	mgr.mu.Lock()
	exists := mgr.stubBridges[bridgeName]
	mgr.mu.Unlock()

	if !exists {
		t.Error("Bridge should exist after creation")
	}

	// Delete bridge
	if err := mgr.DeleteBridge(bridgeName, false, ""); err != nil {
		t.Fatalf("Failed to delete bridge: %v", err)
	}

	// Verify bridge no longer exists
	mgr.mu.Lock()
	exists = mgr.stubBridges[bridgeName]
	mgr.mu.Unlock()

	if exists {
		t.Error("Bridge should not exist after deletion")
	}
}

// Test TAP device manager in stub mode
func TestTAPManagerStubMode(t *testing.T) {
	mgr := NewTAPDeviceManager("stub")

	tapName := "tap-test123"

	// Create TAP device
	if err := mgr.CreateTAPDevice(tapName, false, ""); err != nil {
		t.Fatalf("Failed to create TAP device: %v", err)
	}

	// Verify TAP device exists
	mgr.mu.Lock()
	exists := mgr.stubTAPs[tapName]
	mgr.mu.Unlock()

	if !exists {
		t.Error("TAP device should exist after creation")
	}

	// Delete TAP device
	if err := mgr.DeleteTAPDevice(tapName, false, ""); err != nil {
		t.Fatalf("Failed to delete TAP device: %v", err)
	}

	// Verify TAP device no longer exists
	mgr.mu.Lock()
	exists = mgr.stubTAPs[tapName]
	mgr.mu.Unlock()

	if exists {
		t.Error("TAP device should not exist after deletion")
	}
}

// Test network lifecycle in stub mode
func TestNetworkLifecycleStub(t *testing.T) {
	nsMgr := NewNetworkNamespaceManager("stub")
	brMgr := NewBridgeManager("stub")
	tapMgr := NewTAPDeviceManager("stub")

	projectID := "project-123"
	networkID := "network-456"
	portID := "port-789"

	// 1. Create namespace for project
	if err := nsMgr.CreateNamespace(projectID); err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// 2. Create bridge for network
	bridgeName := "br-" + networkID[:8]
	if err := brMgr.CreateBridge(bridgeName, false, ""); err != nil {
		t.Fatalf("Failed to create bridge: %v", err)
	}

	// 3. Create TAP device for port
	tapName := "tap-" + portID[:8]
	if err := tapMgr.CreateTAPDevice(tapName, false, ""); err != nil {
		t.Fatalf("Failed to create TAP device: %v", err)
	}

	// 4. Attach TAP to bridge
	if err := brMgr.AttachToBridge(tapName, bridgeName, false, ""); err != nil {
		t.Fatalf("Failed to attach TAP to bridge: %v", err)
	}

	// 5. Cleanup - delete in reverse order
	if err := tapMgr.DeleteTAPDevice(tapName, false, ""); err != nil {
		t.Fatalf("Failed to delete TAP device: %v", err)
	}

	if err := brMgr.DeleteBridge(bridgeName, false, ""); err != nil {
		t.Fatalf("Failed to delete bridge: %v", err)
	}

	if err := nsMgr.DeleteNamespace(projectID); err != nil {
		t.Fatalf("Failed to delete namespace: %v", err)
	}

	// Verify all cleaned up
	nsName := nsMgr.GetNamespaceName(projectID)
	if nsMgr.NamespaceExists(nsName) {
		t.Error("Namespace should be deleted")
	}

	brMgr.mu.Lock()
	bridgeExists := brMgr.stubBridges[bridgeName]
	brMgr.mu.Unlock()
	if bridgeExists {
		t.Error("Bridge should be deleted")
	}

	tapMgr.mu.Lock()
	tapExists := tapMgr.stubTAPs[tapName]
	tapMgr.mu.Unlock()
	if tapExists {
		t.Error("TAP device should be deleted")
	}
}
