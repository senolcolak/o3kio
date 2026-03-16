# Horizon Setup Guide for O3K

**IMPORTANT**: Horizon dashboard integration is **extended functionality**, not a core O3K feature. Horizon requires independent setup, configuration, and (optionally) rebranding to work with O3K.

---

## Overview

O3K provides 100% API compatibility with OpenStack Horizon dashboard, but Horizon itself is:
- **Optional**: O3K works perfectly without Horizon (CLI/API/SDK are primary interfaces)
- **Independent**: Horizon runs as a separate service (Django application)
- **Configurable**: Requires specific configuration to connect to O3K
- **Rebrandable**: Can be customized with O3K branding (optional)

---

## Version Requirements

| Component | Version | Requirement |
|-----------|---------|-------------|
| O3K | v0.5.0+ | Required |
| OpenStack | Flamingo 2025.2+ | Required |
| Horizon | Flamingo 2025.2+ | Required |
| Earlier Versions | Zed, Yoga, etc. | **NOT SUPPORTED** |

**Why Flamingo only?**
- O3K is designed for modern OpenStack (2025.2+)
- Earlier versions use deprecated APIs and patterns
- Maintaining backward compatibility would compromise O3K's lightweight design

---

## Deployment Options

### Option 1: Unified Deployment (Recommended)

Deploy O3K and Horizon together using `docker-compose-horizon.yml`:

```bash
cd o3k/deployments
docker compose -f docker-compose-horizon.yml up -d
```

**What's included**:
- PostgreSQL 18 database
- O3K services (Keystone, Nova, Neutron, Cinder, Glance)
- Horizon Flamingo 2025.2
- noVNC console proxy
- Memcached (for Django sessions)

**Access**: http://localhost/dashboard

**Documentation**: [UNIFIED_DEPLOYMENT.md](UNIFIED_DEPLOYMENT.md)

---

### Option 2: Separate Horizon Deployment

Deploy Horizon separately to existing O3K installation:

```bash
cd o3k/deployments
docker compose -f docker-compose-horizon.yml up -d horizon
```

**Prerequisites**:
- O3K services already running
- O3K accessible at known hostname/IP

**Documentation**: [HORIZON_DEPLOYMENT.md](HORIZON_DEPLOYMENT.md)

---

## Configuration

### Step 1: Horizon local_settings.py

Horizon must be configured to connect to O3K services:

```python
# deployments/horizon-config/local_settings
OPENSTACK_HOST = "o3k"  # Docker service name or IP
OPENSTACK_KEYSTONE_URL = "http://%s:35357/v3" % OPENSTACK_HOST

# API Versions (Flamingo compatible)
OPENSTACK_API_VERSIONS = {
    "identity": 3,      # Keystone v3
    "image": 2,         # Glance v2
    "volume": 3,        # Cinder v3
    "compute": 2,       # Nova v2.1 (Horizon expects integer 2)
}

# Keystone Configuration
OPENSTACK_KEYSTONE_MULTIDOMAIN_SUPPORT = True
OPENSTACK_KEYSTONE_DEFAULT_DOMAIN = "Default"

# Session Configuration (matches O3K token TTL)
SESSION_TIMEOUT = 14400  # 4 hours
SESSION_ENGINE = 'django.contrib.sessions.backends.cache'

# Console Access
CONSOLE_TYPE = 'novnc'
NOVNC_PROXY_URL = 'http://localhost:6080'
```

**Key Configuration Points**:
- `OPENSTACK_HOST`: Must resolve to O3K service
- `OPENSTACK_API_VERSIONS`: Match O3K implemented versions
- `SESSION_TIMEOUT`: Should match O3K JWT token TTL (default 24h, recommended 4h for Horizon)
- `CONSOLE_TYPE`: Must match deployed console proxy (noVNC, SPICE, etc.)

---

### Step 2: Mount Configuration

Mount your configuration into the Horizon container:

```yaml
# docker-compose-horizon.yml
services:
  horizon:
    image: quay.io/openstack.kolla/horizon:2025.2-ubuntu-noble
    volumes:
      - ./horizon-config/local_settings:/etc/openstack-dashboard/local_settings.py:ro
```

---

### Step 3: Network Configuration

Horizon must communicate with O3K services:

```yaml
# docker-compose-horizon.yml
services:
  horizon:
    environment:
      - OPENSTACK_HOST=o3k
      - OPENSTACK_KEYSTONE_URL=http://o3k:35357/v3
    networks:
      - o3k-network  # Must be on same network as O3K
```

---

## Optional: Horizon Rebranding

Horizon can be customized with O3K branding (logo, colors, name):

### Rebranding Script (Future)

A standalone rebranding script will be provided to:
- Replace OpenStack logos with O3K logos
- Update site title and branding text
- Customize color scheme
- Add custom CSS/JavaScript

**Status**: Planned for future release

**Current Workaround**: Use `local_settings.py` configuration:

```python
# Site branding
SITE_BRANDING = "O3K Cloud Dashboard"
SITE_BRANDING_LINK = "/"

# Custom product name
CUSTOM_DASHBOARD_NAME = "O3K Horizon Dashboard"
CUSTOM_PRODUCT_NAME = "O3K OpenStack Platform"
CUSTOM_WELCOME_MESSAGE = "Welcome to O3K - Lightweight OpenStack"
```

---

## Default Credentials

```text
Domain: Default
Username: admin
Password: secret
```

**⚠️ Change these credentials in production!**

---

## Verification

### Step 1: Access Horizon

Open http://localhost/dashboard (or your configured URL)

### Step 2: Login

Use default credentials or your configured credentials.

### Step 3: Test Functionality

**Project → Compute → Instances:**
- Launch instance
- Start/stop instance
- Access VNC console
- Delete instance

**Project → Network → Network Topology:**
- View network visualization
- Create network
- Create router

**Project → Volumes:**
- Create volume
- Attach volume to instance
- Create snapshot

---

## Troubleshooting

### Issue: "Connection to Keystone failed"

**Symptoms**: Login page loads but authentication fails

**Causes**:
- O3K services not running
- Incorrect `OPENSTACK_KEYSTONE_URL`
- Network connectivity issue

**Fix**:
```bash
# Verify O3K is running
docker ps | grep o3k

# Test Keystone endpoint
curl http://localhost:35357/v3

# Check Horizon logs
docker logs o3k-horizon
```

---

### Issue: "API version mismatch"

**Symptoms**: Pages load but data doesn't display

**Causes**:
- Incorrect `OPENSTACK_API_VERSIONS` in `local_settings.py`
- Using non-Flamingo Horizon image

**Fix**:
```python
# Verify API versions match O3K
OPENSTACK_API_VERSIONS = {
    "identity": 3,
    "image": 2,
    "volume": 3,
    "compute": 2,  # NOT 2.1 - Horizon expects integer
}
```

---

### Issue: "Session expired immediately after login"

**Symptoms**: Logged out immediately after authentication

**Causes**:
- `SESSION_TIMEOUT` too short
- Memcached not running
- Time sync issue between containers

**Fix**:
```python
# Increase session timeout
SESSION_TIMEOUT = 14400  # 4 hours

# Verify Memcached
docker ps | grep memcached

# Check container time sync
docker exec o3k-horizon date
docker exec o3k date
```

---

### Issue: "noVNC console not working"

**Symptoms**: VNC console button doesn't work or shows blank screen

**Causes**:
- noVNC proxy not running
- Incorrect `NOVNC_PROXY_URL`
- Nova not configured for console access

**Fix**:
```bash
# Verify noVNC proxy is running
docker ps | grep novnc

# Test noVNC endpoint
curl http://localhost:6080

# Check Nova console configuration
grep console_proxy config/o3k.yaml
```

---

## Known Limitations

### Identity Projects Page (404)

**Status**: Known Horizon/Kolla configuration issue (NOT an O3K bug)

**Symptoms**: `/dashboard/identity/projects/` returns HTTP 404

**Root Cause**: Django panel registration issue in Kolla Horizon image

**Workaround**: Use OpenStack CLI or API directly:
```bash
# List projects
openstack project list

# Create project
openstack project create --domain Default my-project

# Delete project
openstack project delete my-project
```

**Resolution**: This is a Horizon packaging issue, not an O3K API issue. The Keystone `/v3/projects` API endpoint works perfectly.

**Documentation**: [HORIZON_PROJECTS_ISSUE.md](HORIZON_PROJECTS_ISSUE.md)

---

## Performance Considerations

### Database Connection Pooling

Horizon makes many API calls per page load. Ensure O3K has sufficient database connections:

```yaml
# config/o3k.yaml
database:
  max_open_conns: 50  # Increase for high Horizon traffic
  max_idle_conns: 10
```

---

### Session Caching

Use Memcached for Django sessions (NOT database backend):

```python
# local_settings.py
SESSION_ENGINE = 'django.contrib.sessions.backends.cache'

CACHES = {
    'default': {
        'BACKEND': 'django.core.cache.backends.memcached.PyMemcacheCache',
        'LOCATION': 'memcached:11211',
    }
}
```

---

## Security Recommendations

### Production Deployment

1. **Change default credentials**:
```bash
openstack user set --password NEW_PASSWORD admin
```

2. **Enable HTTPS**:
```yaml
# docker-compose-horizon.yml
services:
  horizon:
    environment:
      - SESSION_COOKIE_SECURE=True
      - CSRF_COOKIE_SECURE=True
```

3. **Restrict ALLOWED_HOSTS**:
```python
# local_settings.py
ALLOWED_HOSTS = ['your-domain.com']  # NOT ['*']
```

4. **Change JWT secret**:
```yaml
# config/o3k.yaml
keystone:
  jwt_secret: "USE-STRONG-RANDOM-SECRET"
```

---

## Architecture

### Communication Flow

```
┌─────────────┐      HTTP      ┌──────────────┐      HTTP      ┌─────────────┐
│   Browser   │ ───────────────>│   Horizon    │ ───────────────>│     O3K     │
│             │                 │  (Django)    │                 │  Services   │
└─────────────┘                 └──────────────┘                 └─────────────┘
                                      │                                 │
                                      │ Session                         │ JWT
                                      ↓                                 ↓
                                ┌──────────────┐                 ┌─────────────┐
                                │  Memcached   │                 │ PostgreSQL  │
                                └──────────────┘                 └─────────────┘
```

**Key Points**:
- Horizon is stateless (sessions in Memcached)
- O3K uses JWT tokens (no session database)
- All API calls go through O3K (Horizon → O3K → PostgreSQL)
- noVNC proxy provides console access (separate WebSocket connection)

---

## Summary

**Horizon is extended functionality**:
- O3K works perfectly without Horizon (CLI/API/SDK are primary)
- Horizon requires independent setup and configuration
- Version requirement: Flamingo 2025.2+ only
- Rebranding script (future): Optional customization for O3K branding

**Deployment Guides**:
- [UNIFIED_DEPLOYMENT.md](UNIFIED_DEPLOYMENT.md) - Complete setup
- [HORIZON_DEPLOYMENT.md](HORIZON_DEPLOYMENT.md) - Separate Horizon
- [HORIZON_INTEGRATION.md](HORIZON_INTEGRATION.md) - Architecture details
- [QUICK_REFERENCE.md](QUICK_REFERENCE.md) - Command cheatsheet

**Support**:
- API compatibility: 100% (all Horizon workflows functional)
- Known issues: Identity projects page 404 (Horizon bug, not O3K)
- Primary interfaces: OpenStack CLI, API, SDK
- Web UI: Optional Horizon dashboard
