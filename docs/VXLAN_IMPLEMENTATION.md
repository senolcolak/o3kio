# VXLAN Multi-Node Overlay Networking Implementation

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Components](#components)
4. [Configuration](#configuration)
5. [Database Schema](#database-schema)
6. [Deployment](#deployment)
7. [Troubleshooting](#troubleshooting)
8. [Testing](#testing)

---

## Overview

O3K implements VXLAN (Virtual Extensible LAN) overlay networking to enable L2 connectivity across compute nodes in a multi-node deployment. This allows VMs on different physical hosts to communicate as if they were on the same local network.

### Key Features

- **Database-driven coordination**: No message queue required (maintains O3K's synchronous philosophy)
- **Automatic VNI allocation**: Sequential VNI assignment with collision prevention
- **FDB synchronization**: MAC-to-VTEP mappings distributed across all nodes
- **Node health checking**: Heartbeat-based monitoring with automatic cleanup
- **Dual-mode support**: Stub mode for testing, real mode for production
- **Platform compatibility**: Works on Linux with graceful fallback on macOS

### Design Philosophy

- **Pull model**: Nodes poll database every 1 second for changes (no push notifications)
- **Synchronous operations**: Direct function calls, no async message passing
- **Fail-fast**: Clear errors when prerequisites not met
- **Namespace isolation**: Per-project network namespaces for tenant separation

---

## Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Compute Node 1                          │
├─────────────────────────────────────────────────────────────────┤
│  O3K Binary (Neutron API + VXLAN Coordinator)                   │
│    ↓                    ↓                      ↓                │
│  Node Registry    VXLANManager         NamespaceManager         │
│  (Heartbeat)      (VXLAN/FDB)          (netns isolation)        │
│    ↓                    ↓                      ↓                │
│  ┌──────────────────────────────────────────────────┐           │
│  │            PostgreSQL (Shared State)             │           │
│  └──────────────────────────────────────────────────┘           │
│                         ↓                                        │
│         Linux Kernel (netlink, VXLAN, bridges)                  │
│                         ↓                                        │
│           vxlan-XXXXXXXX ←→ br-XXXXXXXX ←→ tap-XXXXXX           │
│                         ↓                                        │
│                   libvirt (KVM VM)                              │
└─────────────────────────────────────────────────────────────────┘
                             ↕ VXLAN tunnel (UDP 4789)
┌─────────────────────────────────────────────────────────────────┐
│                         Compute Node 2                          │
│              (Same architecture as Node 1)                      │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow

#### Network Creation Flow

```
1. User creates network via Neutron API
   ↓
2. Neutron stores network in database (network_type='vxlan')
   ↓
3. VXLAN Coordinator (all nodes) polls database every 1s
   ↓
4. Coordinator allocates VNI (1000-10000) using atomic INSERT
   ↓
5. VXLANManager creates VXLAN interface: vxlan-<network_id>
   ↓
6. VXLANManager attaches VXLAN to bridge: br-<network_id>
   ↓
7. Network ready for VM attachment
```

#### Port Creation Flow

```
1. User creates port via Neutron API
   ↓
2. Neutron stores port in database with MAC address
   ↓
3. Neutron calls VXLANCoordinator.DistributeFDBEntry()
   ↓
4. Coordinator inserts FDB entry in database:
   {network_id, mac_address, vtep_ip (local tunnel IP)}
   ↓
5. All nodes poll database and see new FDB entry
   ↓
6. Each node calls VXLANManager.AddFDBEntry()
   ↓
7. netlink adds kernel FDB entry:
   bridge fdb add <MAC> dev vxlan-<network_id> dst <vtep_ip>
   ↓
8. Kernel now knows where to send packets for this MAC
```

#### Packet Flow (VM1 on Node1 → VM2 on Node2)

```
1. VM1 sends Ethernet frame to VM2's MAC
   ↓
2. Frame arrives on tap-<port_id> → br-<network_id>
   ↓
3. Bridge looks up MAC in FDB
   ↓
4. FDB points to vxlan-<network_id> with dst=10.0.0.2 (Node2's tunnel IP)
   ↓
5. Kernel encapsulates frame in VXLAN (UDP/IP)
   ↓
6. Outer IP header: src=10.0.0.1, dst=10.0.0.2, UDP port 4789
   ↓
7. Packet sent over physical network
   ↓
8. Node2 receives UDP packet on port 4789
   ↓
9. Kernel decapsulates VXLAN → original Ethernet frame
   ↓
10. Frame delivered to br-<network_id> → tap-<port_id> → VM2
```

---

## Components

### 1. VXLANManager (`pkg/networking/vxlan.go`)

Manages VXLAN interfaces and FDB entries at the kernel level.

**Key Functions:**

```go
// Create VXLAN interface
CreateVXLAN(networkID string, vni int, localIP string) error

// Delete VXLAN interface
DeleteVXLAN(networkID string) error

// Add FDB entry (MAC -> VTEP IP mapping)
AddFDBEntry(networkID, macAddress, remoteIP string) error

// Remove FDB entry
RemoveFDBEntry(networkID, macAddress string) error

// Attach VXLAN to bridge
AttachToBridge(networkID, bridgeName string, inNamespace bool, nsName string) error
```

**Dual Mode Support:**

- **Stub mode**: In-memory simulation, no actual networking (for macOS testing)
- **Real mode**: Full Linux netlink integration (for production)

**Platform Handling:**

- Linux: Uses `netlink` and `unix` system calls
- macOS: Stub constants allow compilation, operations no-op in stub mode

### 2. NodeRegistry (`internal/compute/node_registry.go`)

Manages compute node registration and heartbeat monitoring.

**Key Functions:**

```go
// Register node in database
RegisterNode(ctx context.Context) error

// Start heartbeat loop (every 30s by default)
StartHeartbeat(ctx context.Context)

// List active nodes (used by coordinator)
ListActiveNodes(ctx context.Context) ([]ComputeNode, error)
```

**Node Detection:**

- **Node ID**: Auto-generated UUID if not specified in config
- **Hostname**: Detected via `os.Hostname()`
- **Tunnel IP**: Auto-detected from primary network interface or specified in config

### 3. VXLANCoordinator (`internal/neutron/vxlan_coordinator.go`)

Orchestrates VXLAN state synchronization across all nodes.

**Key Functions:**

```go
// Start coordinator (begins polling loop)
Start(ctx context.Context)

// Stop coordinator (graceful shutdown)
Stop()

// Distribute FDB entry to all nodes (called from Neutron API)
DistributeFDBEntry(ctx context.Context, networkID, portID, macAddress string) error

// Remove FDB entry from all nodes
RemoveFDBEntry(ctx context.Context, portID string) error
```

**Synchronization Logic:**

1. **syncNetworks()**: Every 1s, query database for VXLAN networks
   - For each network without VXLAN interface: create it
   - For each network with VNI but no VXLAN: allocate VNI and create interface

2. **syncPorts()**: Every 1s, query database for FDB entries
   - For each entry not in local FDB: add it via netlink
   - For each local FDB entry not in DB: remove it

**VNI Allocation Strategy:**

```go
// Sequential allocation with atomic INSERT
for vni := 1000; vni <= 10000; vni++ {
    result, err := db.Exec(`
        INSERT INTO network_vni_allocations (network_id, vni)
        VALUES ($1, $2)
        ON CONFLICT (vni) DO NOTHING
    `, networkID, vni)

    if result.RowsAffected() > 0 {
        return vni, nil // Successfully allocated
    }
}
```

---

## Configuration

### Enabling VXLAN

Add to `config/o3k.yaml`:

```yaml
compute:
  node_id: auto                  # or specify UUID manually
  tunnel_ip: auto                # or specify IP (e.g., "10.0.0.1")
  vxlan_port: 4789               # Standard VXLAN port
  heartbeat_interval: 30s        # Node heartbeat frequency

neutron:
  port: 9696
  networking_mode: iptables      # or "stub" for testing
  vxlan_enabled: true            # Enable VXLAN overlay
  vni_range_start: 1000          # VNI allocation range start
  vni_range_end: 10000           # VNI allocation range end
  coordination_poll_interval: 1s # How often to sync state
```

### Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `compute.node_id` | auto | Node identifier (UUID). Set to "auto" to generate. |
| `compute.tunnel_ip` | auto | Tunnel endpoint IP for VXLAN. Set to "auto" to detect. |
| `compute.vxlan_port` | 4789 | UDP port for VXLAN encapsulation. |
| `compute.heartbeat_interval` | 30s | How often to send heartbeat to database. |
| `neutron.vxlan_enabled` | false | Enable VXLAN overlay networking. |
| `neutron.vni_range_start` | 1000 | Minimum VNI for allocation. |
| `neutron.vni_range_end` | 10000 | Maximum VNI for allocation. |
| `neutron.coordination_poll_interval` | 1s | Database polling frequency. |

### Tunnel IP Detection

If `tunnel_ip: auto`, O3K will:

1. List all network interfaces
2. Find first non-loopback, IPv4 address
3. Use that IP as tunnel endpoint

To override, set explicit IP:

```yaml
compute:
  tunnel_ip: "192.168.100.10"
```

---

## Database Schema

### Migration 004: VXLAN Support

**compute_nodes**: Tracks compute nodes and their tunnel IPs

```sql
CREATE TABLE compute_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hostname VARCHAR(255) UNIQUE NOT NULL,
    tunnel_ip VARCHAR(50) NOT NULL,
    status VARCHAR(50) DEFAULT 'active',
    capabilities JSONB DEFAULT '{}',
    last_heartbeat TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_compute_nodes_status ON compute_nodes(status);
CREATE INDEX idx_compute_nodes_heartbeat ON compute_nodes(last_heartbeat);
```

**network_vni_allocations**: Maps networks to VNIs

```sql
CREATE TABLE network_vni_allocations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id UUID REFERENCES networks(id) ON DELETE CASCADE UNIQUE,
    vni INTEGER NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CHECK (vni >= 1000 AND vni <= 16777215)
);

CREATE INDEX idx_network_vni_network ON network_vni_allocations(network_id);
CREATE INDEX idx_network_vni_vni ON network_vni_allocations(vni);
```

**vxlan_fdb_entries**: MAC-to-VTEP mappings

```sql
CREATE TABLE vxlan_fdb_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id UUID REFERENCES networks(id) ON DELETE CASCADE,
    mac_address VARCHAR(17) NOT NULL,
    vtep_ip VARCHAR(50) NOT NULL,
    port_id UUID REFERENCES ports(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(network_id, mac_address)
);

CREATE INDEX idx_vxlan_fdb_network ON vxlan_fdb_entries(network_id);
CREATE INDEX idx_vxlan_fdb_mac ON vxlan_fdb_entries(mac_address);
CREATE INDEX idx_vxlan_fdb_port ON vxlan_fdb_entries(port_id);
```

**networks**: Extended with VXLAN metadata

```sql
ALTER TABLE networks ADD COLUMN network_type VARCHAR(50) DEFAULT 'flat';
ALTER TABLE networks ADD COLUMN mtu INTEGER DEFAULT 1500;
```

- `network_type='vxlan'` indicates VXLAN overlay network
- `mtu=1450` accounts for 50-byte VXLAN overhead

---

## Deployment

### Single-Node Deployment (Testing)

1. **Configure O3K:**

```yaml
neutron:
  vxlan_enabled: false  # No multi-node needed
```

2. **Start O3K:**

```bash
./bin/o3k --config config/o3k.yaml
```

### Multi-Node Deployment

#### Prerequisites

- PostgreSQL accessible from all nodes
- Network connectivity between tunnel IPs
- UDP port 4789 open between nodes
- Linux kernel with VXLAN support (kernel 3.7+)

#### Node 1 Setup

**`/etc/o3k/node1.yaml`:**

```yaml
compute:
  node_id: node-1
  tunnel_ip: 10.0.0.1
  vxlan_port: 4789

neutron:
  vxlan_enabled: true
  vni_range_start: 1000
  vni_range_end: 10000

database:
  url: "postgres://o3k:secret@db.example.com/o3k"
```

**Start Node 1:**

```bash
./bin/o3k --config /etc/o3k/node1.yaml
```

#### Node 2 Setup

**`/etc/o3k/node2.yaml`:**

```yaml
compute:
  node_id: node-2
  tunnel_ip: 10.0.0.2  # Different tunnel IP
  vxlan_port: 4789

neutron:
  vxlan_enabled: true
  vni_range_start: 1000
  vni_range_end: 10000

database:
  url: "postgres://o3k:secret@db.example.com/o3k"  # Same database

keystone:
  port: 45357  # Different port to avoid conflict

nova:
  port: 18774  # Different port

neutron:
  port: 19696  # Different port
```

**Start Node 2:**

```bash
./bin/o3k --config /etc/o3k/node2.yaml
```

#### Verification

1. **Check node registration:**

```sql
SELECT hostname, tunnel_ip, status, last_heartbeat
FROM compute_nodes;
```

Expected output:

```
 hostname  | tunnel_ip | status |      last_heartbeat
-----------+-----------+--------+--------------------------
 node-1    | 10.0.0.1  | active | 2024-03-07 10:30:45
 node-2    | 10.0.0.2  | active | 2024-03-07 10:30:46
```

2. **Create VXLAN network:**

```bash
openstack network create test-vxlan
```

3. **Check VNI allocation:**

```sql
SELECT n.name, nv.vni
FROM networks n
JOIN network_vni_allocations nv ON n.id = nv.network_id;
```

Expected output:

```
    name     | vni
-------------+------
 test-vxlan  | 1000
```

4. **Check VXLAN interfaces on both nodes:**

```bash
# Node 1
ip link show | grep vxlan

# Node 2
ip link show | grep vxlan
```

Expected: Both nodes have `vxlan-<network_id>` interface

---

## Troubleshooting

### Issue: Nodes not registering

**Symptoms:**

- `compute_nodes` table empty or missing nodes
- Logs show: "Failed to register node"

**Checks:**

1. **Database connectivity:**

```bash
psql -U o3k -d o3k -h db.example.com -c "SELECT 1;"
```

2. **Migration 004 applied:**

```sql
SELECT table_name FROM information_schema.tables
WHERE table_name = 'compute_nodes';
```

3. **Tunnel IP detection:**

```bash
# Check logs for detected tunnel IP
grep "tunnel IP" /var/log/o3k.log

# Manually test detection
ip -4 addr show | grep inet | grep -v 127.0.0.1
```

**Solutions:**

- Verify database URL in config
- Run migration: `./bin/o3k --config config/o3k.yaml --migrate`
- Set explicit `tunnel_ip` in config if auto-detection fails

---

### Issue: VNI allocation fails

**Symptoms:**

- Network created but no VNI allocated
- Logs show: "Failed to allocate VNI"

**Checks:**

1. **VNI range exhausted:**

```sql
SELECT COUNT(*) FROM network_vni_allocations;
```

If count equals `vni_range_end - vni_range_start + 1`, range is full.

2. **Database constraint error:**

```sql
SELECT * FROM network_vni_allocations
WHERE network_id = '<network_id>';
```

**Solutions:**

- Increase VNI range in config (max: 16777215)
- Clean up unused networks:

```sql
DELETE FROM networks WHERE id NOT IN (
    SELECT DISTINCT network_id FROM ports
);
```

---

### Issue: FDB entries not synchronizing

**Symptoms:**

- Port created but VMs can't communicate across nodes
- `bridge fdb show dev vxlan-<id>` missing entries

**Checks:**

1. **FDB entries in database:**

```sql
SELECT * FROM vxlan_fdb_entries;
```

2. **Coordinator running:**

```bash
# Check logs for "VXLAN coordinator started"
grep "coordinator" /var/log/o3k.log
```

3. **Poll interval too long:**

Check `coordination_poll_interval` in config (should be 1s).

4. **Netlink errors:**

```bash
# Check kernel logs
dmesg | grep -i vxlan
```

**Solutions:**

- Verify `vxlan_enabled: true` in config
- Restart O3K to reinitialize coordinator
- Check Linux kernel VXLAN support:

```bash
modprobe vxlan
lsmod | grep vxlan
```

---

### Issue: Tunnel connectivity failure

**Symptoms:**

- Nodes registered, FDB entries present, but VMs can't ping
- `tcpdump` shows no VXLAN packets on physical interface

**Checks:**

1. **Tunnel IP reachability:**

```bash
# From Node 1
ping -c 3 10.0.0.2

# From Node 2
ping -c 3 10.0.0.1
```

2. **UDP port 4789 open:**

```bash
# On Node 2, listen for VXLAN
tcpdump -i eth0 udp port 4789

# On Node 1, trigger traffic (ping from VM)
```

3. **Firewall rules:**

```bash
iptables -L -n | grep 4789
```

**Solutions:**

- Verify routing between tunnel IPs
- Open UDP port 4789:

```bash
iptables -A INPUT -p udp --dport 4789 -j ACCEPT
iptables -A OUTPUT -p udp --dport 4789 -j ACCEPT
```

- Check MTU settings (VXLAN needs MTU ≥ 1550 on physical interface):

```bash
ip link set eth0 mtu 1550
```

---

### Issue: Heartbeat timeout causing flapping

**Symptoms:**

- Node status flips between active/inactive
- Logs show: "Node heartbeat stopped"

**Checks:**

1. **Database load:**

```sql
SELECT COUNT(*) FROM pg_stat_activity;
```

If connection pool exhausted, heartbeats may fail.

2. **Heartbeat interval:**

Check `heartbeat_interval` in config (default: 30s).

**Solutions:**

- Increase `database.max_connections` in config
- Increase `heartbeat_interval` to reduce database load
- Monitor database performance:

```bash
pg_stat_statements
```

---

## Testing

### Unit Tests

Run VXLAN manager tests:

```bash
go test -v ./pkg/networking -run TestVXLAN
```

Expected output:

```
=== RUN   TestNewVXLANManager
--- PASS: TestNewVXLANManager (0.00s)
=== RUN   TestVXLANManagerStubMode
--- PASS: TestVXLANManagerStubMode (0.00s)
=== RUN   TestGetVXLANName
--- PASS: TestGetVXLANName (0.00s)
=== RUN   TestMultipleFDBEntries
--- PASS: TestMultipleFDBEntries (0.00s)
PASS
```

### Integration Tests

Run multi-node simulation:

```bash
./test/vxlan_multinode_test.sh
```

This script:

1. Starts two O3K instances with different ports
2. Verifies node registration
3. Tests VNI allocation
4. Tests FDB synchronization
5. Tests coordinator resilience (node failure)

**Prerequisites:**

- PostgreSQL running locally
- `jq` installed for JSON parsing
- Database user `o3k` with password `o3k`

### Manual Testing

#### Test 1: Two-Node VM Communication

1. **Setup:**

```bash
# Node 1
openstack network create net1
openstack subnet create --network net1 --subnet-range 10.10.0.0/24 subnet1
openstack server create --flavor m1.small --image cirros --network net1 vm1

# Node 2
openstack server create --flavor m1.small --image cirros --network net1 vm2
```

2. **Verify VMs on different nodes:**

```sql
SELECT i.id, i.hostname, cn.hostname as node
FROM instances i
JOIN compute_nodes cn ON i.node_id = cn.id;
```

3. **Test connectivity:**

```bash
# From VM1 console
ping 10.10.0.5  # VM2's IP
```

#### Test 2: FDB Entry Inspection

1. **Create port:**

```bash
openstack port create --network net1 test-port
```

2. **Check database:**

```sql
SELECT mac_address, vtep_ip FROM vxlan_fdb_entries
WHERE network_id = (SELECT id FROM networks WHERE name = 'net1');
```

3. **Check kernel FDB on both nodes:**

```bash
bridge fdb show dev vxlan-$(openstack network show net1 -f value -c id | cut -c1-8)
```

Expected output includes:

```
52:54:00:xx:xx:xx dev vxlan-XXXXXXXX dst 10.0.0.1 self permanent
```

---

## Performance Considerations

### Database Polling

- **Poll interval**: 1 second (configurable)
- **Query overhead**: ~2 queries per second per node (syncNetworks + syncPorts)
- **Recommendation**: For > 100 nodes, increase `coordination_poll_interval` to 5s

### VNI Allocation

- **Worst case**: O(n) where n = vni_range_end - vni_range_start
- **Typical case**: O(1) for first network, O(n) when range near exhaustion
- **Recommendation**: Use wide VNI range (1000-100000) to reduce collisions

### FDB Scaling

- **FDB entries per network**: ~N where N = number of ports
- **Total FDB entries**: Networks × Ports_per_network × Nodes
- **Example**: 100 networks, 10 ports each, 10 nodes = 10,000 FDB entries
- **Recommendation**: Linux kernel handles 100k+ FDB entries efficiently

### MTU Overhead

- **VXLAN overhead**: 50 bytes (8 VXLAN + 8 UDP + 20 IP + 14 Ethernet)
- **Network MTU**: 1500 (default)
- **VXLAN interface MTU**: 1450 (configured automatically)
- **Recommendation**: Set physical interface MTU to 1550 to avoid fragmentation

---

## References

- [RFC 7348: VXLAN](https://datatracker.ietf.org/doc/html/rfc7348)
- [Linux VXLAN Documentation](https://www.kernel.org/doc/Documentation/networking/vxlan.txt)
- [OpenStack Neutron VXLAN](https://docs.openstack.org/neutron/latest/admin/config-ml2-vxlan.html)
- [netlink Library](https://github.com/vishvananda/netlink)

---

## Changelog

### Version 1.0.0 (2024-03-07)

- Initial VXLAN implementation
- Database-driven coordination
- Node heartbeat monitoring
- Automatic VNI allocation
- FDB synchronization
- Multi-node overlay networking
