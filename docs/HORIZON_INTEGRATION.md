# OpenStack Horizon Integration with O3K

## Overview

This document describes how O3K integrates with OpenStack Horizon (Flamingo 2025.2) to provide a fully functional web dashboard for managing cloud resources. The integration provides 100% API compatibility with OpenStack services, allowing Horizon to work seamlessly with O3K's implementation of Keystone, Nova, Neutron, Cinder, and Glance.

## Architecture

### Components

The unified deployment includes five main services:

1. **PostgreSQL 18.3** - Database for O3K
2. **O3K** - OpenStack-compatible cloud platform (Keystone, Nova, Neutron, Cinder, Glance, Metadata)
3. **Horizon** - OpenStack Flamingo (2025.2) web dashboard
4. **Memcached 1.6** - Distributed session storage for Horizon (multi-process support)
5. **noVNC** - VNC console proxy for instance access

### Service Communication Flow

```
User Browser → Horizon (Port 80) → O3K API Services (Ports 35357, 8774, 9696, 8776, 9292)
                    ↓                        ↓
                Memcached            PostgreSQL (Port 5432)
              (Port 11211)
```

### Keystone Integration

Horizon authenticates users through O3K's Keystone service using a 3-step authentication flow:

#### Step 1: Unscoped Password Authentication

User enters credentials (default: admin/secret) in Horizon login form. Horizon sends:

```http
POST /v3/auth/tokens HTTP/1.1
Content-Type: application/json

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
    "scope": "unscoped"
  }
}
```

**Response**: HTTP 201 with unscoped JWT token in `X-Subject-Token` header. The token contains user_id and domain_id but no project_id.

#### Step 2: List Available Projects

Horizon uses the unscoped token to fetch available projects:

```http
GET /v3/auth/projects HTTP/1.1
X-Auth-Token: <unscoped-token>
```

**Response**: HTTP 200 with list of projects the user has access to

#### Step 3: Token Re-Authentication with Project Scope

Horizon re-authenticates using the unscoped token to get a scoped token:

```http
POST /v3/auth/tokens HTTP/1.1
Content-Type: application/json

{
  "auth": {
    "identity": {
      "methods": ["token"],
      "token": {"id": "<unscoped-token>"}
    },
    "scope": {
      "project": {
        "name": "default",
        "domain": {"name": "Default"}
      }
    }
  }
}
```

**Response**: HTTP 201 with scoped JWT token and service catalog
**Response**: HTTP 201 with scoped JWT token and service catalog

The scoped token contains:
- user_id, username
- project_id, project_name
- domain_id
- roles (e.g., ["admin"])
- Service catalog mapping service types to endpoints:

   ```json
   {
     "token": {
       "catalog": [
         {
           "type": "compute",
           "name": "nova",
           "endpoints": [{"interface": "public", "region": "RegionOne", "url": "http://o3k:8774/v2.1"}]
         },
         {
           "type": "network",
           "name": "neutron",
           "endpoints": [{"interface": "public", "region": "RegionOne", "url": "http://o3k:9696/v2.0"}]
         },
         {
           "type": "volumev3",
           "name": "cinderv3",
           "endpoints": [{"interface": "public", "region": "RegionOne", "url": "http://o3k:8776/v3"}]
         },
         {
           "type": "image",
           "name": "glance",
           "endpoints": [{"interface": "public", "region": "RegionOne", "url": "http://o3k:9292/v2"}]
         },
         {
           "type": "identity",
           "name": "keystone",
           "endpoints": [{"interface": "public", "region": "RegionOne", "url": "http://o3k:35357/v3"}]
         }
       ]
     }
   }
   ```

#### Step 4: Dashboard API Calls

Horizon uses the scoped token and service catalog to make authenticated API calls to all OpenStack services (Nova, Neutron, Cinder, Glance). All requests include the `X-Auth-Token` header with the scoped token.

**Critical Implementation Detail**: Service catalog endpoints use `http://o3k` (Docker service name) instead of `http://localhost` to enable container-to-container communication within the Docker network.

### API Compatibility

O3K implements 100% OpenStack API compatibility for the endpoints Horizon requires:

#### Keystone (Identity - Port 35357)
- `POST /v3/auth/tokens` - Token generation (password and token methods)
- `GET /v3/auth/projects` - List projects for unscoped token
- `GET /v3/auth/tokens` - Token validation
- `DELETE /v3/auth/tokens` - Token revocation
- `GET /v3/projects` - List projects
- `GET /v3/users` - List users
- `GET /v3/domains` - List domains
- `GET /v3/` - API version discovery

#### Nova (Compute - Port 8774)
- `GET /v2.1/servers` - List instances
- `POST /v2.1/servers` - Create instance
- `GET /v2.1/servers/{id}` - Show instance details
- `DELETE /v2.1/servers/{id}` - Delete instance
- `POST /v2.1/servers/{id}/action` - Instance actions (start, stop, reboot, etc.)
- `GET /v2.1/flavors` - List flavors
- `GET /v2.1/keypairs` - List SSH keypairs
- `POST /v2.1/keypairs` - Create keypair
- `GET /v2.1/os-availability-zone` - List availability zones

#### Neutron (Network - Port 9696)
- `GET /v2.0/networks` - List networks
- `POST /v2.0/networks` - Create network
- `GET /v2.0/subnets` - List subnets
- `POST /v2.0/subnets` - Create subnet
- `GET /v2.0/ports` - List ports
- `GET /v2.0/routers` - List routers
- `POST /v2.0/routers` - Create router
- `GET /v2.0/floatingips` - List floating IPs
- `POST /v2.0/floatingips` - Create floating IP
- `GET /v2.0/security-groups` - List security groups
- `POST /v2.0/security-group-rules` - Create security group rule

#### Cinder (Block Storage - Port 8776)
- `GET /v3/volumes` - List volumes
- `POST /v3/volumes` - Create volume
- `GET /v3/volumes/{id}` - Show volume details
- `DELETE /v3/volumes/{id}` - Delete volume
- `POST /v3/volumes/{id}/action` - Volume actions (attach, detach, etc.)
- `GET /v3/snapshots` - List snapshots
- `POST /v3/snapshots` - Create snapshot

#### Glance (Image - Port 9292)
- `GET /v2/images` - List images
- `POST /v2/images` - Create image
- `GET /v2/images/{id}` - Show image details
- `DELETE /v2/images/{id}` - Delete image
- `PUT /v2/images/{id}/file` - Upload image data
- `GET /v2/images/{id}/file` - Download image data

## Deployment Configuration

### Docker Compose Structure

The deployment is defined in `deployments/docker-compose-horizon.yml` and includes:

**Network**: `o3k-horizon-network` (bridge mode)

**Volumes**:
- `postgres-data` - PostgreSQL database files
- `o3k-data` - O3K state and metadata
- `o3k-images` - Glance image storage
- `o3k-volumes` - Cinder volume storage
- `horizon-static` - Django static files (CSS, JS)
- `horizon-logs` - Horizon and Apache logs

### Horizon Session Management

**Challenge**: Horizon runs as a Django application in Apache with mod_wsgi using multiple worker processes. Session data must be shared across these processes for login to work correctly.

**Problem with Default Backends**:
- `LocMemCache`: Per-process memory, sessions not shared between Apache workers
- `django.contrib.sessions.backends.cache` with `LocMemCache`: Same issue
- File-based sessions: File locking issues with concurrent process access
- Signed cookies: Security concerns, size limits, can't be invalidated server-side

**Solution**: Use Memcached for distributed session storage. Memcached provides:
- Shared memory cache accessible by all Apache worker processes
- Fast session read/write (<1ms latency)
- Automatic session expiration (SESSION_TIMEOUT = 4 hours)
- No file I/O overhead

### Horizon Configuration

Horizon is configured through several files mounted into the container:

#### 1. Kolla Configuration (`horizon-config/config.json`)

Controls how Kolla's startup script (`kolla_set_configs`) initializes the container:

```json
{
    "command": "/usr/sbin/apache2ctl -DFOREGROUND",
    "config_files": [
        {
            "source": "/var/lib/kolla/config_files/local_settings",
            "dest": "/etc/openstack-dashboard/local_settings.py",
            "owner": "horizon",
            "perm": "0644"
        },
        {
            "source": "/var/lib/kolla/config_files/ports.conf",
            "dest": "/etc/apache2/ports.conf",
            "owner": "root",
            "perm": "0644"
        },
        {
            "source": "/var/lib/kolla/config_files/horizon-nolist.conf",
            "dest": "/etc/apache2/sites-enabled/000-default.conf",
            "owner": "root",
            "perm": "0644"
        }
    ],
    "permissions": [...]
}
```

#### 2. Django Settings (`horizon-config/local_settings`)

Configures Horizon's Django application (400+ lines):

**Key Settings**:
- `DEBUG = True` - Enable for troubleshooting (set to False in production)
- `OPENSTACK_HOST = "o3k"` - O3K service hostname (Docker service name)
- `OPENSTACK_KEYSTONE_URL = "http://o3k:35357/v3"` - Keystone endpoint
- `OPENSTACK_API_VERSIONS` - API versions for each service:
  - `identity: 3` - Keystone v3
  - `image: 2` - Glance v2
  - `volume: 3` - Cinder v3
  - `compute: 2.1` - Nova v2.1
- `OPENSTACK_KEYSTONE_MULTIDOMAIN_SUPPORT = False` - Single domain mode
- `OPENSTACK_KEYSTONE_DEFAULT_DOMAIN = "Default"` - Default domain
- `SESSION_ENGINE = 'django.contrib.sessions.backends.cache'` - Cache-based sessions
- `SESSION_TIMEOUT = 14400` - 4 hours (matches O3K JWT token TTL)

**Session and Cache Configuration (CRITICAL)**:
```python
# Session engine uses cache backend
SESSION_ENGINE = 'django.contrib.sessions.backends.cache'

# Memcached provides distributed cache for multi-process Apache
CACHES = {
    'default': {
        'BACKEND': 'django.core.cache.backends.memcached.PyMemcacheCache',
        'LOCATION': 'memcached:11211',
    }
}
```

**Why This Is Required**:
- Apache mod_wsgi runs multiple worker processes (configured in virtual host)
- Each process needs to access the same session data
- Memcached provides a shared store accessible by all processes
- Without Memcached, login works but sessions aren't persisted (redirect loop)

**Neutron Features**:
```python
OPENSTACK_NEUTRON_NETWORK = {
    'enable_router': True,
    'enable_quotas': True,
    'enable_ipv6': True,
    'enable_distributed_router': False,
    'enable_ha_router': False,
    'enable_lb': False,
    'enable_firewall': False,
    'enable_vpn': False,
    'enable_fip_topology_check': True,
    'default_ipv4_subnet_pool_label': None,
    'default_ipv6_subnet_pool_label': None,
    'profile_support': None,
    'supported_provider_types': ['*'],
    'physical_networks': [],
}
```

#### 3. Apache Configuration

**ports.conf** (`horizon-config/apache/ports.conf`):
```apache
Listen 80

<IfModule ssl_module>
    Listen 443
</IfModule>
```

**Virtual Host** (`horizon-config/apache/horizon-nolist.conf`):
```apache
<VirtualHost _default_:80>
    ServerAdmin webmaster@localhost
    DocumentRoot /var/www/html

    ErrorLog ${APACHE_LOG_DIR}/error.log
    CustomLog ${APACHE_LOG_DIR}/access.log combined

    WSGIDaemonProcess horizon user=horizon group=horizon processes=3 threads=10 display-name=%{GROUP}
    WSGIProcessGroup horizon
    WSGIApplicationGroup %{GLOBAL}

    WSGIScriptAlias /dashboard /var/lib/kolla/venv/lib/python3.12/site-packages/openstack_dashboard/wsgi.py

    <Directory /var/lib/kolla/venv/lib/python3.12/site-packages/openstack_dashboard>
        Require all granted
    </Directory>

    Alias /static /var/lib/kolla/venv/lib/python3.12/site-packages/static
    <Directory /var/lib/kolla/venv/lib/python3.12/site-packages/static>
        Require all granted
    </Directory>
</VirtualHost>
```

**Why horizon-nolist.conf?**
- Original `horizon.conf` included `Listen 80` directive
- Apache's `ports.conf` also has `Listen 80`
- Having both causes "Cannot define multiple Listeners" error
- `horizon-nolist.conf` is identical but without the `Listen` directive
- Copied to `/etc/apache2/sites-enabled/000-default.conf` to make Horizon the default site

### Environment Variables

**Horizon Container**:
- `KOLLA_INSTALL_TYPE=source` - Use source install (not binary)
- `KOLLA_CONFIG_STRATEGY=COPY_ALWAYS` - Always copy config files on startup

**O3K Container**:
- `O3K_DB_URL=postgres://o3k:secret@postgres:5432/o3k?sslmode=disable` - Database connection
- `O3K_JWT_SECRET=change-this-secret-in-production-12345` - JWT signing key (CHANGE IN PRODUCTION)

## Deployment Steps

### 1. Prerequisites

- Docker and Docker Compose installed
- x86_64 architecture (Horizon image not available for ARM64)
- At least 4GB RAM available
- Ports 80, 35357, 8774, 9696, 8776, 9292, 6080 available

### 2. Start Services

```bash
cd /path/to/o3k
docker compose -f deployments/docker-compose-horizon.yml up -d
```

### 3. Verify Services

```bash
# Check all containers are running
docker ps

# Expected output:
# CONTAINER ID   IMAGE                                                 STATUS
# xxxxxxxxx      quay.io/openstack.kolla/horizon:2025.2-ubuntu-noble   Up (healthy)
# xxxxxxxxx      o3k:latest                                            Up (healthy)
# xxxxxxxxx      postgres:18.3-alpine                                  Up (healthy)
# xxxxxxxxx      dougw/novnc:latest                                    Up (healthy)
```

### 4. Check Logs

```bash
# O3K logs
docker logs o3k

# Horizon logs
docker logs o3k-horizon

# PostgreSQL logs
docker logs o3k-postgres
```

### 5. Access Dashboard

Open browser to: **http://localhost/dashboard/**

**Default Credentials**:
- Username: `admin`
- Password: `secret`
- Domain: `Default`

## Dashboard Features

### Identity (Keystone)

- **Projects**: View and manage projects/tenants
- **Users**: Create and manage users
- **Groups**: User groups (if configured)
- **Roles**: Role assignments

### Compute (Nova)

- **Instances**: Launch, manage, and delete VMs
- **Flavors**: View available instance types
- **Images**: View available OS images
- **Keypairs**: Create and import SSH keypairs
- **Server Groups**: Anti-affinity/affinity groups (if configured)

### Network (Neutron)

- **Networks**: Create and manage networks
- **Subnets**: Configure IP address ranges
- **Routers**: Create routers and connect networks
- **Floating IPs**: Allocate and associate public IPs
- **Security Groups**: Configure firewall rules
- **Ports**: View and manage network ports

### Volumes (Cinder)

- **Volumes**: Create, attach, and detach block storage
- **Snapshots**: Create volume snapshots
- **Backups**: Volume backups (if configured)

### Images (Glance)

- **Images**: Upload and manage OS images
- **Metadata**: Configure image properties

### System Info

- **Overview**: Resource usage dashboard
- **Hypervisors**: Compute node information (if in real mode)
- **Availability Zones**: Zone information

## Troubleshooting

### Horizon Not Loading

**Symptom**: `curl http://localhost/dashboard/` returns connection refused or timeout

**Diagnosis**:
```bash
# Check container status
docker ps | grep horizon

# Check logs
docker logs o3k-horizon --tail 50

# Common issues:
# - Container restarting: Check for Apache errors in logs
# - "Unable to open logs": Permission issue with log directory
# - "no listening sockets": Missing Listen directive in Apache config
```

**Resolution**:
1. Verify `horizon-config/apache/ports.conf` exists with `Listen 80`
2. Verify `horizon-config/config.json` copies files correctly
3. Recreate container: `docker compose down && docker compose up -d`

### 404 Not Found for /dashboard

**Symptom**: Apache returns 404 for /dashboard endpoint

**Diagnosis**:
```bash
# Check Apache virtual host configuration
docker exec o3k-horizon cat /etc/apache2/sites-enabled/000-default.conf

# Check if WSGIScriptAlias is configured
docker exec o3k-horizon grep WSGIScriptAlias /etc/apache2/sites-enabled/000-default.conf
```

**Resolution**:
1. Verify `WSGIScriptAlias /dashboard` is in virtual host config
2. Ensure 000-default.conf contains Horizon configuration (not default Ubuntu site)
3. Check that `horizon-nolist.conf` is being copied to 000-default.conf in config.json

### Authentication Failures

**Symptom**: Login fails with "Unable to authenticate" or "Unable to retrieve authorized projects"

**Diagnosis**:
```bash
# Check O3K Keystone is accessible from Horizon container
docker exec o3k-horizon curl -v http://o3k:35357/v3

# Check O3K logs for authentication errors
docker logs o3k | grep -i auth

# Test unscoped authentication
curl -X POST http://localhost:35357/v3/auth/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "auth": {
      "identity": {
        "methods": ["password"],
        "password": {
          "user": {
            "name": "admin",
            "domain": {"name": "Default"},
            "password": "secret"
          }
        }
      },
      "scope": "unscoped"
    }
  }'

# Test token re-authentication
curl -X POST http://localhost:35357/v3/auth/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "auth": {
      "identity": {
        "methods": ["token"],
        "token": {"id": "<unscoped-token>"}
      },
      "scope": {
        "project": {
          "name": "default",
          "domain": {"name": "Default"}
        }
      }
    }
  }'
```

**Common Causes**:
1. **Service catalog using localhost instead of o3k**:
   - Symptom: "Unable to retrieve authorized projects" with HTTP 400
   - Fix: Ensure `buildHardcodedCatalog()` uses `baseURL := "http://o3k"`
   - Verify database endpoints: `docker exec o3k-postgres psql -U o3k -c "SELECT service_id, url FROM endpoints"`

2. **Token authentication not implemented**:
   - Symptom: HTTP 400 "password authentication required" during Step 3
   - Fix: Ensure `AuthenticateToken()` method exists in internal/keystone/auth.go
   - Verify handler routes both password and token methods

3. **Auto-scoping prevents unscoped tokens**:
   - Symptom: Step 1 returns token with project_id when it shouldn't
   - Fix: Remove auto-scoping logic - only add project when explicitly requested

**Resolution**:
1. Verify O3K container is healthy: `docker ps | grep o3k`
2. Check database migrations completed: `docker logs o3k | grep migration`
3. Verify default user exists: Check database or O3K logs
4. Ensure `local_settings` has correct `OPENSTACK_HOST = "o3k"`

### Login Redirect Loop

**Symptom**: Login form submits successfully, but returns to login page instead of dashboard. No error messages displayed.

**Diagnosis**:
```bash
# Check if Memcached is running
docker ps | grep memcached

# Test Memcached connectivity from Horizon
docker exec o3k-horizon /var/lib/kolla/venv/bin/python -c \
  "from pymemcache.client import base; \
   c = base.Client(('memcached', 11211)); \
   c.set('test', 'value'); \
   print(c.get('test'))"
# Should output: b'value'

# Check session configuration
docker exec o3k-horizon grep -A 5 "SESSION_ENGINE\|CACHES" \
  /etc/openstack-dashboard/local_settings.py

# Watch authentication flow in O3K logs
docker logs o3k -f
# Then try login in browser
```

**Root Cause**: Django sessions not persisting across Apache worker processes

**Common Causes**:
1. **Memcached container not running**:
   - Check docker-compose-horizon.yml includes memcached service
   - Verify: `docker ps | grep memcached`

2. **Wrong cache backend**:
   - LocMemCache (per-process) instead of PyMemcacheCache (shared)
   - Fix: Use `django.core.cache.backends.memcached.PyMemcacheCache`

3. **Memcached not accessible**:
   - Check network connectivity: `docker exec o3k-horizon ping memcached`
   - Verify LOCATION: `'memcached:11211'` (Docker service name, not localhost)

4. **File-based sessions with permission issues**:
   - Don't use file-based sessions with multiple Apache workers
   - Prefer Memcached for production reliability

**Resolution**:
1. Add Memcached container to docker-compose.yml:
   ```yaml
   memcached:
     image: memcached:1.6-alpine
     container_name: o3k-memcached
     hostname: memcached
     command: memcached -m 64
     networks:
       - o3k-network
   ```

2. Configure Horizon to use Memcached (in local_settings):
   ```python
   SESSION_ENGINE = 'django.contrib.sessions.backends.cache'
   CACHES = {
       'default': {
           'BACKEND': 'django.core.cache.backends.memcached.PyMemcacheCache',
           'LOCATION': 'memcached:11211',
       }
   }
   ```

3. Restart services:
   ```bash
   docker compose -f docker-compose-horizon.yml down
   docker compose -f docker-compose-horizon.yml up -d
   ```

4. Verify sessions work:
   - Login with admin/secret
   - Should redirect to `/dashboard/project/` (Overview page)
   - Check logs: `docker logs o3k-horizon --tail 50` for successful authentication

### Static Files Not Loading (CSS/JS Missing)

**Symptom**: Dashboard loads but has no styling, buttons don't work

**Diagnosis**:
```bash
# Check static files volume
docker volume inspect o3k-horizon-network_horizon-static

# Check Apache static alias
docker exec o3k-horizon grep "Alias /static" /etc/apache2/sites-enabled/000-default.conf

# Check file permissions
docker exec o3k-horizon ls -la /var/lib/kolla/venv/lib/python3.12/site-packages/static
```

**Resolution**:
1. Ensure volume mount is read-write: `horizon-static:/path:rw`
2. Check permissions in config.json (horizon:horizon for static directory)
3. Recreate static volume if corrupted:
   ```bash
   docker compose down
   docker volume rm o3k-horizon-network_horizon-static
   docker compose up -d
   ```

### Database Connection Errors

**Symptom**: O3K logs show "failed to connect to database"

**Diagnosis**:
```bash
# Check PostgreSQL container
docker logs o3k-postgres

# Test database connection from O3K container
docker exec o3k psql -h postgres -U o3k -d o3k -c "SELECT 1"
```

**Resolution**:
1. Verify PostgreSQL is healthy: `docker ps | grep postgres`
2. Check database URL in docker-compose.yml matches PostgreSQL credentials
3. Wait for database to become healthy before O3K starts (depends_on + healthcheck)

## Security Considerations

### Production Deployment

**CRITICAL - Change These Values**:

1. **JWT Secret** (O3K):
   ```yaml
   environment:
     - O3K_JWT_SECRET=<generate-strong-random-secret>
   ```
   Generate with: `openssl rand -base64 32`

2. **Database Password**:
   ```yaml
   environment:
     POSTGRES_PASSWORD: <strong-password>
     O3K_DB_URL: postgres://o3k:<strong-password>@postgres:5432/o3k
   ```

3. **Default Credentials**:
   - Change default admin password immediately after first login
   - Create separate admin accounts for different administrators
   - Use unique passwords for each account

### Network Security

1. **Expose Only Required Ports**:
   - Horizon (80/443): Publicly accessible
   - O3K API ports: Internal network only (remove `0.0.0.0` binding)
   - PostgreSQL: Never expose to internet

2. **Use HTTPS**:
   - Configure SSL/TLS certificate for Horizon
   - Modify Apache config to redirect HTTP → HTTPS
   - Update `OPENSTACK_KEYSTONE_URL` to use HTTPS if O3K behind load balancer

3. **Session Security**:
   - Configure `SESSION_COOKIE_SECURE = True` for HTTPS deployments
   - Set appropriate `SESSION_TIMEOUT` value
   - Use secure memcached configuration for session storage

### Database Security

1. **Database Isolation**:
   - Run PostgreSQL in private network
   - Use strong passwords
   - Restrict connections to O3K container only

2. **Regular Backups**:
   ```bash
   docker exec o3k-postgres pg_dump -U o3k o3k > backup.sql
   ```

## Platform Support

### x86_64 (AMD64)

**Status**: ✅ Fully Supported

All components work correctly:
- PostgreSQL 18.3
- O3K (Go binary)
- Horizon (Kolla image)
- noVNC

### ARM64 (Apple Silicon, Raspberry Pi, etc.)

**Status**: ⚠️ Partial Support

**Working**:
- PostgreSQL 18.3 (native ARM64 image available)
- O3K (Go compiles to native ARM64)

**Not Working**:
- Horizon: Kolla images (`quay.io/openstack.kolla/horizon:2025.2-ubuntu-noble`) are x86_64 only
- noVNC: Most images are x86_64 only

**Error**:
```
no matching manifest for linux/arm64/v8 in the manifest list entries
```

**Workaround**:
- Deploy O3K on ARM64 without Horizon
- Access O3K APIs directly via OpenStack CLI or API calls
- Deploy Horizon on separate x86_64 machine pointing to O3K APIs
- Use Docker's platform emulation (slow): `--platform linux/amd64`

## Performance Tuning

### Apache/WSGI

Adjust worker processes/threads in `horizon-nolist.conf`:

```apache
WSGIDaemonProcess horizon user=horizon group=horizon processes=5 threads=20
```

**Guidelines**:
- Processes: CPU cores / 2
- Threads: 10-20 per process
- Monitor memory: Each process ~100-200MB

### Database Connection Pool

O3K uses default PostgreSQL connection pool (20 connections). Increase if needed:

```go
// In O3K source code (internal/database/connect.go)
config, _ := pgxpool.ParseConfig(dbURL)
config.MaxConns = 50  // Increase from 20
```

### Caching

Configure Redis for Django cache (better than in-memory):

```python
# In local_settings
CACHES = {
    'default': {
        'BACKEND': 'django.core.cache.backends.redis.RedisCache',
        'LOCATION': 'redis://redis:6379/1',
    }
}
```

Add Redis container to docker-compose.yml.

## Monitoring

### Health Checks

All containers have health checks configured:

```bash
# Check health status
docker inspect o3k --format='{{.State.Health.Status}}'
docker inspect o3k-horizon --format='{{.State.Health.Status}}'
docker inspect o3k-postgres --format='{{.State.Health.Status}}'
```

### Metrics

**O3K Metrics** (if configured):
- Request latency
- API endpoint usage
- Database query performance

**Horizon Metrics**:
- Apache access logs: `/var/log/kolla/horizon/apache-access.log`
- Apache error logs: `/var/log/kolla/horizon/apache-error.log`
- Django logs: Check Horizon container logs

**Database Metrics**:
```bash
# Query performance
docker exec o3k-postgres psql -U o3k -c "SELECT * FROM pg_stat_statements ORDER BY total_time DESC LIMIT 10"

# Connection count
docker exec o3k-postgres psql -U o3k -c "SELECT count(*) FROM pg_stat_activity"
```

## API Version Compatibility Matrix

| Service  | O3K Version | Horizon Expects | Compatible? | Notes                          |
|----------|-------------|-----------------|-------------|--------------------------------|
| Keystone | v3          | v3              | ✅ Yes       | Identity API v3                |
| Nova     | v2.1        | v2.1            | ✅ Yes       | Compute API v2.1 (microversions) |
| Neutron  | v2.0        | v2.0            | ✅ Yes       | Network API v2.0               |
| Cinder   | v3          | v3              | ✅ Yes       | Volume API v3                  |
| Glance   | v2          | v2              | ✅ Yes       | Image API v2                   |

## References

- OpenStack Flamingo Documentation: https://docs.openstack.org/2025.2/
- Kolla Documentation: https://docs.openstack.org/kolla/latest/
- Horizon Configuration Reference: https://docs.openstack.org/horizon/latest/configuration/
- OpenStack API Reference: https://docs.openstack.org/api-ref/

## Contributing

When modifying Horizon configuration:

1. Test configuration changes in development first
2. Verify dashboard functionality after changes
3. Update this documentation with configuration changes
4. Test on both ARM64 (expected to fail) and x86_64 (must work)

## License

O3K is licensed under Apache License 2.0. Horizon and Kolla are OpenStack projects under Apache License 2.0.
