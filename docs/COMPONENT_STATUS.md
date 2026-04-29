# O3K Component Status

Current implementation status of each subsystem. Updated April 2026.

---

## Nova (Compute)

### Hypervisor — `pkg/hypervisor/`

| Operation | Stub | Real (libvirt) | Notes |
|-----------|:----:|:--------------:|-------|
| Create VM | In-memory map | DomainDefineXML + DomainCreate | Async goroutine, updates DB on completion |
| Delete VM | Map remove | DomainDestroy + DomainUndefine | Checks running state first |
| Start | State update | DomainCreate | |
| Stop | State update | DomainShutdown (graceful) | |
| Reboot | No-op | DomainReboot | |
| Suspend | State update | DomainManagedSave | Saves RAM to disk |
| Resume | State update | DomainCreate (from save) | |
| Attach Device | No-op | DomainAttachDeviceFlags | Disk and NIC |
| Detach Device | No-op | DomainDetachDeviceFlags | |
| Snapshot | NOT IMPLEMENTED | NOT IMPLEMENTED | No code exists |
| Cold Migration | DB state only | NOT IMPLEMENTED | Needs gRPC tunnel |
| Live Migration | DB state only | NOT IMPLEMENTED | Needs shared storage |
| Console (VNC) | Returns placeholder URL | NOT IMPLEMENTED | Needs VNC proxy |

### VM XML Generation — `pkg/hypervisor/xml_template.go`

- Architecture: x86_64, pc-i440fx-2.12
- CPU: host-model passthrough
- Disk: QCOW2 or RBD, virtio bus
- Network: virtio, bridge-attached (`br-{network_id[:8]}`)
- Console: VNC autoport
- Cloud-init: ISO attached as cdrom

### Known Gap: VM-to-Network Wiring

The XML template references a bridge by name, but the code does NOT:
- Create a TAP device for the VM's port
- Move the TAP to the correct namespace
- Attach the TAP to the bridge
- Configure DHCP lease for the specific MAC/IP

This is the P0 gap for real KVM operation.

---

## Neutron (Networking)

### Network Primitives — `pkg/networking/`

| Primitive | Stub | iptables mode | eBPF mode |
|-----------|:----:|:-------------:|:---------:|
| Network namespaces | Map | `ip netns add/del` | `ip netns add/del` |
| Linux bridges | Map | netlink API | netlink API |
| TAP devices | Map | `ip tuntap add` | `ip tuntap add` |
| Bridge attachment | No-op | netlink.LinkSetMaster | netlink.LinkSetMaster |
| VXLAN interfaces | Map | netlink (port 4789, MTU 1450) | netlink |
| FDB entries | Map | netlink.NeighSet/Del | netlink.NeighSet/Del |
| Router namespaces | No-op | `ip netns add` + sysctl | `ip netns add` + sysctl |
| Veth pairs | No-op | `ip link add type veth` | `ip link add type veth` |
| Static routes | No-op | `ip route add` in namespace | `ip route add` in namespace |
| SNAT/Masquerade | No-op | iptables MASQUERADE | iptables MASQUERADE |
| Floating IP NAT | No-op | iptables DNAT + SNAT | iptables DNAT + SNAT |
| DHCP | Flag in map | dnsmasq process per subnet | dnsmasq process per subnet |
| Security groups | Map | iptables chains + rules | XDP program attachment |

### Security Groups

**iptables mode:** Full implementation. Per-security-group chain with DROP default. Rules inserted with ACCEPT for matching traffic. Jump rules from INPUT/OUTPUT to SG chain per interface.

**eBPF mode:** XDP loader framework is real. Rule serialization to eBPF maps is real. But the actual BPF bytecode (`.o` file) that does packet filtering is NOT compiled/included. The C source exists at `pkg/networking/ebpf/secgroup.c` but needs `clang -O2 -target bpf` compilation. Only ingress filtering; egress rules skipped.

### VXLAN Multi-Node — `internal/neutron/vxlan_coordinator.go`

Real implementation that:
- Allocates VNIs per network (1000-16777215 range)
- Creates VXLAN interfaces via netlink
- Syncs FDB entries from database to kernel (1s poll interval)
- Distributes MAC→VTEP IP mappings across nodes

### Known Gap: Port Lifecycle

When a port is created via API, the code:
- Generates MAC, allocates IP, inserts DB record

But does NOT:
- Create TAP device
- Apply security group rules to TAP
- Configure DHCP lease for that MAC/IP pair

TAP creation should happen at port binding time (when a VM is scheduled to a host).

---

## Keystone (Identity)

| Feature | Status |
|---------|--------|
| Password authentication | Real (bcrypt) |
| Token issuance (JWT) | Real (HMAC-SHA256, configurable TTL) |
| Token validation | Real (stateless JWT verification) |
| Service catalog | Real (DB query with hardcoded fallback) |
| Projects CRUD | Real (DB-backed) |
| Users CRUD | Real (DB-backed) |
| Roles + assignments | Real (DB-backed) |
| Domains | Real (DB-backed) |
| Application credentials | Real (DB-backed) |
| Federation/SAML | NOT IMPLEMENTED (by design) |

Fully functional. No gaps for the target use case.

---

## Cinder (Block Storage)

| Feature | Stub | Local | RBD | S3 |
|---------|:----:|:-----:|:---:|:--:|
| Create volume | DB only | File creation | `rbd create` | S3 PutObject |
| Delete volume | DB only | File removal | `rbd remove` | S3 DeleteObject |
| Attach volume | DB only | XML attach via Nova | XML attach | XML attach |
| Detach volume | DB only | XML detach | XML detach | XML detach |
| Snapshot | DB only | Copy-on-write | `rbd snap create` | S3 CopyObject |
| Volume types | DB only | — | — | — |
| Quotas | DB only | — | — | — |

Storage backends are real when configured. The gap is the same as Nova: volume attachment calls `vmManager.AttachDevice()` which works in real mode, but the XML generated for the attachment needs testing with actual RBD pools.

---

## Glance (Image)

| Feature | Stub | Local | RBD | S3 |
|---------|:----:|:-----:|:---:|:--:|
| Upload image | DB only | File write | `rbd import` | S3 PutObject |
| Download image | DB only | File read | `rbd export` | S3 GetObject |
| Delete image | DB only | File removal | `rbd remove` | S3 DeleteObject |
| Image metadata | DB only | DB | DB | DB |
| Multi-backend failover | — | Primary/fallback chain | — | — |

Functional with configured backends.

---

## gRPC Tunnel (Server/Agent)

| Component | Status | Notes |
|-----------|--------|-------|
| Proto definition | Real | Bidirectional streaming, AgentMessage/ServerMessage |
| Hub (server) | Real | Agent registration, removal, selection |
| Token auth (HMAC) | Real | GenerateTokenHash, VerifyTokenHash, enforced on join |
| mTLS | Real | CA generation, cert signing, server/client TLS configs |
| AgentClient | Real | Reconnect loop with 5s backoff |
| Task dispatch | Real | Dispatcher bridges Nova to Hub |
| Subcommands | Real | `o3k server`, `o3k agent`, `o3k token` |
| Agent-side execution | STUB | Prints task and returns success |
| HA scheduling | NOT IMPLEMENTED | PickAgent returns first available |
| Task status tracking | NOT IMPLEMENTED | No acknowledgment/retry |

---

## compat-check (Product Wedge)

| Feature | Status |
|---------|--------|
| CLI (`--dir`, `--output`) | Real |
| Report (JSON + text) | Real |
| Embedded stub server (all 5 services) | Real |
| Recorder middleware | Real |
| Terraform init + plan | Real |
| Token issuance in embedded mode | Real (SeededMockDB) |
| Endpoint overrides | Real |
| E2E result: `compatible:true` | Working |
| CI smoke test | Makefile target exists |

**The product wedge works end-to-end.** `o3k compat-check --dir <terraform-dir>` produces a real compatibility report.

---

## Database

| Feature | Status |
|---------|--------|
| `DBIF` interface | Real |
| `MockDB` (test double) | Real |
| `SeededMockDB` (auth-aware) | Real |
| All services use `activeDB()` | Real (660+ call sites migrated) |
| PostgreSQL 17 | Production driver |
| Connection pooling | Configurable (pgxpool) |
| Migrations | 60 migration files |

---

## Platform Support

| Platform | Hypervisor | Networking | Storage |
|----------|:----------:|:----------:|:-------:|
| Linux (KVM) | Real | Real (iptables/eBPF) | Real (local/RBD/S3) |
| macOS | Stub only | Stub only | Stub only |
| Windows | Untested | Untested | Untested |

---

## Implementation Priority

| # | Gap | Impact | Effort |
|---|-----|--------|--------|
| P0 | VM-to-network wiring (TAP+bridge+DHCP) | VMs have no network without this | 3-5 days |
| P1 | Port binding lifecycle | Ports exist in DB but not on host | 2-3 days |
| P2 | DHCP per-port lease | VMs can't get their allocated IP | 1 day |
| P3 | VNC console proxy | No remote console access | 3-5 days |
| P4 | Snapshot operations | No VM backup capability | 2 days |
| P5 | eBPF bytecode compilation | eBPF mode non-functional | 3-5 days |
| P6 | Port security (anti-spoofing) | VMs can spoof IPs/MACs | 2 days |
| P7 | Agent-side task execution | Multi-node compute doesn't work | 5 days |
| P8 | Live migration | Can't move running VMs | 5-10 days |
