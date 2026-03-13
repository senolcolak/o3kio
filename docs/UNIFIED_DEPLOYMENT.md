# Unified O3K + Horizon Deployment Guide

**O3K Version**: v0.5.0+
**OpenStack Release**: Flamingo (2025.2)
**Last Updated**: 2026-03-13
**Status**: Production Ready

## Overview

This guide provides a complete, unified deployment of O3K and OpenStack Horizon dashboard in a single Docker Compose stack. This is the **recommended deployment method** for getting a complete OpenStack environment with web UI.

### What's Included

- **PostgreSQL 18**: Database backend
- **O3K Services**: All five core OpenStack services (Keystone, Nova, Neutron, Cinder, Glance)
- **Horizon Dashboard**: OpenStack Flamingo (2025.2) web interface
- **noVNC Proxy**: VNC console access for instances

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Docker Network (o3k-horizon-network)      │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────┐   ┌────────────────┐   ┌─────────────────┐  │
│  │PostgreSQL│◄──│  O3K Services  │◄──│     Horizon     │  │
│  │  :5432   │   │   :35357-9292  │   │      :80        │  │
│  └──────────┘   └────────────────┘   └─────────────────┘  │
│                          ▲                                   │
│                          │                                   │
│                  ┌───────┴────────┐                         │
│                  │  noVNC Proxy   │                         │
│                  │     :6080      │                         │
│                  └────────────────┘                         │
└─────────────────────────────────────────────────────────────┘
         │              │              │              │
    Port 5432      Port 35357    Port 80         Port 6080
                   (+ 8774,      (HTTP)          (noVNC)
                    9696, 8776,
                    9292, 8775)
```

## Prerequisites

### Required

- **Docker Engine** 20.10+
- **Docker Compose** 2.0+
- **Available Ports**:
  - 80 (HTTP - Horizon dashboard)
  - 5432 (PostgreSQL)
  - 6080 (noVNC)
  - 35357 (Keystone)
  - 8774 (Nova)
  - 8775 (Metadata)
  - 8776 (Cinder)
  - 9292 (Glance)
  - 9696 (Neutron)
- **System Resources**:
  - 4GB RAM minimum (8GB recommended)
  - 20GB disk space minimum
  - 2 CPU cores minimum (4 recommended)

### Platform Support

- ✅ **Linux x86_64 (AMD64)**: Full support (all services)
- ⚠️ **Linux ARM64**: O3K works natively; Horizon/noVNC run via emulation (slower)
- ✅ **macOS Intel**: Full support (Docker Desktop with Rosetta)
- ⚠️ **macOS Apple Silicon (ARM64)**: O3K works natively; Horizon/noVNC require emulation
  - **Note**: Horizon and noVNC are x86_64 images and will run under emulation
  - O3K itself is built as ARM64 and runs natively
  - For ARM64-only environments, use API-only deployment (see Alternative Deployment Options below)
- ⚠️ **Windows**: WSL2 required

## Quick Start (5 Minutes)

### Step 1: Clone Repository

```bash
git clone https://github.com/cobaltcore-dev/o3k.git
cd o3k/deployments
```

### Step 2: Review Configuration (Optional)

The deployment includes pre-configured settings suitable for development and testing:

```bash
# View Docker Compose configuration
cat docker-compose-horizon.yml

# View Horizon configuration
cat horizon-config/local_settings
```

**Default Settings**:
- Database: PostgreSQL on port 5432
- O3K Mode: Stub (no real VMs, suitable for testing)
- Horizon: Flamingo (2025.2) on port 80
- Default Credentials: admin/secret

### Step 3: Start All Services

```bash
docker compose -f docker-compose-horizon.yml up -d
```

**Expected Output**:
```
[+] Running 5/5
 ✔ Network o3k-horizon-network    Created
 ✔ Container o3k-postgres         Started
 ✔ Container o3k                  Started
 ✔ Container o3k-novnc            Started
 ✔ Container o3k-horizon          Started
```

### Step 4: Wait for Services to Be Ready

```bash
# Check container status
docker compose -f docker-compose-horizon.yml ps

# Watch logs until Horizon is ready
docker compose -f docker-compose-horizon.yml logs -f horizon

# Press Ctrl+C when you see "WSGI app 0 (mountpoint='/dashboard') ready"
```

Wait time: ~30-60 seconds depending on your system.

### Step 5: Access Horizon Dashboard

Open your browser to:
```
http://localhost/dashboard
```

**Login Credentials**:
- Domain: `Default`
- User Name: `admin`
- Password: `secret`

🎉 **You're done!** You now have a fully functional OpenStack environment with web UI.

## Alternative Deployment Options

### API-Only Deployment (ARM64 Native)

If you're on ARM64 (Apple Silicon, ARM servers) and want to avoid emulation overhead, use the API-only deployment which runs O3K natively:

```bash
# Use the standard docker-compose without Horizon
docker compose -f docker-compose.yml up -d

# Access via OpenStack CLI
export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_DOMAIN_NAME=Default
openstack token issue
```

This gives you full OpenStack API access with native ARM64 performance, without the web UI.

### Native Horizon (x86_64 Only)

For x86_64 systems or when performance is critical, the unified deployment (`docker-compose-horizon.yml`) provides the best experience with native performance for all components.

## Detailed Configuration

### Environment Variables

You can customize the deployment by setting environment variables before starting:

```bash
# Database configuration
export O3K_DB_PASSWORD=your-secure-password
export O3K_DB_NAME=o3k
export O3K_DB_USER=o3k

# JWT secret for O3K authentication
export O3K_JWT_SECRET=your-secure-jwt-secret-here

# O3K modes
export O3K_NOVA_MODE=stub      # or 'real' for actual VMs (Linux only)
export O3K_NEUTRON_MODE=stub   # or 'iptables' for real networking
export O3K_CINDER_MODE=local   # or 'rbd', 's3'
export O3K_GLANCE_MODE=local   # or 'rbd', 's3'

# Horizon configuration
export HORIZON_SECRET_KEY=your-horizon-secret-key
```

### Production Configuration

For production deployments, update these settings:

#### 1. Security Settings

**Database Password** - Edit `docker-compose-horizon.yml`:

```yaml
services:
  postgres:
    environment:
      POSTGRES_PASSWORD: CHANGE_THIS_PASSWORD

  o3k:
    environment:
      - O3K_DATABASE_URL=postgres://o3k:CHANGE_THIS_PASSWORD@postgres:5432/o3k?sslmode=disable
```

**JWT Secret** - Edit `docker-compose-horizon.yml`:

```yaml
services:
  o3k:
    environment:
      - O3K_KEYSTONE_JWT_SECRET=GENERATE_SECURE_RANDOM_STRING_HERE
```

Generate a secure JWT secret:
```bash
openssl rand -hex 32
```

**Horizon Secret Key** - Edit `horizon-config/local_settings`:

```python
import secrets
SECRET_KEY = secrets.token_hex(64)
```

#### 2. Enable HTTPS

Use a reverse proxy (nginx, traefik) with SSL certificates:

```yaml
# Add to docker-compose-horizon.yml
services:
  nginx:
    image: nginx:alpine
    container_name: o3k-nginx
    ports:
      - "443:443"
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./certs:/etc/nginx/certs:ro
    networks:
      - o3k-network
    depends_on:
      - horizon
```

Update `horizon-config/local_settings`:

```python
ALLOWED_HOSTS = ['your-domain.com']
SESSION_COOKIE_SECURE = True
CSRF_COOKIE_SECURE = True
```

#### 3. Real Mode (Linux Only)

For production with actual VMs:

```yaml
services:
  o3k:
    environment:
      - O3K_NOVA_LIBVIRT_MODE=real
      - O3K_NEUTRON_NETWORKING_MODE=iptables
    privileged: true  # Required for networking
    volumes:
      - /var/run/libvirt/libvirt-sock:/var/run/libvirt/libvirt-sock
```

**Prerequisites**:
- libvirt + KVM installed on host
- User has permissions for libvirt socket

### Storage Backends

#### Local Storage (Default)

Suitable for development and small deployments:

```yaml
services:
  o3k:
    environment:
      - O3K_CINDER_STORAGE_MODE=local
      - O3K_GLANCE_STORAGE_MODE=local
    volumes:
      - o3k-images:/var/lib/o3k/images
      - o3k-volumes:/var/lib/o3k/volumes
```

#### Ceph RBD Storage

For production with Ceph cluster:

```yaml
services:
  o3k:
    environment:
      - O3K_CINDER_STORAGE_MODE=rbd
      - O3K_GLANCE_STORAGE_MODE=rbd
      - O3K_CEPH_CONF=/etc/ceph/ceph.conf
      - O3K_CEPH_POOL_VOLUMES=o3k-volumes
      - O3K_CEPH_POOL_IMAGES=o3k-images
    volumes:
      - /etc/ceph:/etc/ceph:ro
```

#### S3 Storage

For AWS S3 or MinIO:

```yaml
services:
  o3k:
    environment:
      - O3K_CINDER_STORAGE_MODE=s3
      - O3K_GLANCE_STORAGE_MODE=s3
      - O3K_S3_ENDPOINT=https://s3.amazonaws.com
      - O3K_S3_ACCESS_KEY=your-access-key
      - O3K_S3_SECRET_KEY=your-secret-key
      - O3K_S3_BUCKET_VOLUMES=o3k-volumes
      - O3K_S3_BUCKET_IMAGES=o3k-images
```

## Operations

### Starting Services

```bash
# Start all services
docker compose -f docker-compose-horizon.yml up -d

# Start specific service
docker compose -f docker-compose-horizon.yml up -d horizon

# Start with logs
docker compose -f docker-compose-horizon.yml up
```

### Stopping Services

```bash
# Stop all services (preserves data)
docker compose -f docker-compose-horizon.yml stop

# Stop and remove containers (preserves volumes)
docker compose -f docker-compose-horizon.yml down

# Stop and remove everything including data
docker compose -f docker-compose-horizon.yml down -v
```

### Viewing Logs

```bash
# All services
docker compose -f docker-compose-horizon.yml logs -f

# Specific service
docker compose -f docker-compose-horizon.yml logs -f horizon
docker compose -f docker-compose-horizon.yml logs -f o3k

# Last 100 lines
docker compose -f docker-compose-horizon.yml logs --tail 100 horizon
```

### Checking Status

```bash
# Container status
docker compose -f docker-compose-horizon.yml ps

# Health status
docker compose -f docker-compose-horizon.yml ps --format "table {{.Name}}\t{{.Status}}\t{{.Health}}"

# Resource usage
docker stats --no-stream
```

### Restarting Services

```bash
# Restart all
docker compose -f docker-compose-horizon.yml restart

# Restart specific service
docker compose -f docker-compose-horizon.yml restart horizon

# Rebuild and restart (after config changes)
docker compose -f docker-compose-horizon.yml up -d --build o3k
```

### Accessing Container Shells

```bash
# O3K container
docker exec -it o3k /bin/sh

# Horizon container
docker exec -it o3k-horizon /bin/bash

# PostgreSQL container
docker exec -it o3k-postgres psql -U o3k
```

## Using OpenStack CLI

### Installing OpenStack CLI

```bash
# macOS
brew install openstackclient

# Linux (Ubuntu/Debian)
sudo apt install python3-openstackclient

# Using pip
pip install python-openstackclient
```

### Configure Environment

```bash
# Option 1: Export variables
export OS_AUTH_URL=http://localhost:35357/v3
export OS_PROJECT_NAME=default
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default
export OS_IDENTITY_API_VERSION=3

# Option 2: Create RC file
cat > ~/o3k-openrc.sh <<'EOF'
export OS_AUTH_URL=http://localhost:35357/v3
export OS_PROJECT_NAME=default
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default
export OS_IDENTITY_API_VERSION=3
EOF

source ~/o3k-openrc.sh
```

### Test Commands

```bash
# Test authentication
openstack token issue

# List endpoints
openstack catalog list

# Compute operations
openstack flavor list
openstack image list
openstack server list

# Network operations
openstack network list
openstack subnet list
openstack router list

# Volume operations
openstack volume list
openstack volume snapshot list
```

## Verification and Testing

### 1. Health Checks

```bash
# Check all containers are healthy
docker compose -f docker-compose-horizon.yml ps

# Expected: All containers show "healthy" status
```

### 2. API Endpoints

```bash
# Keystone (Identity)
curl -s http://localhost:35357/v3 | jq

# Nova (Compute)
curl -s http://localhost:8774/ | jq

# Neutron (Network)
curl -s http://localhost:9696/ | jq

# Cinder (Block Storage)
curl -s http://localhost:8776/ | jq

# Glance (Image)
curl -s http://localhost:9292/ | jq
```

### 3. Horizon Dashboard

1. Open http://localhost/dashboard
2. Login with admin/secret
3. Check "Overview" page loads
4. Verify "Compute > Instances" shows empty list
5. Click "Launch Instance" to test wizard
6. Check "Network > Network Topology" loads

### 4. Full Workflow Test

```bash
# Source environment
source ~/o3k-openrc.sh

# Create network
openstack network create test-net
openstack subnet create test-subnet \
  --network test-net \
  --subnet-range 10.0.0.0/24

# Create instance (stub mode returns fake instance)
openstack server create \
  --flavor m1.small \
  --image cirros \
  --network test-net \
  test-vm

# List instances
openstack server list

# Create volume
openstack volume create --size 1 test-volume

# List volumes
openstack volume list

# Cleanup
openstack server delete test-vm
openstack volume delete test-volume
openstack subnet delete test-subnet
openstack network delete test-net
```

## Troubleshooting

### Platform-Specific Issues

#### ARM64 (Apple Silicon, ARM Servers)

**Symptom**: Horizon or noVNC containers restart repeatedly

**Diagnosis**:
```bash
docker compose -f docker-compose-horizon.yml ps
# Look for "platform (linux/amd64) does not match" warnings
```

**Explanation**: Horizon (OpenStack Kolla) and noVNC are x86_64 images. On ARM64 systems, Docker runs them via emulation (QEMU). This works but may be slower and less stable.

**Solutions**:
1. **Continue with emulation** (works but slower):
   - Horizon may take 2-3x longer to start
   - Slightly higher resource usage
   - Fully functional once started

2. **Use API-only deployment** (recommended for ARM64):
   ```bash
   docker compose -f docker-compose.yml up -d
   # Use OpenStack CLI instead of web UI
   ```

3. **Use native ARM64 Horizon** (future):
   - Track: https://github.com/cobaltcore-dev/o3k/issues/XXX
   - Custom ARM64 Horizon build in development

**Verification**:
```bash
# Check O3K (should be ARM64 and healthy)
docker inspect o3k | jq '.[0].Architecture'  # Should show "arm64"
docker compose -f docker-compose-horizon.yml ps o3k  # Should be "healthy"

# O3K works natively on ARM64, only Horizon requires emulation
docker exec o3k curl -s http://localhost:35357/v3 | jq '.version.status'
```

### Service Won't Start

**Symptom**: Container exits immediately after start

**Diagnosis**:
```bash
docker compose -f docker-compose-horizon.yml logs o3k
```

**Common Issues**:

1. **Port already in use**:
   ```
   Error: bind: address already in use
   ```
   Solution: Change port mapping in docker-compose-horizon.yml or stop conflicting service

2. **Database connection failed**:
   ```
   Error: failed to connect to postgres
   ```
   Solution: Ensure postgres container is healthy:
   ```bash
   docker compose -f docker-compose-horizon.yml ps postgres
   docker compose -f docker-compose-horizon.yml logs postgres
   ```

3. **Migration failed**:
   ```
   Error: migration failed
   ```
   Solution: Reset database:
   ```bash
   docker compose -f docker-compose-horizon.yml down -v
   docker compose -f docker-compose-horizon.yml up -d
   ```

### Horizon Shows Connection Errors

**Symptom**: "Unable to retrieve..." errors in Horizon

**Diagnosis**:
```bash
# Check Horizon can reach O3K
docker exec o3k-horizon curl -s http://o3k:35357/v3 | jq

# Check network connectivity
docker exec o3k-horizon ping -c 3 o3k
```

**Solutions**:

1. **Verify O3K is running**:
   ```bash
   docker compose -f docker-compose-horizon.yml ps o3k
   curl -s http://localhost:35357/v3 | jq
   ```

2. **Check Horizon configuration**:
   ```bash
   docker exec o3k-horizon cat /etc/openstack-dashboard/local_settings.py | grep OPENSTACK_HOST
   # Should show: OPENSTACK_HOST = "o3k"
   ```

3. **Restart Horizon**:
   ```bash
   docker compose -f docker-compose-horizon.yml restart horizon
   ```

### VNC Console Doesn't Work

**Symptom**: Console tab shows "Unable to connect"

**Diagnosis**:
```bash
# Check noVNC is running
docker compose -f docker-compose-horizon.yml ps novnc
curl -I http://localhost:6080

# Check O3K can reach noVNC
docker exec o3k curl -I http://novnc:6080
```

**Solutions**:

1. **Verify noVNC container**:
   ```bash
   docker compose -f docker-compose-horizon.yml logs novnc
   docker compose -f docker-compose-horizon.yml restart novnc
   ```

2. **Check port mapping**:
   ```bash
   docker ps | grep novnc
   # Should show: 0.0.0.0:6080->6080/tcp
   ```

### Horizon Performance Issues

**Symptom**: Dashboard is slow to load

**Solutions**:

1. **Increase container resources**:
   ```yaml
   services:
     horizon:
       deploy:
         resources:
           limits:
             memory: 4G
             cpus: '2'
   ```

2. **Check system resources**:
   ```bash
   docker stats --no-stream
   ```

3. **Enable memcached** (add to docker-compose-horizon.yml):
   ```yaml
   services:
     memcached:
       image: memcached:alpine
       container_name: o3k-memcached
       networks:
         - o3k-network
   ```

   Update horizon-config/local_settings:
   ```python
   CACHES = {
       'default': {
           'BACKEND': 'django.core.cache.backends.memcached.PyMemcacheCache',
           'LOCATION': 'memcached:11211',
       }
   }
   ```

### Database Issues

**Symptom**: Database errors in O3K logs

**Diagnosis**:
```bash
docker compose -f docker-compose-horizon.yml logs postgres
docker exec o3k-postgres psql -U o3k -c "\dt"
```

**Solutions**:

1. **Check database connectivity**:
   ```bash
   docker exec o3k-postgres pg_isready -U o3k
   ```

2. **Verify database schema**:
   ```bash
   docker exec o3k-postgres psql -U o3k -c "\dt" | wc -l
   # Should show ~20+ tables
   ```

3. **Reset database** (WARNING: Destroys all data):
   ```bash
   docker compose -f docker-compose-horizon.yml down -v
   docker volume rm deployments_postgres-data
   docker compose -f docker-compose-horizon.yml up -d
   ```

## Upgrading

### Upgrade O3K

```bash
# Pull latest code
cd ~/git/o3k
git pull

# Rebuild O3K image
cd deployments
docker compose -f docker-compose-horizon.yml build o3k

# Restart with new image
docker compose -f docker-compose-horizon.yml up -d o3k
```

### Upgrade Horizon

```bash
# Edit docker-compose-horizon.yml
# Change image version:
# FROM: quay.io/openstack.kolla/horizon:2025.2-ubuntu-noble
# TO:   quay.io/openstack.kolla/horizon:2026.1-ubuntu-noble

# Pull new image
docker compose -f docker-compose-horizon.yml pull horizon

# Restart Horizon
docker compose -f docker-compose-horizon.yml up -d horizon
```

### Database Migration

O3K automatically runs migrations on startup. To manually run migrations:

```bash
docker exec o3k /app/o3k migrate
```

## Backup and Restore

### Backup

```bash
# Create backup directory
mkdir -p ~/o3k-backups/$(date +%Y%m%d)

# Backup database
docker exec o3k-postgres pg_dump -U o3k o3k > \
  ~/o3k-backups/$(date +%Y%m%d)/database.sql

# Backup configuration
cp docker-compose-horizon.yml ~/o3k-backups/$(date +%Y%m%d)/
cp -r horizon-config ~/o3k-backups/$(date +%Y%m%d)/

# Backup volumes (if using local storage)
docker run --rm \
  -v deployments_o3k-images:/data \
  -v ~/o3k-backups/$(date +%Y%m%d):/backup \
  alpine tar czf /backup/images.tar.gz /data

docker run --rm \
  -v deployments_o3k-volumes:/data \
  -v ~/o3k-backups/$(date +%Y%m%d):/backup \
  alpine tar czf /backup/volumes.tar.gz /data
```

### Restore

```bash
# Restore database
cat ~/o3k-backups/20260313/database.sql | \
  docker exec -i o3k-postgres psql -U o3k o3k

# Restore configuration
cp ~/o3k-backups/20260313/docker-compose-horizon.yml .
cp -r ~/o3k-backups/20260313/horizon-config .

# Restore volumes
docker run --rm \
  -v deployments_o3k-images:/data \
  -v ~/o3k-backups/20260313:/backup \
  alpine tar xzf /backup/images.tar.gz -C /

docker run --rm \
  -v deployments_o3k-volumes:/data \
  -v ~/o3k-backups/20260313:/backup \
  alpine tar xzf /backup/volumes.tar.gz -C /

# Restart services
docker compose -f docker-compose-horizon.yml restart
```

## Monitoring

### Container Metrics

```bash
# Real-time stats
docker stats

# Resource limits
docker inspect o3k --format='{{.HostConfig.Memory}}'
docker inspect o3k --format='{{.HostConfig.NanoCpus}}'
```

### Application Metrics

```bash
# O3K metrics endpoint (if enabled)
curl http://localhost:9090/metrics

# Database connections
docker exec o3k-postgres psql -U o3k -c \
  "SELECT count(*) FROM pg_stat_activity;"

# Horizon request logs
docker compose -f docker-compose-horizon.yml logs horizon | \
  grep "GET /dashboard" | tail -20
```

### Health Monitoring

```bash
# Automated health check script
cat > health-check.sh <<'EOF'
#!/bin/bash
set -e

echo "Checking O3K services..."
curl -f http://localhost:35357/v3 > /dev/null || exit 1
echo "✓ Keystone OK"

curl -f http://localhost:8774/ > /dev/null || exit 1
echo "✓ Nova OK"

curl -f http://localhost:9696/ > /dev/null || exit 1
echo "✓ Neutron OK"

curl -f http://localhost:80/dashboard/auth/login/ > /dev/null || exit 1
echo "✓ Horizon OK"

echo "All services healthy!"
EOF

chmod +x health-check.sh
./health-check.sh
```

## Production Deployment Checklist

- [ ] Change default passwords (postgres, admin user)
- [ ] Generate secure JWT secret
- [ ] Generate secure Horizon SECRET_KEY
- [ ] Enable HTTPS with valid certificates
- [ ] Configure ALLOWED_HOSTS in Horizon
- [ ] Enable secure cookies (SESSION_COOKIE_SECURE, CSRF_COOKIE_SECURE)
- [ ] Set up backup schedule
- [ ] Configure monitoring and alerting
- [ ] Review and harden firewall rules
- [ ] Set resource limits on containers
- [ ] Configure log rotation
- [ ] Test disaster recovery procedures
- [ ] Document custom configuration
- [ ] Set up high availability (if required)
- [ ] Configure external storage backend (Ceph, S3)
- [ ] Review and apply security updates regularly

## Integration with Existing Infrastructure

### External Database

Use an existing PostgreSQL instance:

```yaml
services:
  o3k:
    environment:
      - O3K_DATABASE_URL=postgres://user:pass@external-db.example.com:5432/o3k?sslmode=require

# Remove postgres service
```

### External Load Balancer

Place behind load balancer:

```yaml
services:
  horizon:
    # Remove port mapping
    # ports:
    #   - "80:80"
    expose:
      - "80"
    environment:
      - USE_X_FORWARDED_FOR=True
```

Configure load balancer:
- Backend: http://horizon:80
- Health check: /dashboard/auth/login/
- Session affinity: Enabled

### Container Orchestration

For Kubernetes deployment, see separate guide: `docs/KUBERNETES_DEPLOYMENT.md` (coming soon)

## Support and Resources

- **Documentation**: https://github.com/cobaltcore-dev/o3k/tree/main/docs
- **Issues**: https://github.com/cobaltcore-dev/o3k/issues
- **OpenStack Horizon Docs**: https://docs.openstack.org/horizon/latest/
- **O3K Project**: https://github.com/cobaltcore-dev/o3k

## FAQ

**Q: Can I use this in production?**
A: Yes, with proper security hardening (HTTPS, secure passwords, resource limits).

**Q: Does this work on Apple Silicon (M1/M2)?**
A: Yes, containers run via Rosetta emulation. Native ARM64 support coming soon.

**Q: Can I run real VMs?**
A: Yes, on Linux with libvirt/KVM. Set O3K_NOVA_LIBVIRT_MODE=real.

**Q: How do I add more users?**
A: Use OpenStack CLI or Horizon's Identity panel to create users.

**Q: Can I integrate with Active Directory?**
A: Yes, configure Keystone LDAP backend (see O3K docs).

**Q: What's the difference between stub and real mode?**
A: Stub mode returns fake data (for testing). Real mode creates actual VMs and networks.

**Q: How do I upgrade to newer OpenStack release?**
A: Update Horizon image tag in docker-compose-horizon.yml to desired release.

## License

This deployment configuration is part of the O3K project and is licensed under Apache License 2.0.

---

**Last Updated**: 2026-03-13 | **O3K Version**: v0.5.0 | **Horizon Version**: Flamingo (2025.2)
