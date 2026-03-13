# O3K Troubleshooting Guide

**Version**: v0.4.1
**Last Updated**: March 13, 2026

This guide covers common issues, error messages, and solutions for O3K deployments.

---

## Table of Contents

1. [Database Connection Issues](#database-connection-issues)
2. [Authentication & Token Problems](#authentication--token-problems)
3. [API Errors](#api-errors)
4. [Performance Issues](#performance-issues)
5. [Networking Problems](#networking-problems)
6. [Storage Issues](#storage-issues)
7. [Compute (VM) Issues](#compute-vm-issues)
8. [Configuration Problems](#configuration-problems)

---

## Database Connection Issues

### "Failed to connect to database"

**Symptoms**:
```
FATAL: Failed to connect to database: failed to ping database: connection refused
```

**Causes**:
1. PostgreSQL not running
2. Incorrect connection URL
3. Firewall blocking port 5432
4. PostgreSQL not accepting remote connections

**Solutions**:

**Check PostgreSQL is running**:
```bash
# Check status
sudo systemctl status postgresql

# Start if needed
sudo systemctl start postgresql

# Docker Compose
docker compose ps
docker compose up -d postgres
```

**Verify connection URL**:
```yaml
# config/o3k.yaml
database:
  url: "postgres://user:password@host:5432/database?sslmode=disable"
  #        ^user   ^pass      ^host  ^port ^dbname
```

**Test connection manually**:
```bash
psql "postgres://user:password@host:5432/database"
```

**Check PostgreSQL accepts connections**:
```bash
# Edit postgresql.conf
listen_addresses = '*'  # Or specific IP

# Edit pg_hba.conf (add line)
host    all    all    0.0.0.0/0    md5

# Restart PostgreSQL
sudo systemctl restart postgresql
```

### "Too many connections"

**Symptoms**:
```
ERROR: Database error: FATAL: sorry, too many clients already
```

**Causes**:
- PostgreSQL max_connections exceeded
- O3K connection pool too large
- Connection leaks (not closing connections)

**Solutions**:

**Increase PostgreSQL max_connections**:
```ini
# postgresql.conf
max_connections = 200  # Default is 100

# Restart PostgreSQL
sudo systemctl restart postgresql
```

**Reduce O3K connection pool**:
```yaml
# config/o3k.yaml
database:
  max_connections: 20  # Reduce from higher value
```

**Check for connection leaks**:
```bash
# Monitor active connections
psql -U user -d database -c "SELECT count(*) FROM pg_stat_activity;"

# List connections by application
psql -U user -d database -c "SELECT application_name, count(*) FROM pg_stat_activity GROUP BY application_name;"
```

### Slow Database Queries

**Symptoms**:
```
WARN: Slow query detected: duration=250ms query="SELECT * FROM instances WHERE project_id = $1"
```

**Solutions**:

1. **Check for missing indexes**:
```sql
-- List indexes for a table
\d instances

-- Create missing index
CREATE INDEX idx_instances_project_id ON instances(project_id);
```

2. **Analyze query plan**:
```sql
EXPLAIN ANALYZE SELECT * FROM instances WHERE project_id = 'abc-123';
```

3. **See DATABASE_OPTIMIZATION.md** for detailed tuning guide

---

## Authentication & Token Problems

### "Authentication failed"

**Symptoms**:
```
{"unauthorized": {"message": "Invalid credentials", "code": 401}}
```

**Causes**:
1. Wrong username/password
2. User doesn't exist
3. Account disabled

**Solutions**:

**Verify credentials**:
```bash
# Default credentials
OS_USERNAME=admin
OS_PASSWORD=secret

# Test authentication
openstack token issue
```

**Check user exists**:
```sql
-- Connect to database
psql -U lightstack -d lightstack

-- List users
SELECT id, name, enabled FROM users;

-- Check password hash
SELECT name, password FROM users WHERE name = 'admin';
```

**Reset admin password**:
```sql
-- Generate new bcrypt hash (use bcrypt tool)
UPDATE users SET password = '$2a$10$...' WHERE name = 'admin';
```

### "Token validation failed"

**Symptoms**:
```
{"unauthorized": {"message": "Token validation failed", "code": 401}}
```

**Causes**:
1. Token expired (default TTL: 24 hours)
2. JWT secret mismatch
3. Malformed token

**Solutions**:

**Check token expiration**:
```bash
# Get new token
openstack token issue

# Use fresh token
export OS_TOKEN=$(openstack token issue -f value -c id)
```

**Verify JWT secret matches**:
```yaml
# config/o3k.yaml
keystone:
  jwt_secret: "your-secret-here"  # Must be same across restarts
```

**Warning**: Changing JWT secret invalidates all existing tokens

### "Project not found" or Scope Errors

**Symptoms**:
```
{"badRequest": {"message": "Project 'default' could not be found", "code": 400}}
```

**Solutions**:

**List available projects**:
```bash
openstack project list
```

**Use correct project scope**:
```bash
export OS_PROJECT_NAME=default
export OS_PROJECT_DOMAIN_NAME=Default
```

---

## API Errors

### 404 Not Found

**Symptoms**:
```
{"itemNotFound": {"message": "Server abc-123 could not be found", "code": 404}}
```

**Causes**:
- Resource doesn't exist
- Wrong resource ID
- Wrong project scope

**Solutions**:

**Verify resource exists**:
```bash
# List resources
openstack server list
openstack volume list
openstack network list

# Check resource ID
openstack server show <server-id>
```

**Check project scope**:
```bash
# Your current project
openstack token issue -f value -c project_id

# Resource's project
openstack server show <server-id> -f value -c project_id
```

**Database check**:
```sql
-- Check if resource exists
SELECT id, name, project_id FROM instances WHERE id = 'abc-123';
```

### 400 Bad Request (Validation Errors)

**Symptoms**:
```
{"badRequest": {
  "message": "Validation failed",
  "details": "Field 'flavor': must be specified"
}}
```

**Solutions**:

**Check required fields**:
```bash
# Example: Server creation requires flavor and image
openstack server create \
  --flavor m1.small \     # Required
  --image cirros \        # Required
  --network my-net \      # Required
  my-server               # Required
```

**Validate field values**:
```bash
# List valid flavors
openstack flavor list

# List valid images
openstack image list

# List valid networks
openstack network list
```

### 409 Conflict

**Symptoms**:
```
{"conflict": {
  "message": "Network 'my-network' already exists",
  "details": "Name must be unique per project"
}}
```

**Solutions**:

**Use unique names** or **delete existing resource**:
```bash
# Option 1: Use different name
openstack network create my-network-2

# Option 2: Delete existing
openstack network delete my-network
openstack network create my-network
```

### 500 Internal Server Error

**Symptoms**:
```
{"computeFault": {"message": "Database error during create operation", "code": 500}}
```

**Causes**:
- Database error
- External service failure (libvirt, Ceph, S3)
- Bug in O3K code

**Solutions**:

1. **Check O3K logs**:
```bash
# If running with Docker Compose
docker compose logs o3k

# If running as binary
journalctl -u o3k -n 100
```

2. **Check database connectivity**:
```bash
psql -U lightstack -d lightstack -c "SELECT 1;"
```

3. **Check external services**:
```bash
# libvirt (if using real mode)
virsh list

# Ceph (if using RBD)
rados df

# S3 (if using S3 mode)
aws s3 ls --endpoint-url http://your-s3-endpoint
```

---

## Performance Issues

### Slow API Responses

**Symptoms**:
- API calls taking > 1 second
- Timeouts in client applications

**Diagnosis**:

**Enable slow query logging**:
```yaml
# config/o3k.yaml - already enabled by default
# Logs appear in application logs
```

**Check connection pool stats**:
```go
stats := database.GetQueryStats()
fmt.Printf("Acquired: %v, Idle: %v, Acquire Duration: %v\n",
    stats["acquired_conns"],
    stats["idle_conns"],
    stats["acquire_duration"])
```

**Solutions**:

1. **Increase connection pool** (if acquire_duration > 50ms):
```yaml
database:
  max_connections: 50  # Increase from 20
```

2. **Add missing indexes** - See DATABASE_OPTIMIZATION.md

3. **Optimize PostgreSQL**:
```ini
# postgresql.conf
shared_buffers = 4GB          # 25% of RAM
effective_cache_size = 12GB   # 75% of RAM
work_mem = 16MB               # RAM / max_connections / 2
```

### High Memory Usage

**Symptoms**:
- O3K process using excessive memory
- OOM (Out of Memory) killer terminating O3K

**Solutions**:

**Reduce connection pool**:
```yaml
database:
  max_connections: 10  # Reduce from 20
```

**Check for memory leaks**:
```bash
# Monitor memory usage
ps aux | grep o3k

# Use pprof for Go profiling
curl http://localhost:8774/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

### High CPU Usage

**Solutions**:

1. **Profile CPU usage**:
```bash
curl http://localhost:8774/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof
```

2. **Check for query inefficiencies** - See slow query logs

3. **Scale horizontally** - Add more O3K instances behind load balancer

---

## Networking Problems

### "Network namespace not found"

**Symptoms**:
```
ERROR: Failed to create network: network namespace qdhcp-abc-123 not found
```

**Causes**:
- Using real networking mode on macOS (not supported)
- Insufficient permissions (need root/sudo)
- netns not enabled in kernel

**Solutions**:

**Use stub mode on macOS**:
```yaml
# config/o3k.yaml
neutron:
  networking_mode: stub  # Use stub instead of iptables
```

**Grant permissions on Linux**:
```bash
# Run O3K as root
sudo ./bin/o3k

# Or use systemd with proper capabilities
```

**Verify netns support**:
```bash
# Check if network namespaces work
sudo ip netns list
sudo ip netns add test-ns
sudo ip netns del test-ns
```

### DHCP Not Working

**Symptoms**:
- VMs not getting IP addresses
- VM stuck at network configuration

**Solutions**:

**Check dnsmasq is running**:
```bash
# List dnsmasq processes
ps aux | grep dnsmasq

# Check namespace-specific dnsmasq
sudo ip netns exec qdhcp-<network-id> ps aux | grep dnsmasq
```

**Verify DHCP configuration**:
```yaml
# config/o3k.yaml
neutron:
  dhcp_lease_time: 24h
```

**Manual DHCP test**:
```bash
# Inside VM
sudo dhclient -v eth0
```

### Floating IPs Not Working

**Symptoms**:
- Cannot ping floating IP from outside
- VM can't reach internet via floating IP

**Solutions**:

**Check external gateway configured**:
```bash
openstack router show my-router -f value -c external_gateway_info
```

**Verify NAT rules**:
```bash
# Get router namespace
ROUTER_NS=$(sudo ip netns list | grep qrouter-)

# Check NAT rules
sudo ip netns exec $ROUTER_NS iptables -t nat -L -n -v
```

**Check routing**:
```bash
# Router should have route to external network
sudo ip netns exec $ROUTER_NS ip route
```

---

## Storage Issues

### "Volume attachment failed"

**Symptoms**:
```
ERROR: Failed to attach volume: volume abc-123 is not in 'available' state
```

**Causes**:
- Volume already attached
- Volume in wrong state
- VM not running

**Solutions**:

**Check volume status**:
```bash
openstack volume show <volume-id> -f value -c status
```

**Valid states for attachment**:
- `available`: Can be attached
- `in-use`: Already attached (detach first)
- `error`: Fix issue and reset state

**Detach volume first**:
```bash
openstack server remove volume <server-id> <volume-id>
```

**Reset volume state** (admin only):
```bash
openstack volume set --state available <volume-id>
```

### Ceph RBD Connection Failed

**Symptoms**:
```
ERROR: Failed to create volume: libvirt service error: Failed to connect to Ceph cluster
```

**Solutions**:

**Check Ceph cluster health**:
```bash
ceph status
ceph health
```

**Verify Ceph configuration**:
```yaml
# config/o3k.yaml
cinder:
  storage_mode: rbd
  ceph_conf: /etc/ceph/ceph.conf
  ceph_pool: volumes

glance:
  storage_mode: rbd
  ceph_conf: /etc/ceph/ceph.conf
  ceph_pool: images
```

**Test Ceph connection**:
```bash
# List pools
rados lspools

# Test pool access
rados -p volumes ls
```

**Check credentials**:
```bash
# List Ceph auth keys
ceph auth list

# Verify keyring exists
cat /etc/ceph/ceph.client.admin.keyring
```

### S3 Storage Failed

**Symptoms**:
```
ERROR: Failed to upload image: External service error: s3: failed to upload
```

**Solutions**:

**Verify S3 configuration**:
```yaml
# config/o3k.yaml
glance:
  storage_mode: s3
  s3_bucket: my-images-bucket
  s3_region: us-east-1
  s3_endpoint: http://minio:9000  # For MinIO
```

**Test S3 connection**:
```bash
aws s3 ls s3://my-images-bucket --endpoint-url http://minio:9000
```

**Check bucket exists**:
```bash
aws s3 mb s3://my-images-bucket --endpoint-url http://minio:9000
```

---

## Compute (VM) Issues

### "Failed to create VM: libvirt connection failed"

**Symptoms**:
```
ERROR: External service error: libvirt: Failed to connect to qemu:///system
```

**Causes**:
- Using real mode on macOS (not supported)
- libvirt not running
- Permission issues

**Solutions**:

**Use stub mode for development**:
```yaml
# config/o3k.yaml
nova:
  libvirt_mode: stub
```

**Check libvirt on Linux**:
```bash
# Check status
sudo systemctl status libvirtd

# Start if needed
sudo systemctl start libvirtd

# Test connection
virsh list
```

**Fix permissions**:
```bash
# Add user to libvirt group
sudo usermod -a -G libvirt $USER

# Re-login for group to take effect
```

### VM Stuck in "BUILD" State

**Symptoms**:
- VM status remains "BUILD" for > 5 minutes
- `openstack server show` shows no error

**Solutions**:

**Check O3K logs for errors**:
```bash
docker compose logs o3k | grep ERROR
```

**Check libvirt domain**:
```bash
virsh list --all
virsh dominfo <vm-name>
```

**Reset VM state** (admin only):
```bash
# Note: This doesn't fix the underlying issue
nova reset-state <server-id> --active
```

### Console Access Not Working

**Symptoms**:
- VNC console returns blank screen
- Console URL inaccessible

**Solutions**:

**Check console type**:
```bash
# Get console URL
openstack console url show <server-id> --novnc

# Try different console types
openstack console url show <server-id> --serial
```

**Verify VM is running**:
```bash
openstack server show <server-id> -f value -c status
# Should be "ACTIVE"
```

---

## Configuration Problems

### "Config file not found"

**Symptoms**:
```
FATAL: Failed to read config file: no such file or directory
```

**Solutions**:

**Specify config path**:
```bash
./bin/o3k --config /path/to/o3k.yaml
```

**Use default path**:
```bash
# O3K looks for config in these locations (in order):
# 1. ./config/o3k.yaml (current directory)
# 2. /etc/o3k/o3k.yaml (system-wide)

mkdir -p config
cp config/o3k.yaml.example config/o3k.yaml
```

### "Invalid configuration"

**Symptoms**:
```
FATAL: Failed to parse config file: yaml: unmarshal errors
```

**Solutions**:

**Validate YAML syntax**:
```bash
# Use yamllint
yamllint config/o3k.yaml

# Or online validator
# https://www.yamllint.com/
```

**Check required fields**:
```yaml
# Minimum required configuration
database:
  url: "postgres://..."

keystone:
  jwt_secret: "change-me"  # MUST change in production!
```

### "WARNING: Using default JWT secret"

**Symptoms**:
```
WARNING: Using default JWT secret! Set O3K_JWT_SECRET environment variable in production.
```

**Solution** (CRITICAL for security):

```bash
# Generate secure random secret
SECRET=$(openssl rand -base64 32)

# Set in config
sed -i "s/jwt_secret: .*/jwt_secret: \"$SECRET\"/" config/o3k.yaml

# Or use environment variable
export O3K_JWT_SECRET="$SECRET"
```

**Warning**: Changing the secret invalidates all existing tokens. Users must re-authenticate.

---

## Getting Help

### Collect Diagnostic Information

When reporting issues, include:

1. **O3K Version**:
```bash
./bin/o3k --version
```

2. **Configuration** (redact secrets!):
```bash
cat config/o3k.yaml | grep -v password | grep -v secret
```

3. **Logs**:
```bash
# Last 100 lines
docker compose logs o3k --tail 100

# Or systemd
journalctl -u o3k -n 100
```

4. **Database status**:
```bash
psql -U lightstack -d lightstack -c "SELECT version();"
psql -U lightstack -d lightstack -c "SELECT count(*) FROM instances;"
```

5. **System information**:
```bash
uname -a
docker --version  # If using Docker
cat /etc/os-release
```

### Report Issues

**GitHub Issues**: https://github.com/cobaltcore-dev/o3k/issues

Include:
- Clear description of the problem
- Steps to reproduce
- Expected vs actual behavior
- Diagnostic information (above)
- Relevant log snippets

### Community Support

- **Documentation**: `/docs` directory
- **Examples**: `test/` directory (integration tests show usage patterns)

---

## Quick Reference

### Common Commands

```bash
# Check O3K status
./bin/o3k --version
systemctl status o3k

# View logs
docker compose logs o3k -f
journalctl -u o3k -f

# Database operations
psql -U lightstack -d lightstack
psql -U lightstack -d lightstack -c "SELECT count(*) FROM instances;"

# Test API connectivity
curl http://localhost:35357/v3
openstack token issue

# Reset everything (DANGEROUS!)
docker compose down -v
docker compose up -d
```

### Default Ports

| Service | Port | URL |
|---------|------|-----|
| Keystone | 35357 | http://localhost:35357/v3 |
| Nova | 8774 | http://localhost:8774/v2.1 |
| Neutron | 9696 | http://localhost:9696/v2.0 |
| Cinder | 8776 | http://localhost:8776/v3 |
| Glance | 9292 | http://localhost:9292/v2 |
| Metadata | 8775 | http://localhost:8775 |

### Default Credentials

```bash
export OS_AUTH_URL=http://localhost:35357/v3
export OS_PROJECT_NAME=default
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default
```

**⚠️ Change password in production!**

---

**Last Updated**: March 13, 2026
**License**: Apache License 2.0
