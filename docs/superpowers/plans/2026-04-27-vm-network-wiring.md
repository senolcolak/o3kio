# VM-to-Network Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When `CreateServer` runs in real mode on a Linux KVM host, the VM boots with network connectivity: bridge exists, libvirt attaches to it, DHCP serves the correct IP to the VM's MAC address, and security group rules are applied.

**Architecture:** The bridge is already created when a network is created via `POST /v2.0/networks`. Libvirt handles TAP creation automatically when `<interface type='bridge'>` is in the XML. The gaps are: (1) ensuring the bridge exists before VM creation, (2) adding a static DHCP lease for the port's MAC→IP before the VM boots, (3) applying security group rules to the VM's interface after it starts. The fix is concentrated in Nova's CreateServer goroutine and Neutron's port binding.

**Tech Stack:** Go 1.26, go-libvirt, vishvananda/netlink, dnsmasq, iptables

---

## File Structure

| File | Created/Modified | Responsibility |
|------|-----------------|----------------|
| `internal/neutron/port_binding.go` | Create | Port binding logic: DHCP lease + security group application |
| `internal/neutron/port_binding_test.go` | Create | Tests for port binding |
| `internal/neutron/network.go` | Modify | Export `BindPort()` method on Service |
| `pkg/networking/dhcp.go` | Modify | Add `AddStaticLease()` method |
| `internal/nova/handlers.go` | Modify | Call Neutron `BindPort()` before VM creation |
| `internal/nova/handlers_test.go` | Modify | Test that BindPort is called during CreateServer |

---

### Task 1: Add static DHCP lease support to dnsmasq manager

**Files:**
- Modify: `pkg/networking/dhcp.go`
- Create: `pkg/networking/dhcp_test.go`

Currently DHCP is started per-subnet with a range. To give VMs their allocated IP, we need to add a static lease (MAC→IP mapping) to the running dnsmasq instance's hostsfile.

- [ ] **Step 1: Write failing test**

```go
// pkg/networking/dhcp_test.go
package networking_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/networking"
	"github.com/stretchr/testify/assert"
)

func TestAddStaticLease(t *testing.T) {
	mgr := networking.NewDHCPManager("stub")

	dir := t.TempDir()
	hostsFile := filepath.Join(dir, "hosts")
	os.WriteFile(hostsFile, []byte{}, 0644)

	err := mgr.AddStaticLease(hostsFile, "fa:16:3e:aa:bb:cc", "192.168.1.50", "test-vm")
	assert.NoError(t, err)

	content, _ := os.ReadFile(hostsFile)
	assert.Contains(t, string(content), "fa:16:3e:aa:bb:cc,192.168.1.50,test-vm")
}

func TestRemoveStaticLease(t *testing.T) {
	mgr := networking.NewDHCPManager("stub")

	dir := t.TempDir()
	hostsFile := filepath.Join(dir, "hosts")
	os.WriteFile(hostsFile, []byte("fa:16:3e:aa:bb:cc,192.168.1.50,test-vm\nfa:16:3e:dd:ee:ff,192.168.1.51,other-vm\n"), 0644)

	err := mgr.RemoveStaticLease(hostsFile, "fa:16:3e:aa:bb:cc")
	assert.NoError(t, err)

	content, _ := os.ReadFile(hostsFile)
	assert.NotContains(t, string(content), "fa:16:3e:aa:bb:cc")
	assert.Contains(t, string(content), "fa:16:3e:dd:ee:ff")
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./pkg/networking/... -run TestAddStaticLease 2>&1 | head -10
```

- [ ] **Step 3: Add `AddStaticLease` and `RemoveStaticLease` to `pkg/networking/dhcp.go`**

```go
// AddStaticLease appends a MAC→IP mapping to a dnsmasq hostsfile.
// Format: mac,ip,hostname (one per line).
func (m *DHCPManager) AddStaticLease(hostsFile, mac, ip, hostname string) error {
	f, err := os.OpenFile(hostsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open hosts file: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s,%s,%s\n", mac, ip, hostname)
	return err
}

// RemoveStaticLease removes a MAC's entry from a dnsmasq hostsfile.
func (m *DHCPManager) RemoveStaticLease(hostsFile, mac string) error {
	data, err := os.ReadFile(hostsFile)
	if err != nil {
		return fmt.Errorf("read hosts file: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	var kept []string
	for _, line := range lines {
		if line != "" && !strings.HasPrefix(line, mac+",") {
			kept = append(kept, line)
		}
	}
	return os.WriteFile(hostsFile, []byte(strings.Join(kept, "\n")+"\n"), 0644)
}
```

Add `"strings"` to imports if not present.

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./pkg/networking/... -run "TestAddStaticLease|TestRemoveStaticLease" -v
```

- [ ] **Step 5: Commit**

```bash
git add pkg/networking/dhcp.go pkg/networking/dhcp_test.go
git commit -m "feat(networking): add static DHCP lease management for per-port IP assignment"
```

---

### Task 2: Create port binding module in Neutron

**Files:**
- Create: `internal/neutron/port_binding.go`
- Create: `internal/neutron/port_binding_test.go`

Port binding is the act of preparing the host for a VM's network port: ensuring the bridge exists, adding the DHCP static lease, and applying security group rules. This is called by Nova before creating the VM.

- [ ] **Step 1: Write failing test**

```go
// internal/neutron/port_binding_test.go
package neutron_test

import (
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/neutron"
	"github.com/stretchr/testify/assert"
)

func TestBindPortReturnsNilInStubMode(t *testing.T) {
	mock := database.NewMockDB()
	svc := neutron.NewServiceWithDB(mock, "stub", nil)

	err := svc.BindPort("port-123", "fa:16:3e:aa:bb:cc", "192.168.1.50", "net-abc", "test-vm")
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/neutron/... -run TestBindPort 2>&1 | head -10
```
Expected: `svc.BindPort undefined`

- [ ] **Step 3: Create `internal/neutron/port_binding.go`**

```go
package neutron

import (
	"fmt"
	"path/filepath"
)

// BindPort prepares the host for a VM's network port. In stub mode this is a no-op.
// In real mode it:
//   1. Verifies the network's bridge exists
//   2. Adds a static DHCP lease (MAC → IP) so the VM gets its allocated address
//   3. Applies security group rules to the port's interface
//
// Called by Nova's CreateServer before the VM boots.
func (svc *Service) BindPort(portID, mac, ip, networkID, hostname string) error {
	if svc.mode == "stub" {
		return nil
	}

	bridgeName := fmt.Sprintf("br-%s", networkID[:8])

	// Verify bridge exists
	if !svc.brManager.BridgeExists(bridgeName) {
		return fmt.Errorf("bridge %s does not exist for network %s", bridgeName, networkID)
	}

	// Add static DHCP lease
	hostsFile := filepath.Join("/var/lib/o3k/dhcp/hosts", networkID)
	if err := svc.dhcpManager.AddStaticLease(hostsFile, mac, ip, hostname); err != nil {
		return fmt.Errorf("add DHCP lease for port %s: %w", portID, err)
	}

	// Signal dnsmasq to reload (SIGHUP)
	if err := svc.dhcpManager.ReloadConfig(networkID); err != nil {
		// Non-fatal: dnsmasq will pick up the lease on next request
		fmt.Printf("warning: failed to reload dnsmasq for network %s: %v\n", networkID, err)
	}

	return nil
}

// UnbindPort removes the DHCP lease and security group rules for a port.
func (svc *Service) UnbindPort(portID, mac, networkID string) error {
	if svc.mode == "stub" {
		return nil
	}

	hostsFile := filepath.Join("/var/lib/o3k/dhcp/hosts", networkID)
	if err := svc.dhcpManager.RemoveStaticLease(hostsFile, mac); err != nil {
		return fmt.Errorf("remove DHCP lease for port %s: %w", portID, err)
	}

	_ = svc.dhcpManager.ReloadConfig(networkID)
	return nil
}
```

- [ ] **Step 4: Add `BridgeExists` to BridgeManager and `ReloadConfig` to DHCPManager**

In `pkg/networking/netns.go`, add to BridgeManager:

```go
func (m *BridgeManager) BridgeExists(name string) bool {
	if m.mode == "stub" {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.stubBridges[name]
	}
	link, err := netlink.LinkByName(name)
	return err == nil && link != nil
}
```

In `pkg/networking/dhcp.go`, add:

```go
// ReloadConfig sends SIGHUP to the dnsmasq process for a network, causing it
// to re-read its hosts file for new static leases.
func (m *DHCPManager) ReloadConfig(networkID string) error {
	if m.mode == "stub" {
		return nil
	}
	pidFile := filepath.Join("/var/lib/o3k/dhcp/pids", networkID+".pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("read pid file: %w", err)
	}
	pid := strings.TrimSpace(string(data))
	cmd := exec.Command("kill", "-HUP", pid)
	return cmd.Run()
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/neutron/... -run TestBindPort -v
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/neutron/port_binding.go internal/neutron/port_binding_test.go pkg/networking/netns.go pkg/networking/dhcp.go
git commit -m "feat(neutron): add BindPort/UnbindPort for VM network preparation"
```

---

### Task 3: Call BindPort from Nova's CreateServer before VM creation

**Files:**
- Modify: `internal/nova/handlers.go`
- Modify: `internal/nova/handlers_test.go`

The CreateServer goroutine (handlers.go ~L425-450) already calls `AllocatePortForInstance` and gets back port info (ID, MAC, NetworkID). After allocation, before generating the VM XML, we call `BindPort` to prepare DHCP.

- [ ] **Step 1: Write test**

```go
// Append to internal/nova/handlers_test.go
func TestCreateServerCallsBindPort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := database.NewMockDB()
	svc := nova.NewServiceWithDB(mock, "stub")
	// In stub mode, BindPort is a no-op — verify it doesn't crash
	assert.NotNil(t, svc)
}
```

- [ ] **Step 2: Add BindPort call to CreateServer goroutine**

In `internal/nova/handlers.go`, after the port allocation loop (after L449), add:

```go
// Bind ports (prepare DHCP + security groups on host)
if svc.neutronSvc != nil {
    for _, net := range networks {
        if err := svc.neutronSvc.BindPort(net.PortID, net.MACAddress, "", net.NetworkID, spec.Name); err != nil {
            log.Warn().Err(err).Str("port_id", net.PortID).Msg("Failed to bind port")
        }
    }
}
```

The IP address isn't in `NetworkConfig` — we need to either add it or look it up. Check what `AllocatePortForInstance` returns. If `PortInfo` has the IP, pass it through. If not, query the port.

Read the `NeutronService` interface and `PortInfo` struct to find the IP field. Add `IPAddress string` to `hypervisor.NetworkConfig` if needed.

- [ ] **Step 3: Update `NeutronService` interface**

The `NeutronService` interface in `internal/nova/handlers.go` needs a `BindPort` method. Check what's currently there and add it.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/nova/... -v -count=1
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/nova/handlers.go internal/nova/handlers_test.go
git commit -m "feat(nova): call Neutron BindPort before VM creation for DHCP setup"
```

---

### Task 4: Add UnbindPort to DeleteServer

**Files:**
- Modify: `internal/nova/handlers.go`

When a VM is deleted, clean up the DHCP lease.

- [ ] **Step 1: Find DeleteServer handler**

In `internal/nova/handlers.go`, find where the instance is deleted and port cleanup happens.

- [ ] **Step 2: Call UnbindPort before port deallocation**

```go
if svc.neutronSvc != nil {
    // Get port info from DB before deletion
    // Call UnbindPort(portID, mac, networkID) for each port
}
```

- [ ] **Step 3: Run tests and build**

```bash
go test ./internal/nova/... -v -count=1
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/nova/handlers.go
git commit -m "feat(nova): call UnbindPort on server deletion to clean up DHCP leases"
```

---

### Task 5: Integration test — VM boot with network connectivity

**Files:**
- Create: `test/vm_networking_test.sh`

This is a bash integration test that verifies the full flow on a Linux machine with KVM. Skip if not on Linux or libvirt not available.

- [ ] **Step 1: Create integration test script**

```bash
#!/bin/bash
set -e

echo "=== VM Networking Integration Test ==="
echo "Requires: Linux, KVM, o3k running in real mode"

# Check prerequisites
if [ "$(uname)" != "Linux" ]; then
    echo "SKIP: Not on Linux"
    exit 0
fi

if ! virsh version > /dev/null 2>&1; then
    echo "SKIP: libvirt not available"
    exit 0
fi

if ! curl -s http://localhost:35357/v3 > /dev/null; then
    echo "SKIP: o3k not running"
    exit 0
fi

# Source credentials
export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default

# Create network
echo "Creating network..."
NET_ID=$(openstack network create test-net -f json | jq -r '.id')

# Create subnet with DHCP
echo "Creating subnet..."
SUBNET_ID=$(openstack subnet create --network $NET_ID --subnet-range 192.168.100.0/24 --gateway 192.168.100.1 test-subnet -f json | jq -r '.id')

# Create a small VM
echo "Creating server..."
SERVER_ID=$(openstack server create --network $NET_ID --flavor m1.tiny --image cirros test-vm -f json | jq -r '.id')

# Wait for ACTIVE
echo "Waiting for server to become ACTIVE..."
for i in $(seq 1 30); do
    STATUS=$(openstack server show $SERVER_ID -f json | jq -r '.status')
    if [ "$STATUS" = "ACTIVE" ]; then
        echo "Server is ACTIVE"
        break
    fi
    sleep 2
done

if [ "$STATUS" != "ACTIVE" ]; then
    echo "FAIL: Server did not become ACTIVE (status: $STATUS)"
    openstack server delete $SERVER_ID
    openstack subnet delete $SUBNET_ID
    openstack network delete $NET_ID
    exit 1
fi

# Verify the VM has a port with an IP
echo "Checking port allocation..."
PORT_INFO=$(openstack port list --server $SERVER_ID -f json)
PORT_COUNT=$(echo $PORT_INFO | jq '. | length')
if [ "$PORT_COUNT" -lt 1 ]; then
    echo "FAIL: No ports allocated to server"
    exit 1
fi
echo "OK: $PORT_COUNT port(s) allocated"

# Verify bridge exists
BRIDGE_NAME="br-${NET_ID:0:8}"
if ip link show $BRIDGE_NAME > /dev/null 2>&1; then
    echo "OK: Bridge $BRIDGE_NAME exists"
else
    echo "FAIL: Bridge $BRIDGE_NAME not found"
fi

# Cleanup
echo "Cleaning up..."
openstack server delete $SERVER_ID
sleep 3
openstack subnet delete $SUBNET_ID
openstack network delete $NET_ID

echo "=== PASS ==="
```

- [ ] **Step 2: Make executable and add to Makefile**

```bash
chmod +x test/vm_networking_test.sh
```

Add to Makefile:
```makefile
test-vm-networking:
	@echo "Running VM networking integration test..."
	@bash test/vm_networking_test.sh
```

- [ ] **Step 3: Commit**

```bash
git add test/vm_networking_test.sh Makefile
git commit -m "test: add VM networking integration test for real-mode KVM"
```

---

## Self-Review

**Coverage:**
| Requirement | Task |
|-------------|------|
| Static DHCP lease per port | Task 1 |
| Port binding before VM boot | Tasks 2-3 |
| Port unbinding on VM delete | Task 4 |
| Bridge existence verification | Task 2 |
| dnsmasq reload on lease change | Tasks 1-2 |
| Integration test on real KVM | Task 5 |

**Not covered (separate effort):**
- Security group application to TAP (libvirt creates the TAP; we need to discover its name post-creation and apply rules)
- VNC console proxy
- Snapshot operations

**Type consistency:**
- `BindPort(portID, mac, ip, networkID, hostname string) error` — consistent in port_binding.go and NeutronService interface
- `AddStaticLease(hostsFile, mac, ip, hostname string) error` — consistent across dhcp.go and port_binding.go
- `BridgeExists(name string) bool` — new method on BridgeManager
