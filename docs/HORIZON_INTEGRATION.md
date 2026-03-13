# OpenStack Horizon Integration Guide

**Last Updated**: 2026-03-13
**O3K Version**: 1.0.0+
**Horizon Version**: Zed (tested), Yoga/2023.1 (compatible)

## Overview

This guide explains how to deploy and configure OpenStack Horizon dashboard to work with O3K as the backend cloud platform. O3K provides 100% API compatibility with OpenStack, enabling Horizon to function without any modifications.

## Table of Contents

1. [Architecture](#architecture)
2. [Prerequisites](#prerequisites)
3. [Quick Start](#quick-start)
4. [Configuration](#configuration)
5. [Authentication Flow](#authentication-flow)
6. [Features](#features)
7. [Troubleshooting](#troubleshooting)
8. [Production Deployment](#production-deployment)

---

## Architecture

### Component Diagram

```
┌─────────────┐
│   Browser   │
│  (User)     │
└──────┬──────┘
       │ HTTPS (port 443/80)
       ▼
┌─────────────────────────────────────────┐
│      Horizon Dashboard (Django)         │
│  - Authentication UI                    │
│  - Instance Management UI               │
│  - Network Topology Visualization       │
│  - Volume/Image Management UI           │
└──────┬──────────────────────────────────┘
       │ REST API calls
       │ (authenticated with JWT tokens)
       ▼
┌─────────────────────────────────────────┐
│            O3K Services                  │
│  ┌───────────┬──────────┬──────────┐    │
│  │ Keystone  │  Nova    │ Neutron  │    │
│  │ (35357)   │  (8774)  │ (9696)   │    │
│  ├───────────┼──────────┼──────────┤    │
│  │  Cinder   │  Glance  │Metadata  │    │
│  │  (8776)   │  (9292)  │ (8775)   │    │
│  └───────────┴──────────┴──────────┘    │
│                   │                      │
│                   ▼                      │
│         PostgreSQL Database              │
└─────────────────────────────────────────┘

Optional: noVNC Proxy (port 6080)
└─────▶ VNC console access for instances
```

### Key Integration Points

1. **Keystone Authentication**
   - Horizon sends username/password to Keystone
   - Keystone validates credentials and returns JWT token + service catalog
   - Token used for all subsequent API calls

2. **Service Catalog**
   - Keystone token response includes URLs for all services
   - Horizon uses catalog to discover Nova, Neutron, Cinder, Glance endpoints
   - Dynamic endpoint configuration (no hardcoded service URLs in Horizon)

3. **API Compatibility**
   - O3K implements OpenStack APIs with 100% compatibility
   - Horizon makes standard OpenStack REST API calls
   - No custom Horizon plugins or modifications required

---

## Prerequisites

### Software Requirements

- **Docker** 20.10+ (for containerized deployment)
- **O3K** 1.0.0+ running and accessible
- **PostgreSQL** 16+ (O3K database)

### Network Requirements

- Horizon must be able to reach O3K ports:
  - 35357 (Keystone - Identity)
  - 8774 (Nova - Compute)
  - 9696 (Neutron - Network)
  - 8776 (Cinder - Block Storage)
  - 9292 (Glance - Image)
  - 6080 (noVNC - Console access, optional)

### OS Requirements

- **Horizon**: Any platform supporting Docker (Linux, macOS, Windows)
- **O3K**: Linux (for real mode), any platform (for stub mode)

---

## Quick Start

### 1. Start O3K Backend

```bash
# Clone O3K repository
git clone https://github.com/yourusername/o3k.git
cd o3k

# Start O3K services
docker compose -f deployments/docker-compose.yml up -d

# Verify O3K is running
curl http://localhost:35357/v3
```

### 2. Deploy Horizon

See [Quickstart Guide](../specs/002-horizon-full-compatibility/quickstart.md) for complete deployment instructions.

**Summary**:
```bash
# Create Horizon configuration
mkdir -p ~/o3k-horizon/config
# Copy local_settings.py configuration (see Configuration section)

# Start Horizon
cd ~/o3k-horizon
docker compose up -d

# Access dashboard
open http://localhost/dashboard
```

### 3. Login

- **URL**: http://localhost/dashboard
- **Domain**: Default
- **Username**: admin
- **Password**: secret

**⚠️ Change default credentials in production!**

---

## Configuration

### Horizon Configuration (local_settings.py)

```python
# OpenStack API Configuration
OPENSTACK_HOST = "o3k"  # Docker service name or hostname
OPENSTACK_KEYSTONE_URL = "http://%s:35357/v3" % OPENSTACK_HOST
OPENSTACK_KEYSTONE_DEFAULT_ROLE = "_member_"

# API Versions (must match O3K support)
OPENSTACK_API_VERSIONS = {
    "identity": 3,      # Keystone v3
    "image": 2,         # Glance v2
    "volume": 3,        # Cinder v3
    "compute": 2.1,     # Nova v2.1
}

# Keystone v3 Configuration
OPENSTACK_KEYSTONE_MULTIDOMAIN_SUPPORT = True
OPENSTACK_KEYSTONE_DEFAULT_DOMAIN = "Default"

# Session Timeout (match O3K token TTL)
SESSION_TIMEOUT = 14400  # 4 hours (default O3K token TTL)

# Console Access
CONSOLE_TYPE = 'novnc'

# Regional Settings
OPENSTACK_KEYSTONE_REGION_NAME = "RegionOne"
TIME_ZONE = "UTC"

# Security Settings
SECRET_KEY = 'CHANGE-THIS-IN-PRODUCTION'
SESSION_COOKIE_SECURE = False  # Set True with HTTPS
CSRF_COOKIE_SECURE = False     # Set True with HTTPS

# Neutron Features
OPENSTACK_NEUTRON_NETWORK = {
    'enable_router': True,
    'enable_quotas': True,
    'enable_ipv6': False,
    'enable_distributed_router': False,
    'enable_ha_router': False,
    'enable_fip_topology_check': True,
}
```

### O3K Configuration (config/o3k.yaml)

```yaml
# JWT Token Configuration
keystone:
  jwt_secret: "production-secret-key-change-me"
  token_ttl: 14400  # 4 hours (must match Horizon SESSION_TIMEOUT)

# CORS Configuration (if Horizon on different host/port)
cors:
  enabled: true
  allowed_origins:
    - "http://localhost"
    - "http://horizon"
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

# Service Endpoints (auto-configured in service catalog)
services:
  keystone:
    public_url: "http://o3k-host:35357/v3"
  nova:
    public_url: "http://o3k-host:8774/v2.1"
  neutron:
    public_url: "http://o3k-host:9696/v2.0"
  cinder:
    public_url: "http://o3k-host:8776/v3"
  glance:
    public_url: "http://o3k-host:9292/v2"
```

---

## Authentication Flow

### Token-Based Authentication

```
1. User enters credentials in Horizon login page
   ├─ Username: admin
   ├─ Password: secret
   └─ Domain: Default (or select from dropdown)

2. Horizon sends POST request to Keystone
   POST http://o3k:35357/v3/auth/tokens
   {
     "auth": {
       "identity": {
         "methods": ["password"],
         "password": {
           "user": {
             "name": "admin",
             "password": "secret",
             "domain": {"name": "Default"}
           }
         }
       },
       "scope": {
         "project": {
           "name": "default",
           "domain": {"name": "Default"}
         }
       }
     }
   }

3. Keystone validates credentials
   ├─ Checks username/password against database
   ├─ Verifies user belongs to domain
   └─ Validates project access

4. Keystone returns JWT token + service catalog
   Response Header: X-Subject-Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
   Response Body:
   {
     "token": {
       "expires_at": "2026-03-14T10:00:00.000000Z",
       "issued_at": "2026-03-13T06:00:00.000000Z",
       "user": {...},
       "project": {...},
       "catalog": [
         {
           "type": "compute",
           "name": "nova",
           "endpoints": [{
             "interface": "public",
             "url": "http://o3k-host:8774/v2.1"
           }]
         },
         ...all services...
       ]
     }
   }

5. Horizon stores token and uses for all API calls
   Example: List instances
   GET http://o3k:8774/v2.1/servers
   Header: X-Auth-Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

6. O3K validates token on each request
   ├─ JWT signature verification
   ├─ Expiration check
   ├─ Extract user_id, project_id, roles
   └─ Authorize operation based on roles
```

### Token Lifetime

- **Default TTL**: 4 hours (14400 seconds)
- **Configurable**: Set `keystone.token_ttl` in O3K config
- **Session timeout**: Match Horizon `SESSION_TIMEOUT` to O3K TTL
- **Renewal**: Automatic - Horizon re-authenticates on expiration

### Multi-Tenancy

- Each token is scoped to a specific **project**
- Users can switch projects in Horizon dropdown
- Switching projects re-authenticates with new scope
- Resources are isolated by project_id

---

## Features

### Supported Horizon Features

#### ✅ Compute (Nova)

- **Instance Lifecycle**: Launch, start, stop, pause, suspend, shelve, delete
- **Instance Actions**: Reboot, resize, rebuild, migrate, evacuate
- **Advanced Operations**: Create backup, change password, reset state (admin)
- **Security Groups**: Add/remove security groups from instances
- **Console Access**: VNC, SPICE, serial console via noVNC proxy
- **Flavor Management**: Create, list, delete flavors
- **Keypair Management**: Create, import, delete SSH keypairs
- **Hypervisor Statistics**: View compute resource usage in Overview panel

#### ✅ Network (Neutron)

- **Networks**: Create, list, update, delete networks
- **Subnets**: Create subnets with CIDR, gateway, DHCP configuration
- **Routers**: Create routers, attach to networks, set external gateway
- **Floating IPs**: Allocate, associate, disassociate floating IPs
- **Security Groups**: Create groups, add/remove rules (TCP/UDP/ICMP)
- **Network Topology**: Graphical visualization of network architecture
- **Port Forwarding**: Forward external ports to instance internal ports

#### ✅ Block Storage (Cinder)

- **Volumes**: Create, list, update, delete, extend volumes
- **Volume Attachments**: Attach/detach volumes to/from instances
- **Snapshots**: Create snapshots, restore volumes from snapshots
- **Volume Types**: Manage volume types and QoS specs
- **Backups**: Create volume backups, restore from backups

#### ✅ Images (Glance)

- **Image Management**: Upload, download, delete images
- **Image Metadata**: Update image properties and tags
- **Image Sharing**: Share images between projects (member management)
- **Image Import**: Import images from URL or staged data

#### ✅ Identity (Keystone)

- **User Management**: Create, update, delete users
- **Project Management**: Create, update, delete projects
- **Role Assignments**: Assign users to projects with roles
- **Domain Support**: Multi-domain deployments

### Console Access (noVNC)

**VNC Console Flow**:

1. User clicks "Console" tab on instance details page
2. Horizon calls Nova API: `POST /servers/{id}/remote-consoles`
3. Nova generates console URL with JWT token:
   ```json
   {
     "remote_console": {
       "type": "novnc",
       "protocol": "vnc",
       "url": "http://o3k-host:6080/vnc_auto.html?token=eyJ..."
     }
   }
   ```
4. Horizon opens popup window with console URL
5. noVNC proxy validates token and streams VNC data to browser

**Requirements**:
- noVNC proxy deployed (separate container)
- Port 6080 accessible from user's browser
- Instance must be in ACTIVE state
- Real mode: libvirt must be configured

---

## Troubleshooting

### Common Issues

#### 1. "Unable to retrieve version information"

**Symptoms**: Horizon cannot connect to O3K services

**Causes**:
- O3K services not running
- Network connectivity issue
- Incorrect `OPENSTACK_HOST` in Horizon config

**Solutions**:
```bash
# Verify O3K is running
docker ps | grep o3k

# Test Keystone endpoint
curl http://localhost:35357/v3

# Check Horizon can reach O3K (from Horizon container)
docker exec o3k-horizon ping -c 3 o3k

# Verify OPENSTACK_HOST matches Docker network name
```

#### 2. "CORS policy: No 'Access-Control-Allow-Origin'"

**Symptoms**: Browser console shows CORS errors

**Cause**: CORS not configured in O3K

**Solution**: Enable CORS in O3K config (`config/o3k.yaml`):
```yaml
cors:
  enabled: true
  allowed_origins:
    - "http://localhost"
    - "http://horizon-hostname"
```

Restart O3K after configuration change.

#### 3. "Token has expired"

**Symptoms**: Frequent re-authentication required

**Cause**: Token TTL too short or session timeout mismatch

**Solution**: Increase token TTL and match session timeout:

**O3K** (`config/o3k.yaml`):
```yaml
keystone:
  token_ttl: 14400  # 4 hours
```

**Horizon** (`local_settings.py`):
```python
SESSION_TIMEOUT = 14400  # Match O3K token_ttl
```

#### 4. "Console not available"

**Symptoms**: VNC console shows "Connection failed"

**Causes**:
- noVNC proxy not running
- Instance not in ACTIVE state
- Port 6080 not accessible

**Solutions**:
```bash
# Check noVNC proxy running
docker ps | grep novnc

# Verify port 6080 accessible
curl http://localhost:6080

# Check instance status
openstack server show <instance-id> -c status

# View noVNC logs
docker logs o3k-novnc
```

#### 5. "Network topology not loading"

**Symptoms**: Topology tab shows spinner or error

**Cause**: Missing Neutron endpoints or no resources to display

**Solution**:
```bash
# Verify Neutron endpoints in service catalog
openstack catalog show neutron

# Create test resources
openstack network create test-network
openstack router create test-router

# Refresh Horizon page
```

#### 6. "Slow dashboard performance"

**Symptoms**: Pages take >5 seconds to load

**Causes**:
- Large number of resources (100+ instances/networks)
- Database query performance
- Network latency

**Solutions**:
```bash
# Check O3K resource usage
docker stats o3k

# Optimize database (run ANALYZE)
docker exec o3k-postgres psql -U o3k -d o3k -c "ANALYZE;"

# Check database connection pool
# Increase if needed in config/o3k.yaml:
database:
  max_connections: 50  # default: 20
```

---

## Production Deployment

### Security Hardening

#### 1. Change Default Credentials

```bash
# Change admin password
openstack user password set admin

# Update Horizon JWT secret
# Edit config/o3k.yaml:
keystone:
  jwt_secret: "<generate-secure-random-key>"

# Restart O3K
docker compose -f deployments/docker-compose.yml restart o3k
```

#### 2. Enable HTTPS/SSL

**Horizon** (using nginx reverse proxy):
```nginx
server {
    listen 443 ssl http2;
    server_name horizon.example.com;

    ssl_certificate /etc/ssl/certs/horizon.crt;
    ssl_certificate_key /etc/ssl/private/horizon.key;

    location / {
        proxy_pass http://localhost:80;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

**Update Horizon config**:
```python
SESSION_COOKIE_SECURE = True
CSRF_COOKIE_SECURE = True
```

#### 3. Configure Firewall

```bash
# Allow only necessary ports
sudo ufw allow 443/tcp   # Horizon HTTPS
sudo ufw allow 35357/tcp # Keystone (if external access needed)
sudo ufw deny 8774/tcp   # Nova (internal only)
sudo ufw deny 9696/tcp   # Neutron (internal only)
sudo ufw deny 8776/tcp   # Cinder (internal only)
sudo ufw deny 9292/tcp   # Glance (internal only)
```

#### 4. Enable Audit Logging

**O3K logging** (`config/o3k.yaml`):
```yaml
logging:
  level: info
  format: json
  output: /var/log/o3k/o3k.log
  audit: true  # Log all API requests
```

### High Availability

**Horizon HA** (stateless):
- Deploy multiple Horizon containers behind load balancer
- No shared state between containers
- Session data in cookies (encrypted)

**O3K HA** (advanced):
- Deploy multiple O3K instances
- Use HAProxy/nginx load balancer
- Shared PostgreSQL (primary-replica setup)
- Requires external coordination for VXLAN networking

### Backup & Recovery

**PostgreSQL Backup**:
```bash
# Automated daily backups
docker exec o3k-postgres pg_dump -U o3k -d o3k > backup-$(date +%Y%m%d).sql

# Restore from backup
docker exec -i o3k-postgres psql -U o3k -d o3k < backup-20260313.sql
```

**Configuration Backup**:
```bash
# Backup O3K config
cp config/o3k.yaml config/o3k.yaml.backup

# Backup Horizon config
cp ~/o3k-horizon/config/local_settings.py ~/o3k-horizon/config/local_settings.py.backup
```

### Monitoring

**Key Metrics to Monitor**:
- O3K API response times
- PostgreSQL connection count
- Token generation rate
- Instance creation success rate
- Horizon page load times

**Tools**:
- Prometheus + Grafana (metrics)
- ELK Stack (log aggregation)
- OpenStack Health Monitor (service checks)

---

## API Coverage

O3K implements **91% of OpenStack APIs** (308/330 endpoints):

| Service | Coverage | Notes |
|---------|----------|-------|
| Nova (Compute) | 85% | All Horizon-required endpoints supported |
| Neutron (Network) | 95% | Full network topology support |
| Cinder (Block Storage) | 100% | All volume operations complete |
| Glance (Image) | 100% | Full image lifecycle + import |
| Keystone (Identity) | 90% | Federation/SAML not supported (enterprise-only) |

See [API_COVERAGE.md](./API_COVERAGE.md) for detailed endpoint list.

---

## Additional Resources

- **O3K Documentation**: [/docs](/docs)
- **Quickstart Guide**: [quickstart.md](../specs/002-horizon-full-compatibility/quickstart.md)
- **Keystone Auth Flow**: [KEYSTONE_AUTH_FLOW.md](./KEYSTONE_AUTH_FLOW.md)
- **OpenStack Horizon Docs**: https://docs.openstack.org/horizon/latest/
- **O3K GitHub**: https://github.com/yourusername/o3k
- **Issue Tracker**: https://github.com/yourusername/o3k/issues

---

## Contributing

Found an issue or want to improve Horizon integration?

1. Check [existing issues](https://github.com/yourusername/o3k/issues)
2. Open a new issue with:
   - O3K version
   - Horizon version
   - Steps to reproduce
   - Expected vs actual behavior
3. Submit pull requests with tests

---

**Version**: 1.0.0
**Last Updated**: 2026-03-13
**Authors**: O3K Development Team
**License**: Apache 2.0
