# Quick Start: Access O3K via Horizon Dashboard

## TL;DR - Super Quick Start

```bash
# 1. Make sure O3K is running
./o3k --config config/o3k.yaml

# 2. Start Horizon Dashboard (in a new terminal)
cd deployments/horizon
./start-horizon.sh

# 3. Open browser to http://localhost/dashboard
# Login: admin / secret (Domain: Default)
```

---

## What You Just Created

I've set up a complete Horizon Dashboard deployment for you with:

### Files Created:
1. **`deployments/horizon/docker-compose.yaml`** - Docker Compose configuration
2. **`deployments/horizon/horizon_settings.py`** - Horizon configuration for O3K
3. **`deployments/horizon/README.md`** - Complete setup guide
4. **`deployments/horizon/start-horizon.sh`** - One-click startup script

### How It Works:

```
┌─────────────────┐
│   Your Browser  │
│  localhost:80   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│     Horizon     │ ◄── Running in Docker
│   Dashboard     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│      O3K        │ ◄── Running on your Mac (host.docker.internal)
│   localhost:    │
│   35357, 8774,  │
│   9696, 8776    │
└─────────────────┘
```

---

## Step-by-Step Instructions

### Step 1: Ensure O3K is Running

In your current terminal:
```bash
# Check if O3K is running
curl http://localhost:35357/v3

# If not running, start it:
./o3k --config config/o3k.yaml
```

You should see JSON output with API version info.

### Step 2: Start Horizon Dashboard

Open a **new terminal** and run:
```bash
cd /Users/I761222/git/lightstack/deployments/horizon
./start-horizon.sh
```

This script will:
- ✅ Check O3K is running
- ✅ Verify Docker is installed
- ✅ Start Horizon container
- ✅ Wait for it to be ready
- ✅ Open your browser automatically

**First-time startup takes 30-60 seconds** as Horizon collects static files.

### Step 3: Login to Dashboard

The script should open your browser automatically to:
```
http://localhost/dashboard
```

If not, open it manually.

**Login credentials:**
```
Domain:   Default
Username: admin
Password: secret
```

### Step 4: Explore!

Once logged in, you'll see:

**Left Sidebar:**
- **Project** - Your cloud resources
  - Compute → Instances, Images, Key Pairs
  - Network → Networks, Routers, Security Groups, Floating IPs
  - Volumes → Volumes, Snapshots

- **Identity** - User management (admin only)
- **Admin** - System info (admin only)

**Top Menu:**
- User dropdown → Settings, Help, Sign Out
- Project dropdown → Switch projects

---

## What Can You Do?

### View Running Instances
1. Go to: **Project → Compute → Instances**
2. See all your VMs with status, IP, flavor
3. Click instance name for details

### Launch a New Instance
1. Click: **Launch Instance** button
2. Fill in form:
   - **Details:** Give it a name
   - **Source:** Choose "cirros" image
   - **Flavor:** Choose "m1.tiny"
   - **Networks:** Select a network
   - **Security Groups:** Keep "default"
3. Click: **Launch Instance**
4. Watch it build and become Active!

### Create a Network
1. Go to: **Project → Network → Networks**
2. Click: **Create Network**
3. Fill in:
   - **Network Name:** my-network
   - **Subnet:** Create subnet
   - **CIDR:** 192.168.100.0/24
4. Click: **Create**

### Manage Security Groups
1. Go to: **Project → Network → Security Groups**
2. Click: **Create Security Group**
3. Add rules:
   - SSH (TCP 22)
   - HTTP (TCP 80)
   - ICMP (Ping)

### Allocate Floating IP
1. Go to: **Project → Network → Floating IPs**
2. Click: **Allocate IP to Project**
3. Associate it with an instance

---

## Troubleshooting

### Can't Connect to Dashboard

```bash
# Check if Horizon container is running
cd deployments/horizon
docker compose ps

# Check logs
docker compose logs horizon

# Restart
docker compose restart horizon
```

### Can't Login

**Checklist:**
- ✅ Domain must be: `Default` (capital D)
- ✅ Username: `admin`
- ✅ Password: `secret`
- ✅ O3K must be running

**Test O3K authentication:**
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

Should return `201 Created` with a token.

### Dashboard Shows Errors

**Check O3K services:**
```bash
# Keystone (Identity)
curl http://localhost:35357/v3

# Nova (Compute)
curl http://localhost:8774/v2.1/

# Neutron (Network)
curl http://localhost:9696/v2.0/

# Cinder (Volumes)
curl http://localhost:8776/v3/

# Glance (Images)
curl http://localhost:9292/v2/
```

All should return JSON responses.

### Port Already in Use

If port 80 is taken:

Edit `docker-compose.yaml`:
```yaml
horizon:
  ports:
    - "8080:80"  # Use port 8080 instead
```

Then access: http://localhost:8080/dashboard

---

## Useful Commands

```bash
# View Horizon logs
docker compose logs -f horizon

# Restart Horizon
docker compose restart horizon

# Stop Horizon
docker compose down

# Stop and remove everything
docker compose down -v

# Access Horizon container shell
docker compose exec horizon bash
```

---

## What's Configured?

The Horizon setup includes:

### ✅ API Versions
- Identity (Keystone): v3
- Compute (Nova): v2.1
- Network (Neutron): v2.0
- Volume (Cinder): v3
- Image (Glance): v2

### ✅ Features Enabled
- Multi-domain support
- Security groups
- Routers
- Floating IPs
- Quotas
- Key pairs
- Volume types
- Image upload

### ✅ Features Disabled
- Load Balancer (LBaaS)
- VPN (VPNaaS)
- Firewall (FWaaS)
- Backups
- Distributed routers
- HA routers

### ✅ Connection
- Horizon runs in Docker container
- Connects to O3K on host via `host.docker.internal`
- All O3K services accessible

---

## Architecture

```
┌──────────────────────────────────────────┐
│           Your Mac (Host)                │
│                                          │
│  ┌────────────────────────────────────┐ │
│  │  O3K Services (./o3k binary)       │ │
│  │  - Keystone  :35357                │ │
│  │  - Nova      :8774                 │ │
│  │  - Neutron   :9696                 │ │
│  │  - Cinder    :8776                 │ │
│  │  - Glance    :9292                 │ │
│  └────────────────────────────────────┘ │
│                                          │
│  ┌────────────────────────────────────┐ │
│  │  Docker Container                  │ │
│  │  ┌──────────────────────────────┐ │ │
│  │  │  Horizon Dashboard           │ │ │
│  │  │  Port: 80 → localhost:80     │ │ │
│  │  │  Connects to host.docker...  │ │ │
│  │  └──────────────────────────────┘ │ │
│  └────────────────────────────────────┘ │
│                                          │
└──────────────────────────────────────────┘
          │
          ▼
   ┌─────────────┐
   │   Browser   │
   │ localhost   │
   └─────────────┘
```

---

## Next Steps

### Explore Horizon Features:
1. ✅ Dashboard overview
2. ✅ Launch an instance
3. ✅ Create a network
4. ✅ Create a volume
5. ✅ Set up security groups
6. ✅ Allocate floating IPs
7. ✅ Upload an image
8. ✅ Create SSH key pairs

### Test API Integration:
All Horizon actions translate to OpenStack API calls to your O3K instance!

### Customize:
- Change theme in `horizon_settings.py`
- Add custom branding
- Configure SSL/HTTPS
- Set session timeouts

---

## Summary

You now have:
- ✅ O3K running natively on your Mac
- ✅ Horizon Dashboard in Docker
- ✅ Full OpenStack UI access
- ✅ One-command startup script
- ✅ Complete documentation

**To start using it:**
```bash
# Terminal 1: Start O3K
./o3k --config config/o3k.yaml

# Terminal 2: Start Horizon
cd deployments/horizon
./start-horizon.sh

# Browser: http://localhost/dashboard
# Login: admin / secret
```

Enjoy your OpenStack cloud! 🎉
