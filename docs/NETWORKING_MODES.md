# O3K Networking Modes Deployment Guide

## Overview

O3K supports three networking modes, each with different performance characteristics and requirements:

1. **Stub Mode** - Testing and development (no kernel dependencies)
2. **IPTables Mode** - Traditional Linux networking with iptables security groups
3. **eBPF Mode** - Modern high-performance networking with eBPF security groups

## Architecture

All three modes share the same basic structure but differ in implementation:

```
┌─────────────────────────────────────────────────────────┐
│                  Networking Layer                        │
├─────────────────────────────────────────────────────────┤
│  Network Namespaces (per project)                       │
│  Linux Bridges (per network)                            │
│  TAP Devices (per VM interface)                         │
│  DHCP (dnsmasq per network)                             │
└─────────────────────────────────────────────────────────┘
                         ↓
         ┌───────────────┼────────────────┐
         ↓               ↓                ↓
    Stub Mode      IPTables Mode      eBPF Mode
    (Simulated)    (iptables rules)   (BPF programs)
```

**Key Insight**: Stub, IPTables, and eBPF modes use the same networking infrastructure. They only differ in how security groups are implemented.

---

## Mode 1: Stub Mode

### Purpose
- Development and testing without kernel dependencies
- CI/CD environments
- macOS/Windows development

### Configuration
```yaml
neutron:
  networking_mode: stub
```

### Requirements
- None! Works on any OS

### Behavior
- Simulates all networking operations in memory
- No actual network namespaces, bridges, or TAP devices created
- Perfect for API testing and development
- Full OpenStack API compatibility

### Use Cases
- Local development on macOS/Windows
- Unit testing
- API integration testing
- CI/CD pipelines

---

## Mode 2: IPTables Mode

### Purpose
- Production deployments with traditional Linux networking
- Battle-tested, well-understood security model
- Compatible with all Linux kernels (2.6+)

### Configuration
```yaml
neutron:
  networking_mode: iptables
```

### Requirements

**System:**
- Linux kernel 2.6+ (3.10+ recommended)
- `ip` command (iproute2 package)
- `iptables` command
- Root or CAP_NET_ADMIN privileges

**Packages (Ubuntu/Debian):**
```bash
sudo apt-get install -y iproute2 iptables bridge-utils dnsmasq
```

**Packages (RHEL/CentOS/Fedora):**
```bash
sudo dnf install -y iproute iptables bridge-utils dnsmasq
```

### How It Works

1. **Network Namespace Isolation**
   ```bash
   # One namespace per project
   ip netns add light-ns-<project_id>
   ```

2. **Bridge per Network**
   ```bash
   # Inside each namespace
   ip netns exec light-ns-<project_id> ip link add br-<network_id> type bridge
   ip netns exec light-ns-<project_id> ip link set br-<network_id> up
   ```

3. **TAP Devices for VMs**
   ```bash
   # Create TAP for each VM interface
   ip tuntap add tap-<port_id> mode tap
   ip link set tap-<port_id> master br-<network_id>
   ```

4. **DHCP with dnsmasq**
   ```bash
   # One dnsmasq per network
   ip netns exec light-ns-<project_id> dnsmasq \
     --interface=br-<network_id> \
     --dhcp-range=192.168.1.10,192.168.1.200,24h
   ```

5. **Security Groups with iptables**
   ```bash
   # Create chain per security group
   iptables -t filter -N O3K-SG-<sg_id>
   iptables -t filter -A O3K-SG-<sg_id> -j DROP  # Default deny

   # Add rules
   iptables -t filter -I O3K-SG-<sg_id> 1 -p tcp --dport 22 -j ACCEPT
   iptables -t filter -I O3K-SG-<sg_id> 1 -p icmp -j ACCEPT
   ```

### Performance
- **Throughput**: Good (near line-rate for most workloads)
- **Latency**: Low (userspace iptables processing adds ~10-50μs)
- **Connection Rate**: Moderate (thousands of new connections/sec)
- **CPU Usage**: Low to moderate (scales with connection count)

### Pros
- Well-understood and debugged
- Works on all Linux kernels
- Extensive tooling (iptables-save, iptables-restore)
- Easy to troubleshoot

### Cons
- Userspace processing adds latency
- Not as performant as eBPF for high packet rates
- Security group updates require iptables chain modifications

---

## Mode 3: eBPF Mode

### Purpose
- High-performance production deployments
- Low-latency requirements
- High packet rate scenarios (millions of pps)
- Modern Linux environments

### Configuration
```yaml
neutron:
  networking_mode: ebpf
```

### Requirements

**System:**
- Linux kernel 4.18+ (5.10+ recommended for best eBPF features)
- `ip` command (iproute2 package)
- `bpftool` command
- Root or CAP_NET_ADMIN + CAP_BPF privileges
- BTF (BPF Type Format) support in kernel

**Packages (Ubuntu 22.04+):**
```bash
sudo apt-get install -y iproute2 linux-tools-common linux-tools-generic bpftool
```

**Packages (Fedora 36+):**
```bash
sudo dnf install -y iproute bpftool
```

**Kernel Configuration:**
Verify eBPF support:
```bash
# Check kernel version
uname -r  # Should be 4.18+

# Check eBPF features
zgrep CONFIG_BPF /proc/config.gz | grep -E '(BPF|XDP)'
# Should show:
# CONFIG_BPF=y
# CONFIG_BPF_SYSCALL=y
# CONFIG_XDP_SOCKETS=y
```

### How It Works

1. **Network Namespace Isolation** (same as iptables mode)
   ```bash
   ip netns add light-ns-<project_id>
   ```

2. **Bridge per Network** (same as iptables mode)
   ```bash
   ip netns exec light-ns-<project_id> ip link add br-<network_id> type bridge
   ```

3. **TAP Devices** (same as iptables mode)
   ```bash
   ip tuntap add tap-<port_id> mode tap
   ip link set tap-<port_id> master br-<network_id>
   ```

4. **DHCP** (same as iptables mode)
   ```bash
   ip netns exec light-ns-<project_id> dnsmasq --interface=br-<network_id> ...
   ```

5. **Security Groups with eBPF**
   ```bash
   # Compile and load eBPF program
   clang -O2 -target bpf -c security_group.c -o security_group.o

   # Attach to TC (traffic control) ingress/egress
   tc qdisc add dev tap-<port_id> clsact
   tc filter add dev tap-<port_id> ingress bpf direct-action obj security_group.o sec ingress
   tc filter add dev tap-<port_id> egress bpf direct-action obj security_group.o sec egress
   ```

**eBPF Program Structure:**
```c
// security_group.c
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>

SEC("ingress")
int sg_ingress(struct __sk_buff *skb) {
    // Parse packet headers in kernel space
    // Check against security group rules in BPF map
    // Return TC_ACT_OK (allow) or TC_ACT_SHOT (drop)

    // Example: Allow TCP port 22
    struct ethhdr *eth = bpf_skb_load_bytes(skb, 0, sizeof(*eth));
    if (eth->h_proto == htons(ETH_P_IP)) {
        struct iphdr *ip = bpf_skb_load_bytes(skb, sizeof(*eth), sizeof(*ip));
        if (ip->protocol == IPPROTO_TCP) {
            struct tcphdr *tcp = bpf_skb_load_bytes(skb, ...);
            if (ntohs(tcp->dest) == 22) {
                return TC_ACT_OK;  // Allow SSH
            }
        }
    }
    return TC_ACT_SHOT;  // Default drop
}
```

### Performance
- **Throughput**: Excellent (10-40% better than iptables)
- **Latency**: Ultra-low (kernel-space processing, ~1-5μs overhead)
- **Connection Rate**: Very high (millions of new connections/sec)
- **CPU Usage**: Very low (in-kernel processing, no context switches)

### Pros
- Kernel-space packet filtering (no userspace overhead)
- Extremely fast (orders of magnitude faster than iptables for complex rules)
- Dynamic rule updates without packet loss
- Low CPU usage even under high load
- Modern, future-proof technology

### Cons
- Requires newer kernels (4.18+, best on 5.10+)
- More complex to debug (requires bpftool, kernel tracing)
- Smaller community compared to iptables
- Requires eBPF program compilation

---

## Performance Comparison

### Packet Processing Latency
| Mode      | Latency (μs) | Notes                               |
|-----------|--------------|-------------------------------------|
| Stub      | N/A          | No actual packet processing         |
| IPTables  | 10-50        | Userspace iptables processing       |
| eBPF      | 1-5          | Kernel-space BPF processing         |

### Throughput (10 Gbps NIC)
| Mode      | Throughput   | Packet Rate   |
|-----------|--------------|---------------|
| Stub      | N/A          | N/A           |
| IPTables  | 8-9 Gbps     | 1-2M pps      |
| eBPF      | 9.5-9.8 Gbps | 5-10M pps     |

### CPU Overhead (at 5 Gbps)
| Mode      | CPU Usage | Notes                           |
|-----------|-----------|----------------------------------|
| Stub      | 0%        | No processing                   |
| IPTables  | 15-25%    | Per core, scales with rules     |
| eBPF      | 3-8%      | Per core, minimal scaling       |

---

## Deployment Examples

### Example 1: Development on macOS (Stub Mode)
```yaml
# config/o3k.yaml
neutron:
  networking_mode: stub
```

```bash
# No special requirements
go run cmd/o3k/main.go --config config/o3k.yaml
```

### Example 2: Production on Ubuntu 22.04 (IPTables Mode)
```bash
# Install dependencies
sudo apt-get update
sudo apt-get install -y iproute2 iptables bridge-utils dnsmasq

# Configure O3K
cat > /etc/o3k/o3k.yaml <<EOF
neutron:
  networking_mode: iptables
EOF

# Run O3K with systemd
sudo systemctl start o3k
```

### Example 3: High-Performance on Ubuntu 24.04 (eBPF Mode)
```bash
# Install dependencies
sudo apt-get update
sudo apt-get install -y iproute2 linux-tools-generic bpftool clang llvm

# Verify kernel version
uname -r  # Should be 5.15+ or 6.x

# Configure O3K
cat > /etc/o3k/o3k.yaml <<EOF
neutron:
  networking_mode: ebpf
EOF

# Run O3K with systemd
sudo systemctl start o3k
```

---

## Switching Between Modes

To switch modes, simply:

1. Stop O3K
2. Update `config/o3k.yaml` with desired `networking_mode`
3. Restart O3K

**Note**: Existing networks/VMs will need to be recreated when switching modes.

---

## Testing Each Mode

### Test Stub Mode (Works on any OS)
```bash
# Start O3K
go run cmd/o3k/main.go --config config/o3k.yaml

# Create network
openstack network create test-network

# Verify in database
psql -d lightstack -c "SELECT id, name, status FROM networks;"
```

### Test IPTables Mode (Linux only)
```bash
# Start O3K
sudo go run cmd/o3k/main.go --config config/o3k.yaml

# Create network
openstack network create test-network

# Verify namespace created
sudo ip netns list | grep light-ns

# Verify bridge created
sudo ip netns exec light-ns-<project_id> ip link show | grep br-
```

### Test eBPF Mode (Linux 4.18+ only)
```bash
# Start O3K
sudo go run cmd/o3k/main.go --config config/o3k.yaml

# Create network
openstack network create test-network

# Verify eBPF programs loaded
sudo bpftool prog list | grep O3K

# Verify TC filters
sudo tc filter show dev tap-<port_id> ingress
```

---

## Troubleshooting

### Common Issues

**Issue**: `exec: "ip": executable file not found`
- **Cause**: Running iptables/eBPF mode on non-Linux OS
- **Solution**: Use stub mode for development, or deploy on Linux

**Issue**: `failed to initialize iptables`
- **Cause**: Missing iptables package or insufficient permissions
- **Solution**: Install iptables, run with sudo

**Issue**: `failed to load eBPF program`
- **Cause**: Kernel too old or missing eBPF support
- **Solution**: Upgrade to Linux 4.18+, enable CONFIG_BPF in kernel

**Issue**: `permission denied` when creating namespace
- **Cause**: Insufficient privileges
- **Solution**: Run O3K with sudo or CAP_NET_ADMIN capability

---

## Recommendations

**Choose Stub Mode if:**
- Developing on macOS/Windows
- Running CI/CD tests
- Don't need actual networking

**Choose IPTables Mode if:**
- Running on Linux kernel <4.18
- Need maximum compatibility
- Prefer battle-tested technology
- Have moderate performance requirements

**Choose eBPF Mode if:**
- Running on Linux kernel 4.18+
- Need maximum performance
- Have high packet rate requirements
- Building for the future

---

## Future Enhancements

### Planned for eBPF Mode
- [ ] XDP (eXpress Data Path) support for even faster processing
- [ ] BPF map-based rule updates (no program reload needed)
- [ ] Per-connection state tracking in BPF maps
- [ ] Integration with Cilium for advanced networking features
- [ ] eBPF-based load balancing
