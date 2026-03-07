# O3K Docker Compose Deployment

This directory contains everything you need to run O3K (OpenStack-compatible cloud) and Horizon Dashboard using Docker Compose.

## Quick Start

```bash
# 1. Start all services
docker compose up -d

# 2. Wait for services to be ready (30-60 seconds)
docker compose logs -f o3k

# 3. Access Horizon Dashboard
open http://localhost/dashboard

# Login credentials:
#   Domain: Default
#   Username: admin
#   Password: secret
```

## What Gets Deployed

### Services:
1. **PostgreSQL** (port 5432) - Database for O3K state
2. **O3K** (ports 5000, 8774, 9696, 8776, 9292, 8775) - OpenStack API services
3. **Horizon** (ports 80, 443) - Web dashboard

### Network:
- All services run in the `o3k-network` bridge network
- Horizon communicates with O3K via internal Docker networking

### Volumes:
- `postgres_data` - Persistent database storage
- `horizon_static` - Horizon static files

## Architecture

```
┌─────────────────────────────────────────┐
│         Docker Compose Stack            │
├─────────────────────────────────────────┤
│                                         │
│  ┌──────────────┐   ┌───────────────┐  │
│  │   Horizon    │   │               │  │
│  │   :80, :443  │──▶│      O3K      │  │
│  └──────────────┘   │  :5000-9292   │  │
│                     └───────┬───────┘  │
│                             │           │
│                     ┌───────▼───────┐  │
│                     │  PostgreSQL   │  │
│                     │     :5432     │  │
│                     └───────────────┘  │
└─────────────────────────────────────────┘
         │
         ▼
   Your Browser
   localhost
```

## Prerequisites

- Docker 20.10+
- Docker Compose V2
- At least 4GB RAM available
- At least 10GB disk space

## Commands

### Start Services
```bash
# Start in background
docker compose up -d

# Start with logs
docker compose up

# Start specific service
docker compose up -d postgres o3k
```

### View Logs
```bash
# All services
docker compose logs -f

# Specific service
docker compose logs -f o3k
docker compose logs -f horizon
docker compose logs -f postgres
```

### Stop Services
```bash
# Stop all services
docker compose down

# Stop and remove volumes (WARNING: deletes database)
docker compose down -v
```

### Restart Services
```bash
# Restart all
docker compose restart

# Restart specific service
docker compose restart o3k
docker compose restart horizon
```

### Check Service Health
```bash
# Service status
docker compose ps

# Check O3K API
curl http://localhost:5000/v3

# Check Horizon
curl http://localhost/dashboard/
```

## Configuration

### Environment Variables

Create a `.env` file to customize:

```bash
# O3K Configuration
O3K_JWT_SECRET=your-secret-key-here
O3K_LOG_LEVEL=info
O3K_LOG_FORMAT=json

# Database Configuration
POSTGRES_PASSWORD=secret
POSTGRES_USER=lightstack
POSTGRES_DB=lightstack

# Horizon Configuration
OPENSTACK_HOST=o3k
DEBUG=False
```

### Ports

If you need to change ports (e.g., port 80 is taken):

Edit `docker-compose.yml`:
```yaml
horizon:
  ports:
    - "8080:80"  # Access Horizon on http://localhost:8080
```

## Using O3K

### Via Horizon Dashboard

1. Open: http://localhost/dashboard
2. Login:
   - Domain: `Default`
   - Username: `admin`
   - Password: `secret`
3. Explore:
   - Compute → Instances (VMs)
   - Network → Networks
   - Volumes → Volumes
   - Identity → Projects/Users

### Via OpenStack CLI

```bash
# Install CLI
pipx install python-openstackclient

# Set environment
export OS_AUTH_URL=http://localhost:5000/v3
export OS_PROJECT_NAME=default
export OS_PROJECT_DOMAIN_NAME=Default
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_USER_DOMAIN_NAME=Default
export OS_IDENTITY_API_VERSION=3

# Use it
openstack server list
openstack network list
openstack volume list
```

## Troubleshooting

### Issue: O3K won't start

**Check logs:**
```bash
docker compose logs o3k
```

**Common causes:**
- Database not ready → wait 10 seconds, restart O3K
- Port conflict → check if ports are free: `lsof -i:5000`
- Migration failed → check database connection

**Solution:**
```bash
docker compose restart o3k
```

### Issue: Horizon shows "Service Unavailable"

**Check O3K is running:**
```bash
curl http://localhost:5000/v3
```

**Check Horizon logs:**
```bash
docker compose logs horizon
```

**Solution:**
```bash
# Restart Horizon
docker compose restart horizon

# Or rebuild if config changed
docker compose up -d --force-recreate horizon
```

### Issue: Can't login to Horizon

**Checklist:**
- ✅ Domain: `Default` (capital D)
- ✅ Username: `admin`
- ✅ Password: `secret`
- ✅ O3K is running: `curl http://localhost:5000/v3`

**Test authentication manually:**
```bash
curl -i -X POST http://localhost:5000/v3/auth/tokens \
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

Should return `201 Created` with `X-Subject-Token` header.

### Issue: Port already in use

**Find what's using the port:**
```bash
lsof -i:80    # For Horizon
lsof -i:5000  # For Keystone
lsof -i:5432  # For PostgreSQL
```

**Solution:**
Either stop the conflicting service or change ports in `docker-compose.yml`.

### Issue: Horizon works but shows errors for some tabs

**Common causes:**
- Specific O3K service not fully implemented
- API version mismatch

**Check O3K logs:**
```bash
docker compose logs o3k | grep ERROR
```

**Check which API is failing:**
```bash
# Test each service
curl http://localhost:5000/v3         # Keystone (Identity)
curl http://localhost:8774/v2.1/      # Nova (Compute)
curl http://localhost:9696/v2.0/      # Neutron (Network)
curl http://localhost:8776/v3/        # Cinder (Volumes)
curl http://localhost:9292/v2/        # Glance (Images)
```

### Issue: Database connection failed

**Check PostgreSQL is running:**
```bash
docker compose ps postgres
```

**Check database logs:**
```bash
docker compose logs postgres
```

**Solution:**
```bash
# Restart database
docker compose restart postgres

# Wait for it to be healthy
docker compose ps
```

## ARM Mac Considerations

**Important:** Horizon uses x86 Docker images which may not work well on ARM Macs (Apple Silicon).

If Horizon doesn't work on ARM Mac:
1. Use OpenStack CLI instead (see above)
2. Or connect from an x86 machine/server

The O3K service itself is built from source and will work natively on ARM.

## Production Deployment

For production use:

1. **Change default passwords:**
   ```bash
   # In .env file
   O3K_JWT_SECRET=$(openssl rand -base64 32)
   POSTGRES_PASSWORD=$(openssl rand -base64 32)
   ```

2. **Enable HTTPS for Horizon:**
   - Add SSL certificates
   - Update `docker-compose.yml` to mount certs
   - Set `DEBUG=False` in environment

3. **Use external PostgreSQL:**
   - Remove postgres service from docker-compose.yml
   - Point O3K to external database via O3K_DB_URL

4. **Resource limits:**
   Add to `docker-compose.yml`:
   ```yaml
   o3k:
     deploy:
       resources:
         limits:
           cpus: '2'
           memory: 4G
   ```

5. **Backup strategy:**
   ```bash
   # Backup database
   docker compose exec postgres pg_dump -U lightstack lightstack > backup.sql

   # Restore database
   cat backup.sql | docker compose exec -T postgres psql -U lightstack lightstack
   ```

## Development

### Rebuild O3K after code changes:
```bash
# Rebuild and restart
docker compose up -d --build o3k

# View startup logs
docker compose logs -f o3k
```

### Access O3K container:
```bash
docker compose exec o3k /bin/sh
```

### Access PostgreSQL:
```bash
docker compose exec postgres psql -U lightstack -d lightstack
```

### Run migrations manually:
```bash
docker compose exec o3k /app/o3k-migrate status
docker compose exec o3k /app/o3k-migrate up
```

## Monitoring

### Service Health
```bash
# Check health status
docker compose ps

# Expected output:
# o3k        running (healthy)
# postgres   running (healthy)
# horizon    running (healthy)
```

### Resource Usage
```bash
# Container stats
docker stats

# Disk usage
docker compose exec postgres du -sh /var/lib/postgresql/data
```

### Logs
```bash
# Follow all logs
docker compose logs -f

# Export logs
docker compose logs > o3k-logs.txt
```

## Cleanup

### Remove everything:
```bash
# Stop and remove containers, networks
docker compose down

# Also remove volumes (deletes database)
docker compose down -v

# Remove images
docker rmi $(docker images 'o3k*' -q)
```

### Fresh start:
```bash
docker compose down -v
docker compose up -d --build
```

## Support

- **Documentation:** See `docs/` directory
- **Issues:** https://github.com/cobaltcore-dev/o3k/issues
- **Logs:** Run `docker compose logs` to troubleshoot

## Quick Reference

| Service | Port | URL | Purpose |
|---------|------|-----|---------|
| Keystone | 5000 | http://localhost:5000/v3 | Identity/Auth |
| Nova | 8774 | http://localhost:8774/v2.1 | Compute/VMs |
| Neutron | 9696 | http://localhost:9696/v2.0 | Networking |
| Cinder | 8776 | http://localhost:8776/v3 | Block Storage |
| Glance | 9292 | http://localhost:9292/v2 | Images |
| Metadata | 8775 | http://localhost:8775 | VM Metadata |
| Horizon | 80 | http://localhost/dashboard | Web UI |
| PostgreSQL | 5432 | localhost:5432 | Database |

## Next Steps

1. ✅ Start services: `docker compose up -d`
2. ✅ Wait for healthy: `docker compose ps`
3. ✅ Access Horizon: http://localhost/dashboard
4. ✅ Or use OpenStack CLI (see above)
5. ✅ Create VMs, networks, volumes!

Enjoy your OpenStack cloud! 🚀
