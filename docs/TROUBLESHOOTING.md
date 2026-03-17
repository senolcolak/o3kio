# O3K Troubleshooting Guide

## Instance Creation - Silent Failure (SOLVED)

**Problem**: Instances go to ERROR status with no logs.

**Root Causes**:
1. No panic recovery in VM creation goroutine
2. Missing debug logging
3. Image file path mismatch
4. Image status not "active"

**Solution**:
- Added panic recovery and debug logging to `internal/nova/handlers.go`
- Created symlink: `ln -sf cirros-0.6.2.img {UUID}.qcow2`
- Update image status: `UPDATE images SET status='active'`

**Result**: VMs now create successfully and show proper error messages.

---

## Networking - TAP Device Creation Fails

**Error**: `failed to create TAP device: exit status 255`

**Impact**: VMs start but have no network connectivity.

**Status**: Known issue under investigation.

**Workaround**: Create VMs without networking initially.

---

## Horizon - Connection Issues (SOLVED)

**Problem**: "Unable to establish connection to keystone endpoint"

**Solution**:
1. Update `/opt/o3k/config/horizon-config/local_settings`:
   - Use public domain name: `OPENSTACK_HOST = "o3k.cephos.eu-de-2.cloud.sap"`
   - Use HOST_IP for memcached: `'LOCATION': '10.2.199.101:11211'`

2. Update service catalog:
   ```sql
   UPDATE endpoints SET url = REPLACE(url, 'http://localhost:', 'http://o3k.cephos.eu-de-2.cloud.sap:');
   ```

3. Fix Python indentation errors (no leading spaces)

4. Memcached listens on 0.0.0.0 not 127.0.0.1

**Result**: Horizon fully functional at http://o3k.cephos.eu-de-2.cloud.sap/dashboard

---

## Quick Diagnostics

```bash
# Check O3K logs
docker logs o3k --tail 50 | grep -E "ERROR|DEBUG"

# Check instance status
virsh list --all
sudo -u postgres psql o3k -c "SELECT name, status FROM instances;"

# Check network namespaces
docker exec o3k ip netns list

# Check libvirt socket
ls -la /var/run/libvirt/libvirt-sock  # Should be 666
```

---

## Current Working State

✅ Horizon dashboard accessible
✅ Authentication working (JWT tokens)
✅ Network creation working
✅ VM creation working (KVM/libvirt)
✅ Instance visible in Horizon
✅ Instance console accessible (noVNC)

⚠️ VM networking (TAP devices) - under investigation

---

## Credentials

- **Username**: admin
- **Password**: secret
- **Domain**: Default
- **Project**: default
