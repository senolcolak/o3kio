# Design: o3k Server/Agent Scaling Architecture

**Date**: 2026-04-10
**Status**: Approved
**Author**: senol.colak

---

## Goal

Scale o3k from a single-node binary to a distributed system where one `o3k server`
command runs the control plane and multiple `o3k agent` commands run compute workers —
identical philosophy to k3s.

---

## Background

o3k currently runs all five OpenStack services in a single process on a single node.
Multi-node scaling today requires manual HAProxy + Ceph + keepalived setup (ops-heavy,
no native clustering). The k3s model — single binary, two subcommands, join token,
websocket back-channel — is the right template.

The existing `compute_nodes` table and `NodeRegistry` heartbeat code prove the
foundation is already there. This design replaces the DB-polling heartbeat model
with a live gRPC tunnel model.

---

## Scope

**In scope**: binary interface, join protocol, gRPC tunnel, async task queue,
scheduler with atomic reservation, agent local state, mTLS, HA topology.

**Out of scope**: live migration, VM evacuation on node failure, Ceph deployment
automation, Horizon integration changes.

---

## Design

### 1. Binary Interface

The `o3k` binary gains two top-level subcommands. Everything else stays as-is.

```
o3k server [--config path]
o3k agent  --server https://<host>:6385 --token <join-token> [--node-id <id>]
```

`o3k server` starts all existing services (Keystone, Nova, Neutron, Cinder, Glance,
Placement, Metadata) plus the TunnelHub gRPC server on `:6385`.

`o3k agent` starts only:
- gRPC client → dials TunnelHub, authenticates, enters task loop
- Local libvirt executor (VM lifecycle)
- Local netlink/VXLAN executor (network namespaces, bridges, security groups)

No OpenStack API ports open on agent nodes.

**Backward compatibility**: running `o3k` with no subcommand defaults to `o3k server`
behaviour so existing deployments are unaffected.

---

### 2. Join Token and mTLS

Join flow mirrors k3s exactly.

```bash
# On server node (token auto-generated at first start)
o3k server
cat /var/lib/o3k/server/node-token     # or: o3k token get

# On agent node — one command
o3k agent --server https://10.0.0.1:6385 --token K10abc...
```

**Token format**: `<cluster-id>:<HMAC-SHA256(node-password, cluster-secret)>`

**mTLS flow**:
1. Agent presents token on initial HTTP upgrade request.
2. Server verifies token, issues a short-lived TLS client certificate signed by
   the cluster CA: `CN=<node-id>, O=o3k-agents`.
3. Agent stores cert at `/var/lib/o3k/agent/client.crt`.
4. All subsequent gRPC connections use mTLS — no bearer token on the wire.
5. Server validates `O=o3k-agents` on every connection.

Node identity is a UUID auto-generated on first run, stored at
`/var/lib/o3k/agent/node-id`. Survives restarts. Sent in the HELLO message.

---

### 3. gRPC Tunnel — Three Independent Streams

Single HTTP/2 connection, three logical gRPC streams. Head-of-line blocking
on one stream does not affect the others.

```protobuf
service AgentTunnel {
  // Task dispatch: server sends Tasks, agent replies with TaskResults
  rpc TaskStream(stream TaskResult) returns (stream Task) {}

  // Stats: agent reports resource availability every 10s
  rpc StatsStream(stream AgentStats) returns (google.protobuf.Empty) {}

  // Heartbeat: 5s ping/pong, liveness only
  rpc HeartbeatStream(stream Heartbeat) returns (stream HeartbeatAck) {}
}

message Task {
  string id          = 1;  // UUID, correlation key
  string type        = 2;  // "vm.create", "net.ensure_namespace", etc.
  bytes  payload     = 3;  // JSON-encoded type-specific payload
  int32  timeout_sec = 4;
}

message TaskResult {
  string id      = 1;
  bool   ok      = 2;
  bytes  data    = 3;  // JSON-encoded result
  string error   = 4;
}

message AgentStats {
  string node_id      = 1;
  int32  vcpu_free    = 2;
  int64  ram_mb_free  = 3;
  int64  disk_gb_free = 4;
}
```

**Task types**:

| Domain | Types |
|--------|-------|
| VM | `vm.create`, `vm.delete`, `vm.start`, `vm.stop`, `vm.reboot`, `vm.get_state` |
| Network | `net.ensure_namespace`, `net.delete_namespace`, `net.add_port`, `net.remove_port` |
| Security | `net.apply_security_group`, `net.remove_security_group` |
| VXLAN | `vxlan.add_peer`, `vxlan.remove_peer` |
| Image | `image.prefetch` (pre-warm cache, separate from VM create) |

**Liveness**: HeartbeatStream drops → server marks agent `offline` immediately →
scheduler stops placing work on it. This is the fail-fast signal.

---

### 4. Async Task Queue — API Contract

Nova/Neutron/Cinder APIs return **202 Accepted** immediately. Client polls for status.
This is consistent with the real OpenStack API contract (already what clients expect).

```
POST /v2/{project}/servers
→ 202 Accepted
  {
    "server": {
      "id": "uuid",
      "status": "BUILD",
      "task_state": "scheduling"
    }
  }

GET /v2/{project}/servers/{id}
→ 200 OK  { "status": "ACTIVE" }    ← poll until this
```

**Task lifecycle in DB**:

```
pending → dispatched → completed
               ↘ failed (retries < 3) → pending (with next_retry_at delay)
               ↘ dead   (retries exhausted)
```

New `tasks` table:

```sql
CREATE TABLE tasks (
  id              UUID PRIMARY KEY,
  type            TEXT NOT NULL,
  resource_id     UUID NOT NULL,        -- instance_id, port_id, etc.
  agent_id        UUID,                 -- set after scheduling
  payload         JSONB NOT NULL,
  status          TEXT NOT NULL DEFAULT 'pending',
  retries         INT NOT NULL DEFAULT 0,
  next_retry_at   TIMESTAMPTZ,          -- set on failure, controls retry delay
  idempotency_key TEXT UNIQUE,          -- client-supplied X-Idempotency-Key
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  dispatched_at   TIMESTAMPTZ,
  completed_at    TIMESTAMPTZ,
  error           TEXT
);
```

**Background worker** (runs on every server node, coordinates via `FOR UPDATE SKIP LOCKED`):

```
loop every 500ms:
  tasks = SELECT ... FROM tasks WHERE status='pending'
          AND (next_retry_at IS NULL OR next_retry_at <= now())
          FOR UPDATE SKIP LOCKED
          LIMIT 10

  for each task:
    agent = Scheduler.Reserve(task)   -- atomic, see §5
    if no agent: skip (will retry)

    result = TunnelHub.Dispatch(agent, task, timeout)
    if error:
      task.retries++
      task.status = 'failed' if retries >= 3 else 'pending'
      task.next_retry_at = now() + exponential_backoff(retries)
    else:
      task.status = 'completed'
      UPDATE resource table (instances, ports, etc.)

    db.Save(task)
```

`FOR UPDATE SKIP LOCKED` ensures two server nodes never process the same task.
No distributed lock needed.

---

### 5. Scheduler — Atomic Resource Reservation

Stats from agents update the `compute_nodes` table. Scheduling is a two-step atomic
operation: **reserve then dispatch**. Reservation prevents double-booking between
concurrent schedulers on different server nodes.

```sql
-- New columns on compute_nodes
ALTER TABLE compute_nodes ADD COLUMN total_vcpu    INT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN total_ram_mb  BIGINT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN reserved_vcpu INT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN reserved_ram_mb BIGINT NOT NULL DEFAULT 0;
```

Scheduling transaction:

```sql
BEGIN;

SELECT id, reserved_vcpu, total_vcpu, reserved_ram_mb, total_ram_mb
FROM compute_nodes
WHERE status = 'connected'
  AND (total_vcpu - reserved_vcpu) >= $req_vcpu
  AND (total_ram_mb - reserved_ram_mb) >= $req_ram_mb
ORDER BY (total_vcpu - reserved_vcpu) DESC
LIMIT 1
FOR UPDATE;          -- row-level lock, prevents concurrent reservation

UPDATE compute_nodes
SET reserved_vcpu   = reserved_vcpu   + $req_vcpu,
    reserved_ram_mb = reserved_ram_mb + $req_ram_mb
WHERE id = $agent_id;

INSERT INTO tasks (...) VALUES (...);

COMMIT;
```

Reservation is released when task completes or fails (decrement `reserved_*`).
If server crashes before releasing: a reconciliation goroutine scans tasks in
`dispatched` state older than 2× timeout and releases their reservations.

---

### 6. Image Pre-Fetch — Decoupled from VM Create

Image pull is separated from VM creation. Two phases:

**Phase 1 — `image.prefetch` task** (timeout: 5 minutes, retryable):
- Dispatched by Nova when `POST /servers` is received, before `vm.create`
- Agent checks local image cache (SQLite index at `/var/lib/o3k/agent/images.db`)
- If not cached: pulls directly from Glance backend URL
- On success: task completes, `vm.create` task is queued

**Phase 2 — `vm.create` task** (timeout: 30s):
- Image is guaranteed local — no network I/O
- libvirt domain create only: fast, bounded

This means large images (20GB+) never cause `vm.create` to timeout. The prefetch
may be slow, but it's isolated, retryable, and doesn't block the scheduler.

---

### 7. Agent Local State

Agent maintains a SQLite database at `/var/lib/o3k/agent/state.db`:

```sql
-- Current in-flight task (at most one row)
CREATE TABLE current_task (
  task_id    TEXT PRIMARY KEY,
  type       TEXT NOT NULL,
  payload    TEXT NOT NULL,   -- JSON
  status     TEXT NOT NULL,   -- 'executing', 'completed', 'failed'
  result     TEXT,            -- JSON result
  error      TEXT,
  started_at TEXT NOT NULL
);

-- Image cache index
CREATE TABLE image_cache (
  image_id   TEXT PRIMARY KEY,
  local_path TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  cached_at  TEXT NOT NULL
);
```

**Reconnect recovery**:
1. Agent dials new server (via VIP).
2. After HELLO/HELLO_ACK, agent queries `current_task` for status `completed` or `failed`.
3. If found: sends `TaskResult` immediately to new server.
4. New server looks up `task_id` in `tasks` table and updates accordingly.
5. If server doesn't know the task (server crashed before writing to DB): agent
   logs warning and clears local state. Resource may be orphaned — operator tooling
   (`o3k node reconcile`) handles cleanup.

---

### 8. HA — Multiple Servers

Three or more server nodes behind a load balancer (or keepalived VIP). PostgreSQL
is the shared state store — no etcd required (already in use).

```
          ┌──────────────────────────────────────┐
          │  Load Balancer / VIP                 │
          │  :6385  (agent gRPC tunnel)          │
          │  :5000 :8774 :9696 :8776 :9292 (API) │
          └───────────┬──────────────┬────────────┘
                      │              │
              ┌───────▼──┐    ┌──────▼────┐
              │ Server 1 │    │ Server 2  │   active/active
              │ TunnelHub│    │ TunnelHub │   shared PostgreSQL
              │ Worker   │    │ Worker    │   FOR UPDATE SKIP LOCKED
              └──────────┘    └───────────┘
                      │              │
              ┌───────▼──────────────▼──────┐
              │        PostgreSQL            │
              │  tasks, compute_nodes, ...   │
              └──────────────────────────────┘
                      │
          ┌───────────┼────────────┐
      ┌───▼───┐   ┌───▼───┐   ┌───▼───┐
      │Agent 1│   │Agent 2│   │Agent 3│
      └───────┘   └───────┘   └───────┘
```

Agent connections are sticky to one server until that server dies. On disconnect,
agent reconnects to VIP (any healthy server). No state loss because all task state
lives in PostgreSQL and agent local SQLite.

---

### 9. Failure Mode Table

| Failure | Detection | Outcome |
|---------|-----------|---------|
| Agent gRPC stream drops | HeartbeatStream timeout (5s) | Agent marked `offline`, scheduler stops placing work, in-flight task retried |
| Server crashes mid-dispatch | Task stays `dispatched` in DB, reconciler detects after 2× timeout | Task retried on different agent |
| Agent crashes mid-execution | Agent local state records completion on restart, sent on reconnect | Result delivered to new server |
| Double-booking race | `SELECT FOR UPDATE` prevents concurrent reservation | One scheduler wins, other retries |
| Image pull timeout | `image.prefetch` task retried up to 3× with backoff | VM create queued only after successful prefetch |
| All agents offline | Scheduler finds no eligible agent | Task stays `pending`, 503 not returned — client polls until agent comes back |

---

### 10. New Files and Changes

**New packages**:
- `internal/tunnel/` — TunnelHub, gRPC server, stream management
- `internal/worker/` — background task worker, retry logic
- `internal/scheduler/` — atomic reservation, placement algorithm
- `pkg/agent/` — agent main loop, task executor, local state SQLite
- `proto/agent.proto` — gRPC service definition

**Modified**:
- `cmd/o3k/main.go` — add `server` / `agent` subcommand dispatch
- `internal/nova/` — return 202 on create, write task to DB instead of direct dispatch
- `internal/neutron/` — same pattern for port/network operations that touch compute
- `internal/compute/node_registry.go` — extend with `reserved_*` columns, replace
  heartbeat-via-DB with heartbeat-via-gRPC

**New DB migrations**:
- `tasks` table
- `compute_nodes` additions (`total_vcpu`, `reserved_vcpu`, `total_ram_mb`, `reserved_ram_mb`)

**Unchanged**: Keystone, Cinder, Glance, Placement, Metadata, middleware, existing
config structure (new `agent` section added to `Config`).

---

### 11. Configuration

New `agent` section in `config/o3k.yaml`:

```yaml
agent:
  server_url: "https://10.0.0.1:6385"
  token: ""                              # or K3S_TOKEN-style env var O3K_TOKEN
  node_id: "auto"                        # auto = UUID persisted to disk
  state_dir: "/var/lib/o3k/agent"
  heartbeat_interval: 5s
  stats_interval: 10s
  task_timeout: 30s
  image_prefetch_timeout: 5m
  image_cache_dir: "/var/lib/o3k/agent/images"
```

---

## What This Is Not

- Not a full scheduler (no anti-affinity, no availability zones in v1)
- Not live migration (VMs stay on their node)
- Not automatic recovery from agent death (VMs on dead agent stay in `ERROR` state,
  operator triggers evacuation manually)
- Not a replacement for Ceph (shared storage still required for image backends)

These are follow-on specs.

---

## Success Criteria

1. `o3k agent --server ... --token ...` joins cluster and receives work within 5s
2. `POST /servers` returns 202 in < 100ms regardless of VM create duration
3. Agent node failure detected within 10s (2× heartbeat interval)
4. No double-booking under concurrent load (verified by integration test)
5. Agent reconnect delivers in-flight task result to new server
6. Rolling server update causes zero task loss (tasks in DB survive)
