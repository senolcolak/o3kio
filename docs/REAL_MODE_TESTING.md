# Real Mode Testing Guide

## Status

Real mode is implemented for Nova (libvirt/KVM), Neutron (netlink + iptables),
Cinder (local/RBD/S3), and Glance (local/RBD/S3). It has been exercised on
Linux developer machines and in the contract-test environment, but it has
**not** been hardened or audited for production use. Treat this guide as
"how to drive real mode for evaluation," not "how to deploy O3K."

Stub mode is the default and runs on macOS, Linux, and Windows without any
external dependencies. Real mode requires Linux with KVM and libvirt for
Nova, and root or `CAP_NET_ADMIN` for Neutron's namespace and iptables work.

## Real Mode Requirements

### Linux System Requirements

1. **KVM kernel modules** loaded
   ```bash
   lsmod | grep kvm
   # Should show: kvm_intel or kvm_amd
   ```

2. **libvirt daemon** running
   ```bash
   systemctl status libvirtd
   ```

3. **libvirt socket** available at `/var/run/libvirt/libvirt-sock`
   (the path is currently hard-coded; see
   `pkg/hypervisor/libvirt.go:connectLibvirt`).

4. **User permissions**
   ```bash
   groups $USER | grep libvirt
   # If missing: sudo usermod -aG libvirt $USER && newgrp libvirt
   ```

5. **Networking real mode** additionally requires the ability to create
   network namespaces and modify iptables. In practice this means running
   O3K as root or granting `CAP_NET_ADMIN` + `CAP_SYS_ADMIN`.

## Testing Real Mode on Linux

### Step 1: Install libvirt (Ubuntu/Debian)

```bash
sudo apt-get update
sudo apt-get install -y \
  qemu-kvm \
  libvirt-daemon-system \
  libvirt-clients \
  bridge-utils

sudo usermod -aG libvirt $USER
sudo systemctl enable --now libvirtd

# Sanity check
virsh list --all
```

### Step 2: Configure O3K for Real Mode

```yaml
# config/o3k.yaml
nova:
  port: 8774
  libvirt_uri: "qemu:///system"
  default_flavor: m1.small
  libvirt_mode: real

neutron:
  networking_mode: iptables   # or stub for evaluation without netns

cinder:
  storage_mode: local         # or rbd, s3, or comma-separated hybrid

glance:
  storage_mode: local         # or rbd, s3, or comma-separated hybrid
```

Change the JWT secret and seed credentials before any non-development
use. See `SECURITY.md`.

### Step 3: Start O3K

```bash
./bin/o3k --config config/o3k.yaml
```

If libvirt is not reachable O3K logs a warning and continues with the
hypervisor disabled. VM-creating endpoints will then return errors.

### Step 4: Authenticate and Create a VM

The seed migration uses `Default` (capitalised) for the domain name.

```bash
export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default

openstack server create \
  --flavor m1.small \
  --image cirros \
  test-vm-real
```

### Step 5: Verify

```bash
openstack server show test-vm-real

# And on the host:
virsh list --all
# Id   Name              State
# 1    instance-eb08da45 running
```

### Step 6: Lifecycle

```bash
openstack server stop test-vm-real
openstack server start test-vm-real
openstack server reboot test-vm-real
openstack server delete test-vm-real
```

## What Real Mode Currently Does

### Nova (`libvirt_mode: real`)
- `CreateVM`: `DomainDefineXML` + `DomainCreate`
- `DeleteVM`: `DomainDestroy` + `DomainUndefine`
- `StartVM` / `StopVM` / `RebootVM`: `DomainCreate` / `DomainShutdown` /
  `DomainReboot`
- `GetVMState`: `DomainGetState`
- `AttachDevice` / `DetachDevice`: `DomainAttachDeviceFlags` /
  `DomainDetachDeviceFlags` (used for both disk and NIC hot-plug)
- `SuspendVM` / `ResumeVM`: `DomainManagedSave` / `DomainCreate`

XML templates live in `pkg/hypervisor/xml_template.go`.

### Neutron (`networking_mode: iptables`)
- Linux network namespaces per project (`pkg/networking/netns.go`)
- Linux bridges and TAP devices for VM attachment
  (`TAPDeviceManager.CreateTAPDevice`)
- iptables rules for security groups
- Optional VXLAN overlay for multi-node setups
  (`neutron.vxlan_enabled: true`)

### Cinder (`storage_mode: local|rbd|s3` or hybrid)
- Local: host filesystem under the configured directory
- RBD: Ceph RBD via `github.com/ceph/go-ceph`
- S3: AWS S3 / MinIO via the AWS SDK v2
- Hybrid: comma-separated, first entry primary, others fallback

### Glance (`storage_mode: local|rbd|s3` or hybrid)
- Same backends as Cinder. Hybrid mode falls back automatically when the
  primary backend errors.

### Metadata (port 8775)
- EC2-style endpoints (`/latest/user-data`, `/latest/meta-data/...`)
- OpenStack endpoints (`/openstack/latest/user_data`,
  `/openstack/latest/meta_data.json`)
- See `internal/metadata/service.go`.

### Console
- VNC console allocation and serial console output via
  `internal/nova/console.go`. Requires libvirt to expose a VNC port; the
  console service returns a token URL.

## Honest Caveats

Things that work in code paths but have not been hardened or fully
exercised end-to-end across releases:

- **Cloud-init injection beyond user-data over HTTP**: SSH key injection
  relies on the metadata service being reachable from the guest network.
  If networking real mode is not configured, the guest cannot reach the
  metadata endpoint.
- **Live migration**: not implemented.
- **Resize / cold migration**: partial; not covered by contract tests.
- **VNC console TLS**: VNC is exposed plaintext; front it with a TLS
  proxy if exposed beyond localhost.
- **Multi-node scheduling under load**: the scheduler exists but has not
  been benchmarked or chaos-tested.

## Stub vs Real Mode

| Feature | Stub Mode | Real Mode |
|---|---|---|
| libvirt required | No | Yes (Nova) |
| KVM required | No | Yes (Nova) |
| Linux required | No | Yes (Nova, Neutron) |
| Network namespaces | No | Yes (Neutron iptables/eBPF) |
| VM visible to `virsh` | No | Yes |
| Storage on disk | No | Yes (local / RBD / S3) |
| Metadata endpoint usable from guest | Limited | Yes |
| Suitable for | API exploration, contract testing | Evaluation on a single Linux host |

## Testing on macOS

macOS does not have KVM, so real-mode Nova is not available. Options:

- **Multipass / Lima / OrbStack**: launch a Linux VM and run O3K real
  mode inside it.
- **Cloud VM**: any provider that exposes nested virtualisation.
- **Stay in stub mode** for API and contract testing — it is fully
  supported on macOS.

## Error Handling

| Symptom | Likely Cause |
|---|---|
| `dial unix /var/run/libvirt/libvirt-sock: connect: no such file or directory` | libvirtd is not running, or the socket lives elsewhere |
| `permission denied` on the libvirt socket | User is not in the `libvirt` group; run `newgrp libvirt` |
| `failed to define domain: invalid argument` | XML template referenced a flavor or image that does not exist; check Nova logs |
| `operation not permitted` from netlink | Neutron real mode needs root or `CAP_NET_ADMIN` |

Inspect libvirt logs for VM-side failures:

```bash
journalctl -u libvirtd -f
```

## Verification Commands

```bash
# Connection
virsh -c qemu:///system list --all

# O3K mode (look for "real" in the startup log)
./bin/o3k --config config/o3k.yaml 2>&1 | grep -i 'libvirt\|hypervisor'

# Permissions
groups | tr ' ' '\n' | grep libvirt
```

## Not Yet Suitable For Production

To be explicit: this guide describes how to *exercise* real mode, not
how to operate it as a production OpenStack control plane. See the
top-level README's project-status section and `SECURITY.md` for the
gap list.
