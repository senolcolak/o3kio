# Running O3K in a Test Environment

This guide covers how to run O3K with the server/agent architecture in a test environment, from stub mode (no KVM needed) to real mode (Linux with KVM/libvirt).

---

## Option 1: Stub Mode (Any OS — Development/Testing)

Runs everything in-process without real VMs or networking. Good for API testing, Terraform compatibility checks, and development.

### Prerequisites

- Go 1.26+
- Docker (for PostgreSQL)
- Terraform (optional, for compat-check)

### Start

```bash
# Start PostgreSQL
make db-up

# Run migrations
make migrate

# Start O3K in server mode (stub)
make run
# Or directly:
./bin/o3k server --config config/o3k.yaml
```

### Test it works

```bash
# Get a token
export OS_AUTH_URL=http://localhost:35357/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=default
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default

openstack token issue

# List flavors
openstack flavor list

# Create a server (stub mode — returns fake instance)
openstack server create --flavor m1.small --image cirros test-vm
openstack server list
```

### Run compat-check

```bash
make build-compat-check

# Against a Terraform directory
./bin/compat-check --dir /path/to/your/terraform --output json
```

---

## Option 2: Server + Agent (Stub Mode — Tests Task Queue)

Runs the async task dispatch pipeline without real VMs. Validates that the task queue, worker, reconciler, and agent executor all function correctly.

### Prerequisites

- Same as Option 1
- Two terminal windows (or run agent in background)

### Configure async mode

Edit `config/o3k.yaml`:

```yaml
nova:
  async_compute: true   # Enable task queue dispatch

tunnel:
  port: 6385
  token_secret: "my-test-secret"

tasks:
  max_workers: 2
  reconciler_interval_sec: 30
```

### Terminal 1: Start server

```bash
make build
./bin/o3k server --config config/o3k.yaml
```

You should see:
```
TunnelHub listening on :6385 (tls=false)
Task scheduler started: 2 workers, reconciler every 30s
```

### Terminal 2: Start agent

```bash
# Generate token for the agent
TOKEN=$(./bin/o3k token --config config/o3k.yaml --node-id test-agent-1)

# Start agent in stub mode
./bin/o3k agent --server 127.0.0.1:6385 --token-file <(echo $TOKEN)
```

### Test async VM creation

```bash
# Create a server — should return 202 immediately
openstack server create --flavor m1.small --image cirros async-vm

# Check status (should transition to ACTIVE via task queue)
openstack server show async-vm -f json | jq '.status'
```

### What happens under the hood

1. Nova handler inserts an `instances` row (status=BUILD) + `tasks` row (type=VM_CREATE)
2. `pg_notify('new_task', taskID)` wakes the worker
3. Worker claims the task (BeginTx + FOR UPDATE SKIP LOCKED)
4. Worker picks an agent with capacity (stats_updated_at < 30s, free vcpu/ram/disk)
5. Worker dispatches task via gRPC to the agent
6. Agent's executor calls `vmManager.CreateVM` (stub mode: in-memory)
7. Agent sends TaskResult back via the stream
8. Worker receives result, updates task to `completed`, instance to `ACTIVE`

### Verify task queue

```bash
# Connect to PostgreSQL and check tasks
psql -U o3k -d o3k -h localhost -c "SELECT id, type, status, retries, agent_id FROM tasks ORDER BY created_at DESC LIMIT 5;"

# Check compute_nodes
psql -U o3k -d o3k -h localhost -c "SELECT id, status, total_vcpu, reserved_vcpu, stats_updated_at FROM compute_nodes;"
```

---

## Option 3: Real Mode (Linux with KVM)

Runs actual VMs via libvirt. Requires a Linux host with KVM enabled.

### Prerequisites

- Linux (Ubuntu 22.04+ or similar)
- KVM enabled: `kvm-ok` should say "KVM acceleration can be used"
- libvirt installed: `apt install libvirt-daemon-system qemu-kvm`
- PostgreSQL (local or Docker)
- A cirros image at `/var/lib/o3k/images/cirros.qcow2`

### Configure real mode

```yaml
# config/o3k.yaml
nova:
  libvirt_mode: real
  libvirt_uri: "qemu:///system"
  async_compute: true

neutron:
  networking_mode: iptables

tunnel:
  port: 6385
  token_secret: "my-production-secret"

tasks:
  max_workers: 5
  reconciler_interval_sec: 30
```

### Prepare the host

```bash
# Download cirros test image
sudo mkdir -p /var/lib/o3k/images
sudo curl -o /var/lib/o3k/images/cirros.qcow2 \
  https://download.cirros-cloud.net/0.6.2/cirros-0.6.2-x86_64-disk.img

# Ensure o3k can access libvirt
sudo usermod -aG libvirt $(whoami)

# Create directories for DHCP
sudo mkdir -p /var/lib/o3k/dhcp/{hosts,configs,pids,leases}
sudo mkdir -p /var/lib/o3k/agent
```

### Start server + agent on same host

```bash
# Terminal 1: Server
./bin/o3k server --config config/o3k.yaml

# Terminal 2: Agent (real mode)
TOKEN=$(./bin/o3k token --config config/o3k.yaml --node-id agent-1)
./bin/o3k agent --server 127.0.0.1:6385 --token-file <(echo $TOKEN)
```

### Create a real VM

```bash
# Create network first
openstack network create test-net
openstack subnet create --network test-net --subnet-range 192.168.100.0/24 test-subnet

# Create server
openstack server create --flavor m1.tiny --image cirros --network test-net real-vm

# Wait for ACTIVE
watch "openstack server show real-vm -f json | jq '.status'"

# Verify VM exists in libvirt
virsh list --all
```

### Run the integration test

```bash
make test-vm-networking
```

---

## Option 4: Docker Compose (Everything Containerized)

Use the existing Docker Compose setup for a fully containerized test.

```bash
# Start all services (PostgreSQL + Redis + O3K)
docker compose -f deployments/docker-compose.yml up -d

# Wait for health
docker compose -f deployments/docker-compose.yml ps

# Run tests
make test-contract
make test-integration
```

---

## Troubleshooting

### "no eligible agent found" / tasks stuck in pending

The scheduler requires agents to have `stats_updated_at > now() - 30s`. If the agent hasn't sent a heartbeat yet (takes 10s after connect), tasks stay pending.

**Fix**: Wait 10 seconds after agent connects, then retry.

### "agent busy" errors

The inflight semaphore limits each agent to 1 concurrent task. If a task is in-flight, new dispatches return "agent busy" and the task stays pending until the current one completes.

**Fix**: This is working as designed. Add more agents for parallelism.

### Tasks stuck in "dispatched" forever

The reconciler scans every 30s for tasks past 2x their timeout. Default timeout is 120s, so reconciliation happens after 240s minimum.

**Fix**: Check `SELECT * FROM tasks WHERE status='dispatched'` and verify `dispatched_at`. Wait for reconciler or restart the server.

### Agent can't connect to server

```bash
# Check the tunnel port is open
nc -zv localhost 6385

# Check token is correct
./bin/o3k token --config config/o3k.yaml --node-id my-agent
```

### VM creation fails in real mode

```bash
# Check libvirt is running
systemctl status libvirtd

# Check KVM is available
ls /dev/kvm

# Check image exists
ls -la /var/lib/o3k/images/

# Check libvirt logs
journalctl -u libvirtd -f
```

---

## Configuration Reference

```yaml
# config/o3k.yaml — task queue settings

nova:
  async_compute: false    # true = use task queue, false = synchronous (default)

tunnel:
  port: 6385             # gRPC tunnel port (0 = disabled)
  token_secret: ""       # HMAC secret for join tokens
  token_file: ""         # Alternative: read secret from file

tasks:
  max_workers: 10        # Worker goroutines per server node
  reconciler_interval_sec: 30  # How often to scan for stalled tasks
```

---

## Architecture Summary

```
┌─────────────────────────────────────────────────┐
│                  o3k server                       │
│                                                   │
│  ┌──────────┐  ┌──────────┐  ┌──────────────┐  │
│  │ Keystone │  │   Nova   │  │   Neutron    │  │
│  │ :35357   │  │  :8774   │  │   :9696      │  │
│  └──────────┘  └────┬─────┘  └──────────────┘  │
│                      │                            │
│                      │ INSERT INTO tasks          │
│                      ▼                            │
│  ┌──────────────────────────────────────────┐   │
│  │         PostgreSQL (tasks table)          │   │
│  │   pending → dispatched → completed       │   │
│  └─────────────────┬────────────────────────┘   │
│                     │ pg_notify                    │
│                     ▼                             │
│  ┌──────────────────────────────────────────┐   │
│  │      Worker Pool (N goroutines)           │   │
│  │  Tx1: claim task + reserve agent          │   │
│  │  Dispatch: send via gRPC stream           │   │
│  │  Tx2: record result + release reservation │   │
│  └─────────────────┬────────────────────────┘   │
│                     │                             │
│  ┌──────────────────┴───────────────────────┐   │
│  │         TunnelHub (:6385)                 │   │
│  │  Inflight semaphore (max 1/agent)         │   │
│  │  Result channel routing                    │   │
│  └─────────────────┬────────────────────────┘   │
│                     │ gRPC bidirectional stream   │
└─────────────────────┼───────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│                  o3k agent                        │
│                                                   │
│  ┌──────────────────────────────────────────┐   │
│  │           Executor                        │   │
│  │  VM_CREATE  → libvirt DomainDefine+Create │   │
│  │  VM_DELETE  → libvirt Destroy+Undefine    │   │
│  │  VM_START   → libvirt DomainCreate        │   │
│  │  VM_STOP    → libvirt DomainShutdown      │   │
│  │  VM_REBOOT  → libvirt DomainReboot        │   │
│  │  NET_*      → networking primitives       │   │
│  └──────────────────────────────────────────┘   │
│                                                   │
│  ┌──────────────────────────────────────────┐   │
│  │       Heartbeat (every 10s)               │   │
│  │  Reports: vcpu_total, ram_mb_total        │   │
│  └──────────────────────────────────────────┘   │
└─────────────────────────────────────────────────┘
```
