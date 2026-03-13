# Quickstart: Deploy Horizon with O3K

**Feature**: OpenStack Horizon 100% Compatibility
**Time to Complete**: 15-30 minutes
**Target Audience**: Cloud administrators, developers evaluating O3K

## Overview

This guide walks through deploying OpenStack Horizon dashboard with O3K as the backend cloud platform. After completion, you'll have a fully functional web-based cloud management interface.

**What You'll Get**:
- Horizon dashboard accessible at http://localhost
- Full instance lifecycle management (create, start, stop, delete)
- Network management (networks, subnets, floating IPs)
- Volume management (create, attach, snapshot)
- Image management (upload, download, launch)
- VNC console access to instances (with noVNC proxy)

---

## Prerequisites

### Required Software

- **Docker** (version 20.10+)
- **Docker Compose** (version 2.0+)
- **4GB RAM** minimum (8GB recommended)
- **10GB disk space** available

### Check Prerequisites

```bash
# Verify Docker
docker --version
# Expected: Docker version 20.10.0 or higher

# Verify Docker Compose
docker compose version
# Expected: Docker Compose version v2.0.0 or higher

# Check available disk space
df -h
# Ensure at least 10GB free
```

---

## Step 1: Deploy O3K Backend

### 1.1 Clone O3K Repository (if not already done)

```bash
cd ~/projects
git clone https://github.com/yourusername/o3k.git
cd o3k
```

### 1.2 Start O3K Services

```bash
# Start PostgreSQL database and O3K
docker compose -f deployments/docker-compose.yml up -d

# Verify services running
docker compose -f deployments/docker-compose.yml ps

# Expected output:
# NAME                 STATUS    PORTS
# o3k-postgres         Up        5432/tcp
# o3k                  Up        0.0.0.0:35357->35357/tcp, ...
```

### 1.3 Verify O3K API Accessibility

```bash
# Test Keystone endpoint
curl -s http://localhost:35357/v3 | jq '.version'

# Expected output:
# {
#   "id": "v3.14",
#   "status": "stable"
# }

# Configure OpenStack CLI environment
cat > ~/.o3k-env <<EOF
export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default
export OS_IDENTITY_API_VERSION=3
EOF

source ~/.o3k-env

# Test authentication
openstack token issue

# Expected: Token details displayed
```

---

## Step 2: Deploy Horizon Dashboard

### 2.1 Create Horizon Configuration

Create a directory for Horizon configuration:

```bash
mkdir -p ~/o3k-horizon/config
cd ~/o3k-horizon
```

Create `config/local_settings.py`:

```python
# config/local_settings.py
import os

# Horizon Settings
WEBROOT = '/dashboard/'
LOGIN_URL = '/dashboard/auth/login/'
LOGOUT_URL = '/dashboard/auth/logout/'
LOGIN_REDIRECT_URL = '/dashboard/'

# OpenStack API Configuration
OPENSTACK_HOST = "o3k"  # Docker service name
OPENSTACK_KEYSTONE_URL = "http://%s:35357/v3" % OPENSTACK_HOST
OPENSTACK_KEYSTONE_DEFAULT_ROLE = "_member_"

# API Versions
OPENSTACK_API_VERSIONS = {
    "identity": 3,
    "image": 2,
    "volume": 3,
    "compute": 2.1,
}

# Keystone v3 Configuration
OPENSTACK_KEYSTONE_MULTIDOMAIN_SUPPORT = True
OPENSTACK_KEYSTONE_DEFAULT_DOMAIN = "Default"

# Session Timeout (match O3K token TTL)
SESSION_TIMEOUT = 14400  # 4 hours (matches O3K default)

# Enable Console Access
CONSOLE_TYPE = 'novnc'

# Regional Settings
OPENSTACK_KEYSTONE_REGION_NAME = "RegionOne"
TIME_ZONE = "UTC"

# Security Settings
SECRET_KEY = 'change-this-in-production-12345'
SESSION_COOKIE_SECURE = False  # Set True in production with HTTPS
CSRF_COOKIE_SECURE = False     # Set True in production with HTTPS

# Disable Volume Backup Service (if not using Cinder backup)
OPENSTACK_CINDER_FEATURES = {
    'enable_backup': False,
}

# Enable Neutron Features
OPENSTACK_NEUTRON_NETWORK = {
    'enable_router': True,
    'enable_quotas': True,
    'enable_ipv6': False,  # Set True if using IPv6
    'enable_distributed_router': False,
    'enable_ha_router': False,
    'enable_fip_topology_check': True,
}

# Logging
DEBUG = False
```

### 2.2 Create Docker Compose Configuration

Create `docker-compose.yml` in `~/o3k-horizon/`:

```yaml
version: '3.8'

services:
  horizon:
    image: kolla/ubuntu-horizon:zed
    container_name: o3k-horizon
    ports:
      - "80:80"
    environment:
      - KOLLA_INSTALL_TYPE=source
    volumes:
      - ./config/local_settings.py:/etc/openstack-dashboard/local_settings.py:ro
    networks:
      - o3k-network
    depends_on:
      - novnc
    restart: unless-stopped

  novnc:
    image: novnc/noVNC:latest
    container_name: o3k-novnc
    ports:
      - "6080:6080"
    environment:
      - VNC_HOST=o3k
      - VNC_PORT=5900
    networks:
      - o3k-network
    restart: unless-stopped

networks:
  o3k-network:
    external: true
    name: o3k_default
```

**Note**: Assumes O3K Docker Compose created `o3k_default` network. If different, adjust network name.

### 2.3 Start Horizon

```bash
# From ~/o3k-horizon directory
docker compose up -d

# Verify containers running
docker compose ps

# Expected output:
# NAME            STATUS    PORTS
# o3k-horizon     Up        0.0.0.0:80->80/tcp
# o3k-novnc       Up        0.0.0.0:6080->6080/tcp

# Check Horizon logs
docker compose logs horizon | tail -20

# Wait for Horizon to initialize (30-60 seconds)
sleep 60
```

---

## Step 3: Access Horizon Dashboard

### 3.1 Open Horizon in Browser

```bash
# Open in default browser (macOS)
open http://localhost/dashboard

# Linux with xdg-open
xdg-open http://localhost/dashboard

# Or manually navigate to: http://localhost/dashboard
```

### 3.2 Login

**Credentials** (from O3K seed data):
- **Domain**: Default
- **User Name**: admin
- **Password**: secret

Click **Sign In**

### 3.3 Verify Dashboard Load

Expected view after login:
- **Overview** panel showing hypervisor statistics
- **Compute** menu with Instances, Images, Key Pairs
- **Network** menu with Networks, Routers, Security Groups
- **Volumes** menu with Volumes, Snapshots

**Troubleshooting**: If dashboard doesn't load or shows errors, see [Step 6: Troubleshooting](#step-6-troubleshooting).

---

## Step 4: Verify Core Functionality

### 4.1 Launch Test Instance

1. Navigate to **Compute → Instances**
2. Click **Launch Instance**
3. Fill in wizard:
   - **Instance Name**: test-vm
   - **Source**: Select boot source → Image → Select "cirros" (if available)
   - **Flavor**: m1.small
   - **Network**: Select default network
4. Click **Launch Instance**
5. Verify instance appears with status **Active**

### 4.2 Test VNC Console

1. Click on instance name **test-vm**
2. Click **Console** tab
3. noVNC console window should open showing VM boot process
4. Login prompt should appear (for cirros: user=cirros, pass=gocubsgo)

**Troubleshooting**: If console shows "Connection failed", see [Step 6.3](#63-console-not-available).

### 4.3 Test Network Operations

1. Navigate to **Network → Networks**
2. Click **Create Network**
3. Fill in details:
   - **Network Name**: test-network
   - **Subnet Name**: test-subnet
   - **Network Address**: 192.168.100.0/24
4. Click **Create**
5. Verify network appears in list

### 4.4 Test Volume Operations

1. Navigate to **Volumes → Volumes**
2. Click **Create Volume**
3. Fill in details:
   - **Volume Name**: test-volume
   - **Size (GiB)**: 1
4. Click **Create Volume**
5. Verify volume status changes to **Available**

---

## Step 5: Configuration Reference

### 5.1 CORS Configuration (if needed)

If Horizon runs on different hostname/port than O3K, configure CORS in O3K:

**Edit**: `o3k/config/o3k.yaml`

```yaml
cors:
  enabled: true
  allowed_origins:
    - "http://localhost"
    - "http://horizon"
    - "http://your-horizon-domain.com"
  allowed_methods:
    - GET
    - POST
    - PUT
    - PATCH
    - DELETE
  allowed_headers:
    - "*"
  expose_headers:
    - "X-Subject-Token"
```

**Restart O3K**:
```bash
cd ~/projects/o3k
docker compose -f deployments/docker-compose.yml restart o3k
```

### 5.2 Token TTL Tuning

Match Horizon session timeout with O3K token TTL:

**O3K Configuration** (`config/o3k.yaml`):
```yaml
keystone:
  jwt_secret: "production-secret-key-here"
  token_ttl: 14400  # 4 hours (default)
```

**Horizon Configuration** (`config/local_settings.py`):
```python
SESSION_TIMEOUT = 14400  # Match O3K token_ttl
```

**Recommended Settings**:
- Development: 24 hours (86400 seconds)
- Production: 4 hours (14400 seconds)
- Minimum: 1 hour (3600 seconds)

### 5.3 noVNC Proxy Configuration

**Current Setup**: noVNC proxy in Docker container

**Production Considerations**:
- Deploy noVNC proxy on dedicated host
- Configure SSL/TLS for encrypted console access
- Use SSH tunnel for VNC connections

**Advanced Configuration** (optional):
```yaml
# docker-compose.yml snippet
novnc:
  image: novnc/noVNC:latest
  environment:
    - TOKEN_VALIDATION_URL=http://o3k:35357/v3/auth/tokens
    - VNC_TIMEOUT=30
  volumes:
    - ./novnc-certs:/etc/novnc/certs:ro  # SSL certificates
```

---

## Step 6: Troubleshooting

### 6.1 "Unable to retrieve version information"

**Symptoms**: Horizon cannot connect to O3K services

**Diagnosis**:
```bash
# Check O3K services running
docker ps | grep o3k

# Test Keystone endpoint
curl http://localhost:35357/v3

# Check Horizon can reach O3K (from within Horizon container)
docker exec o3k-horizon ping -c 3 o3k
```

**Solutions**:
1. Verify O3K is running: `docker compose -f deployments/docker-compose.yml up -d`
2. Check network connectivity between containers
3. Verify `OPENSTACK_HOST` in `local_settings.py` matches O3K container name

### 6.2 "CORS policy: No 'Access-Control-Allow-Origin'"

**Symptoms**: Browser console shows CORS errors

**Solution**: Enable CORS in O3K (see [Step 5.1](#51-cors-configuration-if-needed))

### 6.3 Console Not Available

**Symptoms**: VNC console shows "Connection failed" or blank screen

**Diagnosis**:
```bash
# Check noVNC proxy running
docker ps | grep novnc

# Test console URL generation
TOKEN=$(openstack token issue -f value -c id)
curl -X POST http://localhost:8774/v2.1/servers/{SERVER_ID}/remote-consoles \
  -H "X-Auth-Token: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"remote_console":{"protocol":"vnc","type":"novnc"}}' | jq
```

**Solutions**:
1. Verify noVNC proxy running: `docker ps | grep novnc`
2. Check noVNC logs: `docker logs o3k-novnc`
3. Ensure instance is in ACTIVE state
4. Verify port 6080 accessible: `curl http://localhost:6080`

### 6.4 "Token has expired"

**Symptoms**: Frequent re-authentication required

**Solution**: Increase token TTL (see [Step 5.2](#52-token-ttl-tuning))

### 6.5 Dashboard Loads Slowly

**Symptoms**: Horizon takes >5 seconds to load pages

**Diagnosis**:
```bash
# Check O3K resource usage
docker stats o3k

# Check database connections
docker exec o3k-postgres psql -U o3k -d o3k -c "SELECT count(*) FROM pg_stat_activity;"
```

**Solutions**:
1. Increase O3K memory allocation (edit docker-compose.yml)
2. Optimize database queries (run `ANALYZE` on tables)
3. Reduce resource count (delete unused instances/networks)

### 6.6 Network Topology Not Loading

**Symptoms**: Network topology tab shows spinner or error

**Diagnosis**:
```bash
# Test Neutron endpoints
openstack network list
openstack router list
openstack port list
```

**Solutions**:
1. Verify Neutron endpoints in service catalog
2. Create at least one network and router for topology visualization
3. Check browser JavaScript console for errors

---

## Step 7: Next Steps

### Production Deployment Checklist

- [ ] Change default passwords (`admin` user, Horizon `SECRET_KEY`)
- [ ] Configure SSL/TLS for HTTPS access
- [ ] Enable CORS only for trusted origins
- [ ] Set `SESSION_COOKIE_SECURE=True` and `CSRF_COOKIE_SECURE=True`
- [ ] Configure noVNC proxy with SSL certificates
- [ ] Set up backups for PostgreSQL database
- [ ] Configure firewall rules (restrict port access)
- [ ] Set production-grade `JWT_SECRET` in O3K config
- [ ] Enable logging and monitoring
- [ ] Review and adjust token TTL based on security requirements

### Additional Resources

- **O3K Documentation**: `/Users/I761222/git/o3k/docs/`
- **Horizon Documentation**: https://docs.openstack.org/horizon/latest/
- **OpenStack CLI Guide**: https://docs.openstack.org/python-openstackclient/latest/
- **O3K GitHub Repository**: https://github.com/yourusername/o3k

### Learning More

**Recommended Next Steps**:
1. Create networks with routers and floating IPs
2. Launch instances with multiple attached volumes
3. Configure security groups with custom rules
4. Create instance snapshots and backups
5. Explore quota management
6. Test multi-project isolation

---

## Success Criteria

After completing this quickstart, you should be able to:

- ✅ Access Horizon dashboard at http://localhost/dashboard
- ✅ Login with admin credentials
- ✅ View Overview panel with hypervisor statistics
- ✅ Launch instances through the wizard
- ✅ Access VNC console for running instances
- ✅ Create networks, subnets, and routers
- ✅ Create and attach volumes to instances
- ✅ Upload and manage images

**Total Time**: 15-30 minutes (depending on Docker download speeds)

---

## Cleanup (Optional)

To remove all components:

```bash
# Stop and remove Horizon
cd ~/o3k-horizon
docker compose down -v

# Stop and remove O3K
cd ~/projects/o3k
docker compose -f deployments/docker-compose.yml down -v

# Remove configuration directory (optional)
rm -rf ~/o3k-horizon
```

**Warning**: This deletes all data including instances, networks, volumes, and images.

---

## Support

**Issues**: https://github.com/yourusername/o3k/issues
**Community**: (link to community forum/Slack/Discord)
**Email**: support@o3k-project.org

---

**Last Updated**: 2026-03-13
**O3K Version**: 1.0.0+
**Horizon Version**: Zed (tested), Yoga (compatible)
