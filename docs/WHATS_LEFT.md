# What's Left — O3K Implementation Gaps

**Version**: v0.7.0 | **API Coverage**: 342/330 endpoints (104%) | **Updated**: April 2026

For detailed per-component status, see [COMPONENT_STATUS.md](COMPONENT_STATUS.md).

---

## Critical Path: Real KVM + Networking

The API surface is complete. The gap is **real-mode operation**: making VMs actually boot with network connectivity on a Linux KVM host.

### P0: VM-to-Network Wiring (3-5 days)

When `CreateServer` runs in real mode, the VM XML references a bridge but nothing creates the TAP device, attaches it to the bridge, or configures DHCP for the VM's MAC/IP. Without this, VMs boot with no network.

**What needs to happen:**
1. Nova asks Neutron to bind the port to the host
2. Neutron creates a TAP device named `tap-{port_id[:11]}`
3. TAP is attached to the network's bridge (`br-{network_id[:8]}`)
4. DHCP lease configured for the port's MAC → IP mapping
5. Security group rules applied to the TAP
6. VM XML references the TAP device

### P1: Port Binding Lifecycle (2-3 days)

Ports exist in the database but are never "bound" to a host. Port binding should create the TAP and wire security groups when a VM is scheduled.

### P2: Per-Port DHCP Lease (1 day)

dnsmasq currently gets a range. It should get explicit MAC→IP static leases from port allocations.

---

## Implemented and Working

### compat-check (Product Wedge)
- [x] CLI binary with `--dir` and `--output` flags
- [x] Report struct (JSON + text output)
- [x] Recorder middleware captures all API calls
- [x] Embedded server with all 5 services in stub mode
- [x] Terraform init + plan with 5-minute timeout
- [x] Token issuance via SeededMockDB
- [x] Endpoint overrides route Terraform to embedded server
- [x] **E2E: produces `compatible:true` against real Terraform config**

### gRPC Server/Agent
- [x] Proto definition (bidirectional streaming)
- [x] Hub with agent registration/removal/selection
- [x] AgentClient with reconnect loop (5s backoff)
- [x] Task dispatch (Dispatcher bridges Nova to Hub)
- [x] Token auth (HMAC-SHA256, enforced on join)
- [x] mTLS (CA generation, cert signing, server/client configs)
- [x] Subcommands: `o3k server`, `o3k agent`, `o3k token`
- [x] Nova async dispatch when `async_compute: true`

### Database DI
- [x] `DBIF` interface + `MockDB` + `SeededMockDB`
- [x] Global `database.DB` migrated to interface
- [x] All 660+ call sites across 53 files use `activeDB()`
- [x] Unit tests with MockDB (Nova, Keystone)

### Networking Primitives (all real in iptables/ebpf mode)
- [x] Network namespaces, bridges, TAP devices
- [x] VXLAN interfaces with FDB management
- [x] Router namespaces with IP forwarding
- [x] Floating IP NAT (DNAT + SNAT via iptables)
- [x] SNAT/Masquerade for internal subnets
- [x] DHCP via dnsmasq (per-subnet)
- [x] Security groups (iptables chains with DROP default)
- [x] VXLAN coordination (DB-polled FDB sync)

### Hypervisor (real in KVM mode)
- [x] Create/Delete/Start/Stop/Reboot via libvirt
- [x] Suspend/Resume (DomainManagedSave)
- [x] Attach/Detach devices
- [x] XML generation (x86_64, virtio, VNC, cloud-init)

---

## Not Implemented (by design)

| Feature | Reason |
|---------|--------|
| Keystone Federation/SAML | Enterprise-only, <5% demand |
| Neutron DVR | Large-cloud-only feature |
| Nova live migration | Requires shared storage infrastructure |
| OVS/OpenFlow | Linux bridges sufficient for target use case |
| Heat (orchestration) | Out of scope (use Terraform directly) |
| Swift (object storage) | Out of scope (use S3) |
| Barbican (secrets) | Out of scope |
| Designate (DNS) | Out of scope |
| Octavia (LB) | Out of scope |

---

## Next Steps

1. **Fix P0** — VM-to-network wiring. This makes real KVM work.
2. **Behavioral fidelity** — Error codes and pagination for the 40% of APIs Terraform calls.
3. **Find users** — The compat-check wedge works. Put it in front of architects doing migrations.
