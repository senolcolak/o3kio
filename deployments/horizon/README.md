# Horizon Dashboard Setup Guide

## Quick Start

This guide shows you how to access your running O3K instance via the OpenStack Horizon Dashboard.

## Prerequisites

- O3K running on your host machine (ports 35357, 8774, 9696, 8776, 9292)
- Docker and Docker Compose installed
- At least 2GB free RAM for Horizon container

## Option 1: Using Docker Compose (Recommended)

### Step 1: Start Horizon Dashboard

```bash
cd deployments/horizon
docker-compose up -d
```

This will:
- Start the Horizon container
- Connect it to O3K running on your host
- Expose the dashboard on http://localhost

### Step 2: Wait for Horizon to Start

```bash
# Check logs
docker-compose logs -f horizon

# Wait for "Apache/2.4.x (Unix) configured" message
```

Initial startup takes 30-60 seconds as Horizon collects static files.

### Step 3: Access the Dashboard

Open your browser and go to:
```
http://localhost/dashboard
```

### Step 4: Login

```
Domain: Default
Username: admin
Password: secret
```

### Step 5: Explore!

You should now see the Horizon dashboard with:
- **Project** tab - Overview, Instances, Volumes, Networks, etc.
- **Identity** tab - Projects, Users (if admin)
- **Admin** tab - System info, Hypervisors (if admin)

---

## Option 2: Manual Setup (Alternative)

If you prefer to run Horizon directly:

### Using Kolla Ansible

```bash
# Install kolla-ansible
pip install kolla-ansible

# Create configuration
mkdir -p /etc/kolla
cat > /etc/kolla/globals.yml <<EOF
kolla_base_distro: "ubuntu"
kolla_install_type: "source"
enable_horizon: "yes"
enable_keystone: "no"
enable_glance: "no"
enable_nova: "no"
enable_neutron: "no"
enable_cinder: "no"
keystone_internal_fqdn: "host.docker.internal"
keystone_admin_url: "http://host.docker.internal:35357/v3"
EOF

# Deploy Horizon
kolla-ansible deploy -i all-in-one
```

---

## Troubleshooting

### Issue: "Can't connect to Keystone"

**Solution:**
1. Verify O3K is running:
   ```bash
   curl http://localhost:35357/v3
   ```

2. Check O3K logs:
   ```bash
   tail -f /tmp/o3k*.log
   ```

3. Verify ports are accessible:
   ```bash
   netstat -an | grep -E "35357|8774|9696"
   ```

### Issue: "Horizon container won't start"

**Solution:**
```bash
# Check logs
docker-compose logs horizon

# Restart container
docker-compose restart horizon

# Rebuild if needed
docker-compose up -d --build
```

### Issue: "Can't login"

**Checklist:**
1. ✅ Domain: `Default` (case-sensitive)
2. ✅ Username: `admin`
3. ✅ Password: `secret`
4. ✅ O3K is running

**Test authentication manually:**
```bash
curl -i -X POST http://localhost:35357/v3/auth/tokens \
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
      "scope": {
        "project": {
          "name": "default",
          "domain": {"name": "Default"}
        }
      }
    }
  }'
```

You should get a `201 Created` response with `X-Subject-Token` header.

### Issue: "Dashboard loads but shows errors"

**Common causes:**
1. Service not running (Nova/Neutron/Cinder)
2. Database connection issue
3. API version mismatch

**Check O3K status:**
```bash
# Verify all services responding
curl -s http://localhost:35357/v3 | jq .
curl -s http://localhost:8774/v2.1/ | jq .
curl -s http://localhost:9696/v2.0/ | jq .
curl -s http://localhost:8776/v3/ | jq .
curl -s http://localhost:9292/v2/ | jq .
```

### Issue: "Page not found / 404 errors"

**Solution:**
Clear browser cache and refresh:
```
Ctrl + Shift + R (or Cmd + Shift + R on Mac)
```

Or access directly:
```
http://localhost/dashboard/auth/login/
```

---

## Configuration

### Custom Port

Edit `docker-compose.yaml`:
```yaml
horizon:
  ports:
    - "8080:80"  # Change 80 to your preferred port
```

Then restart:
```bash
docker-compose up -d
```

### Custom Theme

Edit `horizon_settings.py`:
```python
DEFAULT_THEME = 'material'  # Options: default, material
```

### Debug Mode

For troubleshooting, enable debug:

Edit `horizon_settings.py`:
```python
DEBUG = True
```

Then restart:
```bash
docker-compose restart horizon
```

**Warning:** Never enable DEBUG in production!

---

## Testing Horizon Features

### 1. Dashboard Overview
- Go to: **Project → Compute → Overview**
- Should show: Instance count, vCPU usage, RAM usage
- Check: Resource usage graphs

### 2. Launch Instance
- Go to: **Project → Compute → Instances**
- Click: **Launch Instance**
- Fill in:
  - **Details:** Instance name
  - **Source:** Select an image (e.g., cirros)
  - **Flavor:** Select flavor (e.g., m1.tiny)
  - **Networks:** Select a network
  - **Security Groups:** default
- Click: **Launch Instance**
- Should see: Instance in "Build" then "Active" state

### 3. Create Network
- Go to: **Project → Network → Networks**
- Click: **Create Network**
- Fill in:
  - **Network Name:** test-network
  - **Subnet:** Create subnet
  - **Subnet Name:** test-subnet
  - **CIDR:** 192.168.100.0/24
- Click: **Create**
- Should see: Network in list

### 4. Create Volume
- Go to: **Project → Volumes → Volumes**
- Click: **Create Volume**
- Fill in:
  - **Volume Name:** test-volume
  - **Size:** 10 GB
- Click: **Create**
- Should see: Volume in "Available" state

### 5. Security Groups
- Go to: **Project → Network → Security Groups**
- Click: **Create Security Group**
- Add rules for SSH, HTTP, ICMP
- Should see: Rules in list

### 6. Floating IPs
- Go to: **Project → Network → Floating IPs**
- Click: **Allocate IP to Project**
- Select external network
- Should see: Floating IP allocated

---

## Performance Tips

### Faster Page Loads

1. **Enable caching** in `horizon_settings.py`:
```python
CACHES = {
    'default': {
        'BACKEND': 'django.core.cache.backends.memcached.MemcachedCache',
        'LOCATION': 'memcached:11211',
    }
}
```

2. **Add memcached** to `docker-compose.yaml`:
```yaml
memcached:
  image: memcached:alpine
  container_name: o3k-memcached
  networks:
    - o3k-network
```

### Reduce API Calls

In `horizon_settings.py`:
```python
OPENSTACK_ENABLE_PASSWORD_RETRIEVE = False
API_RESULT_LIMIT = 100
API_RESULT_PAGE_SIZE = 20
```

---

## Production Configuration

### HTTPS Setup

1. Create SSL certificates:
```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout deployments/horizon/server.key \
  -out deployments/horizon/server.crt
```

2. Update `docker-compose.yaml`:
```yaml
horizon:
  volumes:
    - ./server.crt:/etc/ssl/certs/horizon.crt:ro
    - ./server.key:/etc/ssl/private/horizon.key:ro
```

3. Enable HTTPS in `horizon_settings.py`:
```python
CSRF_COOKIE_SECURE = True
SESSION_COOKIE_SECURE = True
SECURE_PROXY_SSL_HEADER = ('HTTP_X_FORWARDED_PROTO', 'https')
```

### Session Security

```python
SESSION_TIMEOUT = 3600  # 1 hour
SESSION_COOKIE_HTTPONLY = True
SESSION_COOKIE_SECURE = True  # HTTPS only
CSRF_COOKIE_SECURE = True
```

### Logging

```python
LOGGING = {
    'version': 1,
    'handlers': {
        'file': {
            'level': 'INFO',
            'class': 'logging.FileHandler',
            'filename': '/var/log/horizon/horizon.log',
        },
    },
    'loggers': {
        'django': {
            'handlers': ['file'],
            'level': 'INFO',
        },
    },
}
```

---

## Advanced: Multi-Region Setup

To add multiple O3K regions:

```python
AVAILABLE_REGIONS = [
    ('http://region1.example.com:35357/v3', 'RegionOne'),
    ('http://region2.example.com:35357/v3', 'RegionTwo'),
]
```

---

## Maintenance

### View Logs
```bash
docker-compose logs -f horizon
```

### Restart Horizon
```bash
docker-compose restart horizon
```

### Update Horizon
```bash
docker-compose pull horizon
docker-compose up -d
```

### Backup Configuration
```bash
cp horizon_settings.py horizon_settings.py.backup
```

### Clean Up
```bash
# Stop and remove
docker-compose down

# Remove volumes (database)
docker-compose down -v
```

---

## Quick Reference

### URLs
- Dashboard: http://localhost/dashboard
- Admin: http://localhost/dashboard/admin
- API: http://localhost:35357/v3

### Default Credentials
```
Domain: Default
User: admin
Password: secret
Project: default
```

### Common Paths in Container
```
Config: /etc/openstack-dashboard/
Static: /var/lib/kolla/venv/lib/python3.*/site-packages/static
Logs: /var/log/kolla/horizon/
```

### Docker Commands
```bash
# Start
docker-compose up -d

# Stop
docker-compose down

# Logs
docker-compose logs -f horizon

# Shell access
docker-compose exec horizon bash

# Restart
docker-compose restart horizon
```

---

## Screenshots Guide

### What You Should See

1. **Login Page**
   - O3K OpenStack Dashboard header
   - Domain, Username, Password fields
   - "Sign In" button

2. **Overview Page**
   - Instance count
   - vCPU usage chart
   - RAM usage chart
   - Security groups count
   - Volumes count

3. **Instances Tab**
   - List of running instances
   - Launch Instance button
   - Instance name, IP, status, power state

4. **Launch Instance Dialog**
   - Multi-step wizard
   - Details, Source, Flavor, Networks tabs
   - Form fields populated from O3K

5. **Networks Tab**
   - Network topology view (optional)
   - List of networks with subnets
   - Create Network button

---

## Support

### Getting Help

1. Check O3K logs: `tail -f /tmp/o3k*.log`
2. Check Horizon logs: `docker-compose logs horizon`
3. Verify services: `curl http://localhost:35357/v3`
4. Check documentation: `docs/HORIZON_TESTING_COMPLETE.md`

### Known Issues

1. **Limits endpoint not implemented** - Some dashboard metrics may not show
2. **VNC console not available** - Console tab will show limited info
3. **First load may be slow** - Horizon collects static files on startup

### Report Issues

File issues at: https://github.com/cobaltcore-dev/o3k/issues

Include:
- O3K version
- Horizon logs
- Browser console errors
- Steps to reproduce

---

## Success Checklist

After setup, verify:
- [ ] Login works
- [ ] Dashboard overview loads
- [ ] Instances tab loads
- [ ] Launch instance form opens
- [ ] Networks tab loads
- [ ] Volumes tab loads
- [ ] Security groups tab loads
- [ ] No console errors in browser

---

## Next Steps

1. ✅ Start Horizon: `docker-compose up -d`
2. ✅ Access dashboard: http://localhost/dashboard
3. ✅ Login with admin/secret
4. ✅ Explore all tabs
5. ✅ Try launching an instance
6. ✅ Create a network
7. ✅ Test all features

Enjoy your OpenStack dashboard! 🎉
