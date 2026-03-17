# O3K Troubleshooting Guide

## TAP Device and Bridge Networking Issues

### Issue: "setting the network namespace failed: Invalid argument"

**Symptom**: VM creation fails with TAP device errors like:
```
failed to create TAP device tap-xxx in namespace light-ns-xxx: exit status 255,
output: setting the network namespace "light-ns-xxx" failed: Invalid argument
```

**Root Cause**: When using Docker with host networking mode (`network_mode: host`), network namespaces must be created on the host filesystem, not inside the container. The `/var/run/netns` directory must be shared between host and container.

**Solution**:
1. Create `/var/run/netns` on the host:
   ```bash
   sudo mkdir -p /var/run/netns
   sudo chmod 755 /var/run/netns
   ```

2. Mount it in docker-compose.yml:
   ```yaml
   o3k:
     volumes:
       - /var/run/netns:/var/run/netns:shared
   ```

3. Restart the container:
   ```bash
   docker compose up -d o3k
   ```

### Issue: "Cannot get interface MTU on 'br-xxx': No such device"

**Symptom**: VM creation fails with libvirt error:
```
Cannot get interface MTU on 'br-3426f435': No such device
```

**Root Cause**: Bridges were created inside project network namespaces, but libvirt runs on the host and cannot access namespaced bridges.

**Solution**: TAP devices and bridges must be created in the default namespace (not project namespaces) for libvirt access. This is the correct architecture:
- TAP devices: created in default namespace
- Bridges: created in default namespace  
- VMs: connect via bridges in default namespace
- Network isolation: handled via security groups and VXLAN, not namespaces

**Code Change** (already implemented in ports.go and network.go):
```go
// Create TAP and bridge in default namespace (not project namespace)
svc.tapManager.CreateTAPDevice(tapName, false, "")  // false = default namespace
svc.brManager.CreateBridge(bridgeName, false, "")   // false = default namespace
```

### Issue: "Failed to get write lock: Is another process using the image"

**Symptom**: QEMU fails to start VM:
```
Failed to get "write" lock
Is another process using the image [/var/lib/o3k/images/xxx.qcow2]?
```

**Root Cause**: Previous VM instances are still running and using the image file.

**Solution**:
1. List all VMs:
   ```bash
   sudo virsh list --all
   ```

2. Destroy and undefine old VMs:
   ```bash
   sudo virsh destroy instance-xxx
   sudo virsh undefine instance-xxx
   ```

3. Verify no VMs are using the image:
   ```bash
   sudo lsof /var/lib/o3k/images/*.qcow2
   ```

## Silent VM Creation Failures

### Issue: Instances go to ERROR status with no logs

**Symptom**: VMs reach ERROR status immediately with no error messages in logs.

**Root Causes**:
1. Panics in goroutines not caught
2. Insufficient debug logging
3. Image file path mismatches (UUID.qcow2 vs actual filename)
4. Image status not "active" in database

**Solutions**:

1. Add panic recovery in handlers.go:
   ```go
   defer func() {
       if r := recover(); r != nil {
           log.Printf("PANIC in VM creation: %v", r)
           database.DB.Exec(ctx, "UPDATE instances SET status = 'ERROR' WHERE id = $1", instanceID)
       }
   }()
   ```

2. Add extensive debug logging:
   ```go
   log.Printf("DEBUG: VM creation goroutine started for instance %s", instanceID)
   log.Printf("DEBUG: Allocating ports for instance %s", instanceID)
   // ... more logging at each step
   ```

3. Fix image file paths:
   ```bash
   # Create symlink if needed
   cd /var/lib/o3k/images
   ln -sf actual-image.img UUID.qcow2
   ```

4. Fix image status:
   ```sql
   UPDATE images SET status='active', size_bytes=21000000 WHERE name='cirros-0.6.2';
   ```

## Verification Commands

### Check VM Status
```bash
# Via OpenStack API
TOKEN=$(curl -s -X POST http://localhost:35357/v3/auth/tokens ...)
curl -H "X-Auth-Token: $TOKEN" http://localhost:8774/v2.1/servers/detail | jq '.servers'

# Via virsh
sudo virsh list --all
```

### Check Networking
```bash
# Check bridges
sudo brctl show

# Check TAP devices
sudo ip link show | grep tap-

# Check interfaces in bridge
sudo brctl show br-3426f435

# Check namespace (if using namespaces)
sudo ip netns list
sudo ip netns exec light-ns-xxx ip link show
```

### Check O3K Logs
```bash
# Docker deployment
docker logs o3k --tail 100

# Look for specific errors
docker logs o3k 2>&1 | grep -E 'ERROR|failed'

# Check debug logs
docker logs o3k 2>&1 | grep DEBUG
```

## Common Diagnostic Steps

1. **Verify host requirements**:
   ```bash
   # Check KVM support
   lsmod | grep kvm
   
   # Check libvirt
   sudo systemctl status libvirtd
   
   # Check networking
   sudo ip link show
   ```

2. **Verify O3K configuration**:
   ```bash
   # Check mode settings
   grep -E 'libvirt_mode|networking_mode' /opt/o3k/config/o3k.yaml
   
   # Verify database connection
   psql -U o3k -d o3k -c "SELECT COUNT(*) FROM instances;"
   ```

3. **Check file permissions**:
   ```bash
   # libvirt socket
   ls -la /var/run/libvirt/libvirt-sock
   
   # Image directory
   ls -la /var/lib/o3k/images/
   
   # netns directory
   ls -la /var/run/netns/
   ```

4. **Review recent changes**:
   ```bash
   # Check O3K container status
   docker ps | grep o3k
   
   # Check container restarts
   docker inspect o3k | jq '.[].State'
   
   # Check mounted volumes
   docker inspect o3k | jq '.[].Mounts'
   ```
