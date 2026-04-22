# Design: o3k Server/Agent Scaling Architecture

**Date**: 2026-04-10
**Status**: Approved
**Author**: senol.colak
**Version**: 1.4.0
**Review**: CEO review 2026-04-20 — 29 issues addressed (7 critical, 12 warnings,
4 test gaps, 6 additional: performance, observability, deployment, trajectory);
Eng review 2026-04-22 — 10 issues addressed (3 arch, 3 code quality, 2 test gaps,
2 performance);
CEO review 2026-04-22 — 47 issues addressed (4 critical, 23 warnings, 20 info:
error/rescue map, security gaps, HMAC encoding, reconciler retries, stream timeouts,
concurrent prefetch, instance lifecycle, test coverage, observability)

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
gRPC back-channel — is the right template.

The existing `compute_nodes` table and `NodeRegistry` heartbeat code prove the
foundation is already there. This design replaces the DB-polling heartbeat loop
with a live gRPC HeartbeatStream signal. The `compute_nodes` table is retained as
the atomic resource reservation store for the scheduler.

**Note**: The existing `NodeRegistry.NewNodeRegistry()` regenerates the UUID on
every call. The agent must be changed to persist the generated UUID to
`/var/lib/o3k/agent/node-id` and reload it on restart.

---

## Architecture Principle Trade-off

The existing core principle "Synchronous Operations: No async state machines —
operations complete before API returns" cannot hold when compute operations execute
on remote agents over a network. A synchronous `POST /servers` that blocks for 30s
on a remote libvirt call violates fail-fast, exhausts HTTP connections, and breaks
under load balancer timeouts.

This design introduces an async task queue for operations dispatched to remote agents.
The tunnel itself remains synchronous and fail-fast: a dead agent produces an
immediate error in the worker. The API layer returns 202 Accepted (which is already
the real OpenStack API contract — Terraform and gophercloud expect it).

**This supersedes the synchronous-operations principle for multi-node compute and
network operations only.** Single-node deployments (`o3k` with no subcommand) retain
synchronous behavior. Keystone, Glance metadata, Cinder (non-compute), and Placement
remain synchronous.

When `docs/ARCHITECTURE.md` is updated for this feature, the "Design Philosophy"
section must note this exception.

---

## Scope

**In scope**: binary interface, join protocol, gRPC tunnel, async task queue,
scheduler with atomic reservation, agent local state, mTLS, HA topology, test
strategy, observability.

**Out of scope**: live migration, VM evacuation on node failure, Ceph deployment
automation, Horizon integration changes.

---

## Design

### 1. Binary Interface

The `o3k` binary gains two top-level subcommands. Everything else stays as-is.

```
o3k server [--config path]
o3k agent  --server https://<host>:6385 --token-file /etc/o3k/token [--node-id <id>]
```

**Implementation prerequisites** — add to `go.mod` before writing any code:
```bash
go get google.golang.org/grpc@latest
go get modernc.org/sqlite@latest
```

**Token input** (priority order):
1. `--token-file /path/to/file` (preferred — no process list exposure)
2. `O3K_TOKEN` environment variable
3. `--token <value>` (dev/testing only — emits warning: "token visible in process list")

`o3k server` starts all existing services (Keystone, Nova, Neutron, Cinder, Glance,
Placement, Metadata) plus the TunnelHub gRPC server on `:6385`.

`o3k agent` starts only:
- gRPC client -> dials TunnelHub, authenticates, enters task loop
- Local libvirt executor (VM lifecycle)
- Local netlink/VXLAN executor (network namespaces, bridges, security groups)

No OpenStack API ports open on agent nodes.

**`async_compute` flag** — gates the 202 behavior. Single-node deployments (no
`o3k agent` nodes) use synchronous mode by default. Set this in `config/o3k.yaml`
to enable async dispatch:

```yaml
nova:
  async_compute: false   # default — synchronous, original behavior
                         # set true when running o3k agent nodes
```

When `async_compute: false` (default), `POST /servers` returns the existing
synchronous response. When `async_compute: true` and no eligible agent is found,
`POST /servers` returns 503 (not 202 + silent hang).

**Backward compatibility**: running `o3k` with no subcommand defaults to `o3k server`
behaviour so existing deployments are unaffected. Subcommand dispatch uses
`flag.NewFlagSet` per subcommand to avoid breaking existing `--config` flag parsing:

```go
func main() {
    if len(os.Args) < 2 || !isSubcommand(os.Args[1]) {
        runServer(os.Args[1:])  // backward compat
        return
    }
    switch os.Args[1] {
    case "server":
        runServer(os.Args[2:])
    case "agent":
        runAgent(os.Args[2:])
    case "token":
        runTokenCmd(os.Args[2:])
    }
}
```

---

### 2. Join Token and mTLS

Join flow mirrors k3s, with added security hardening.

```bash
# On server node (token auto-generated at first start)
o3k server
o3k token get                     # requires root or o3k service user

# On agent node
o3k agent --server https://10.0.0.1:6385 --token-file /etc/o3k/token
```

**Token format**: `O3K<version>:<cluster-id>:<HMAC-SHA256(node-password, cluster-secret)>`

The `O3K` prefix distinguishes from k3s tokens. Version field enables future rotation.

**Token single-use enforcement**: The join token is single-use. When an agent first
joins, the server records the token as used in the `join_tokens` table (columns:
`token_hash TEXT PRIMARY KEY, used_at TIMESTAMPTZ`). Subsequent use of the same token
returns gRPC `PERMISSION_DENIED`. This prevents replay attacks if a token is intercepted.
`token_hash` stores `SHA-256(raw_token)` — never the raw token. The `join_tokens` row
is created atomically with the agent's `compute_nodes` row during the first join.

**Cluster-secret requirements**:
- Generated with 256 bits of CSPRNG entropy at first server start
- Stored at `/var/lib/o3k/server/node-token` with mode `0600`, owned by o3k service user
- `o3k token get` requires root or o3k service user — enforced via `os.Getuid()` check
  in the binary; exits with error: "permission denied — run as root or o3k service user"
- `o3k token rotate` generates a new secret with a configurable grace period
  (both old and new secrets validate during grace window, then old is invalidated)

**Token rotation — connected agents**: During rotation, agents that are already
connected (authenticated via mTLS client cert) are **not disconnected**. Their
established mTLS session remains valid regardless of token secret changes — the
token is only used during the initial join, not on reconnects. Agents must
re-authenticate with the new token only if their cert is revoked and they need to
rejoin. This means token rotation does not cause a cluster disruption for running
agents. Document this explicitly in the operator runbook for `o3k token rotate`.

**Join endpoint rate-limiting**: max 5 attempts per source IP per minute. Failed
verifications increment a per-IP counter; after 10 failures, IP blocked for 5 minutes.

**mTLS flow**:
1. Agent presents token on initial gRPC connection metadata.
2. Server verifies token, checks `node-id` -> `public-key-fingerprint` binding in
   `compute_nodes` table (if first join, creates binding; if existing, validates).
3. Server issues a short-lived TLS client certificate (90-day expiry) signed by
   the cluster CA: `CN=<node-id>, O=o3k-agents`.
4. Agent stores cert at `/var/lib/o3k/agent/client.crt`.
5. All subsequent gRPC connections use mTLS — no bearer token on the wire.
6. Server validates `O=o3k-agents` on every connection.

**Certificate lifecycle**:
- Expiry: 90 days. Agent requests renewal before expiry via a `CertRenew` RPC.
- Revocation: `revoked_agent_certs` table in PostgreSQL (`serial_number`, `node_id`,
  `revoked_at`). TunnelHub checks this table on every new connection.
  **If the revocation DB query fails (e.g., DB unavailable), the connection is
  refused with gRPC status `UNAVAILABLE` — fail closed, never fail open.**
  This prevents a DB outage from allowing revoked agents to reconnect.
- `o3k node deregister <node-id>` writes to revocation table and marks node `down`.
- Agent attempting to join with a revoked cert gets connection refused.

**Graceful drain on deregister**: `o3k node deregister <node-id>` follows a drain
sequence before revoking the cert:
1. Set node `status = 'maintenance'` in `compute_nodes` — suppresses new dispatch
2. Wait up to `drain_timeout` (default: `60s`) for in-flight tasks to complete
3. After drain or timeout: mark node `down`, write revocation record,
   terminate the agent's gRPC stream with `UNAVAILABLE`

```yaml
server:
  deregister_drain_timeout: 60s  # default
```

Tasks still dispatched at drain start are allowed to complete normally. Tasks that
do not complete within `drain_timeout` are requeued by the reconciler after the
node goes `down`. No tasks are silently dropped.

**Node identity**: UUID auto-generated on first run, persisted to
`/var/lib/o3k/agent/node-id`. On HELLO, server checks for existing rows matching
the agent's `hostname` even if `node_id` differs (handles lost node-id file).

**CA key distribution for HA**: The cluster CA private key lives at
`/var/lib/o3k/server/ca.key`, generated once on the first server node, and must
be distributed to all server nodes via operator tooling (Vault, k8s Secret, or
manual `scp`). All server nodes must share the same CA to accept each other's
agent certificates.

**HA CA key model — explicit decision**: All server nodes hold a copy of `ca.key`.
This is a conscious tradeoff: replication of the private key across nodes vs. a
single CA node that becomes a signing bottleneck. For v1.1.0, replication is the
correct choice — it avoids a single-point-of-failure for `CertRenew`. Vault/HSM
integration is a follow-on spec.

**CA key rotation procedure** (manual, documented before v1.0 release):
1. Generate new CA key+cert on one server node
2. Distribute new CA cert to all server nodes' `ca.crt` (old cert stays as intermediate)
3. Distribute new CA key to all server nodes' `ca.key` (rolling, one at a time)
4. Issue new agent certs signed by new CA (`o3k node reissue-all`)
5. Remove old CA cert from trust bundle
No agent downtime required — agents hold both old and new certs during transition.

**CA key file permissions**: `ca.key` must be created with mode `0600`, owned by
the o3k service user. On startup, the server reads `ca.key` and checks:
- File mode is exactly `0600` (not `0644`, not `0640`)
- File is owned by the current process uid

If either check fails, the server refuses to start with a clear error:
`"CA key at /var/lib/o3k/server/ca.key is readable by other users (mode 0644) — refusing to start. Fix: chmod 0600 /var/lib/o3k/server/ca.key"`

**TunnelHub bind address**: The gRPC server must bind to an internal interface only.
Configure `server.tunnel_bind_addr` (default `0.0.0.0` with a warning in startup
logs). Production deployments must set this to the internal network interface IP.
A misconfigured `0.0.0.0` binding on a public-facing host exposes the join endpoint
to the internet — this is a critical security misconfiguration.

```yaml
server:
  tunnel_bind_addr: "10.0.0.1"   # REQUIRED in production — internal interface only
  tunnel_port: 6385
```

**CertRenew — CA key unavailable**: If a server node does not have the CA key
(common if the operator distributed it to only some nodes), `CertRenew` must return
gRPC status `FAILED_PRECONDITION` with message "CA key not available on this server
— retry on a different server node". The agent retries via VIP (which may route to
a node that has the CA key). Agents must log this error and the impending cert expiry
prominently.

---

### 3. gRPC Tunnel — Three Independent Streams

Single HTTP/2 connection, three logical gRPC streams. Head-of-line blocking
on one stream does not affect the others.

```protobuf
service AgentTunnel {
  // One-shot registration: agent identifies itself, server acknowledges
  rpc Register(Hello) returns (HelloAck) {}

  // Bidirectional: server sends Tasks, agent replies with TaskResults
  rpc TaskStream(stream TaskResult) returns (stream Task) {}

  // Bidirectional: agent reports stats, server acknowledges per-message
  rpc StatsStream(stream AgentStats) returns (stream StatsAck) {}

  // Bidirectional: 5s ping/pong, liveness only
  rpc HeartbeatStream(stream Heartbeat) returns (stream HeartbeatAck) {}

  // Certificate renewal
  rpc CertRenew(CertRenewRequest) returns (CertRenewResponse) {}
}

message Hello {
  string node_id   = 1;
  string hostname  = 2;
  repeated string cached_images = 3;
  repeated OrphanReport orphans = 4;  // max 10 entries; server rejects excess
}

message HelloAck {
  string cluster_id  = 1;
  string server_id   = 2;
  bytes  server_nonce = 3;  // single-use nonce for TaskResult authentication
}

message OrphanReport {
  string task_id      = 1;
  string task_type    = 2;
  bytes  result       = 3;
  string completed_at = 4;
}

enum TaskType {
  TASK_TYPE_UNSPECIFIED       = 0;
  VM_CREATE                   = 1;
  VM_DELETE                   = 2;
  VM_START                    = 3;
  VM_STOP                     = 4;
  VM_REBOOT                   = 5;
  VM_GET_STATE                = 6;
  NET_ENSURE_NAMESPACE        = 7;
  NET_DELETE_NAMESPACE         = 8;
  NET_ADD_PORT                = 9;
  NET_REMOVE_PORT             = 10;
  NET_APPLY_SECURITY_GROUP    = 11;
  NET_REMOVE_SECURITY_GROUP   = 12;
  VXLAN_ADD_PEER              = 13;
  VXLAN_REMOVE_PEER           = 14;
  IMAGE_PREFETCH              = 15;
}

message Task {
  string                     id      = 1;
  TaskType                   type    = 2;
  bytes                      payload = 3;  // validated against type before dispatch
  google.protobuf.Duration   timeout = 4;
  int64                      max_payload_bytes = 5;  // enforced: default 64KB
}

message TaskResult {
  string    id         = 1;
  bytes     data       = 2;  // populated on success
  string    error      = 3;  // empty = success, non-empty = failure
  ErrorCode code       = 4;
  bytes     result_mac = 5;  // HMAC(server_nonce + task_id + data/error, agent_key)
                             // Inputs are length-prefixed (4-byte big-endian):
                             //   len(nonce)||nonce||len(task_id)||task_id||len(data_or_error)||data_or_error
                             // Verified with crypto/subtle.ConstantTimeCompare.
                             // On HMAC failure: treat task as ERROR_TRANSIENT (Tx2 retries
                             // it) while the per-agent failure counter increments.
                             // After 3 consecutive HMAC failures from the same agent:
                             // deregister the agent (write revocation record, close stream).
                             // The 3-failure window resets on each successful verification.
}

enum ErrorCode {
  ERROR_NONE      = 0;
  ERROR_TRANSIENT = 1;  // retry with backoff
  ERROR_PERMANENT = 2;  // skip retries, go to failed immediately
  ERROR_TIMEOUT   = 3;  // counted as transient
}

message AgentStats {
  string   node_id       = 1;
  int64    vcpu_total    = 2;  // physical capacity (stable)
  int64    ram_mb_total  = 3;
  int64    disk_gb_total = 4;
  repeated string cached_images = 5;
}

message StatsAck {
  string node_id    = 1;
  int64  server_seq = 2;
}
```

**Task payload validation**: Server validates payload against `TaskType` before
writing to the `tasks` table. Invalid payloads are rejected with `ERROR_PERMANENT`.
Maximum payload size: 64KB (configurable). Typed payload structs for each TaskType
are defined in a companion file (`proto/payloads.proto`).

**`proto/payloads.proto` — required message schemas** (implementors must define all
15 types before any implementation begins; these are the minimum required fields):

| TaskType | Proto message name | Key fields |
|----------|--------------------|------------|
| `VM_CREATE` | `VMCreatePayload` | `instance_id`, `flavor_id`, `image_local_path`, `keypair_name`, `network_id`, `vcpu`, `ram_mb`, `disk_gb` |
| `VM_DELETE` | `VMDeletePayload` | `instance_id`, `domain_name` |
| `VM_START` / `VM_STOP` / `VM_REBOOT` | `VMStatePayload` | `instance_id`, `domain_name` |
| `VM_GET_STATE` | `VMGetStatePayload` | `instance_id`, `domain_name` |
| `NET_ENSURE_NAMESPACE` | `NetNamespacePayload` | `network_id`, `project_id`, `vxlan_vni` (optional) |
| `NET_DELETE_NAMESPACE` | `NetDeleteNamespacePayload` | `network_id` |
| `NET_ADD_PORT` / `NET_REMOVE_PORT` | `NetPortPayload` | `port_id`, `network_id`, `mac_address`, `ip_address`, `instance_id` |
| `NET_APPLY_SECURITY_GROUP` / `NET_REMOVE_SECURITY_GROUP` | `NetSecGroupPayload` | `port_id`, `rules` (repeated `SecurityGroupRule`) |
| `VXLAN_ADD_PEER` / `VXLAN_REMOVE_PEER` | `VXLANPeerPayload` | `vni`, `peer_ip`, `peer_mac` |
| `IMAGE_PREFETCH` | `ImagePrefetchPayload` | `image_id`, `download_url`, `checksum`, `size_bytes`, `image_format` |

Each payload message must include a `version` field (int32, default 1) for future
schema migration. The contract test `TestProto_TaskStreamRoundTrip` must cover all
15 types.

**Agent-side payload size enforcement**: Agents enforce the same 64KB limit on
received `Task.payload` independently — they do not trust the server's enforcement.
Tasks exceeding the limit are rejected with `ERROR_PERMANENT` and logged.

**Register error handling**: If `TunnelHub.Register()` cannot reach the database
(e.g., connection pool exhausted, DB unavailable), it must return gRPC status
`UNAVAILABLE` immediately — never leave the agent hanging. The agent retries with
exponential backoff (1s, 2s, 4s, 8s, cap 60s).

**`Register()` decomposition**: The `Register` handler must be decomposed into
distinct steps — token verification, node-id binding, cert issuance, OrphanReport
processing, and semaphore initialization — with clear error returns from each step.
A monolithic `Register()` function is hard to test. Each step must be testable
independently.

**Duplicate registration (same node_id, live stream)**: If a `Register` call arrives
for a `node_id` that already has an active gRPC stream on this server, the handler must:
1. Close the old stream (send gRPC stream close signal).
2. Atomically replace the in-memory `agentConn` entry in `TunnelHub`.
3. Update `agent_stream_server_id` in `compute_nodes` to the new connection.
4. Log an `agent.reconnect` audit event.
This handles mobile network blips and TCP connection GC lag — both cause the old
connection to linger while the agent already re-registered. Do not reject the new
registration because the old stream is still technically open.

**OrphanReport deduplication**: Server processes OrphanReports via upsert on
`task_id`. If the agent reconnects repeatedly (crash loop) and sends the same
OrphanReport multiple times, each is idempotent. The `current_task` table in agent
SQLite is only cleared after the server's `HelloAck` confirms receipt.

**OrphanReport validation**: For `VM_CREATE` OrphanReports, the server must validate
that the reported `task_id` corresponds to a known `VM_CREATE` task in the `tasks`
table before reconciling the instance row. A fabricated `task_id` that does not exist
in `tasks` is rejected with a structured error log — no instance row is created.
This prevents a compromised agent from inserting arbitrary instance records.

**Liveness**: HeartbeatStream drops -> server marks agent `offline`. Agent `offline`
status suppresses new dispatch only. In-flight `TaskResult` messages from offline
agents MUST be accepted and processed. Task requeueing happens only after
`task.timeout` expires with no result received.

**Server-side per-agent concurrency**: TunnelHub enforces max in-flight tasks per
agent (default: 1 for v1). Dispatch returns `ErrAgentBusy` immediately if
`inflight >= max_agent_inflight`. Worker treats `ErrAgentBusy` as a skip (task
stays `pending`, worker moves to next poll cycle).

```go
type agentConn struct {
    stream   AgentTunnel_TaskStreamServer
    inflight atomic.Int32  // max 1 for v1
}

// Dispatcher is the interface TunnelHub exposes to the worker.
// This interface must be defined so the worker can be tested with a mock TunnelHub.
type Dispatcher interface {
    // Dispatch sends task to the agent and waits for the result.
    // Returns ErrAgentBusy if the agent already has an in-flight task.
    // Returns ErrAgentGone if the agent disconnected before or during dispatch.
    // The context must carry a deadline (context.WithTimeout using task.timeout_sec).
    Dispatch(ctx context.Context, agentID uuid.UUID, task Task) (TaskResult, error)
}
```

---

### 4. Async Task Queue — API Contract

Nova/Neutron APIs return **202 Accepted** immediately when `async_compute: true`.
Client polls for status. When `async_compute: false` (default), existing synchronous
behavior is preserved.

**Note**: Cinder is not modified in v1 — volume operations remain synchronous.
Only Nova and Neutron operations that execute on remote agents use the task queue.

```
POST /v2/{project}/servers
-> 202 Accepted
  Headers: X-O3K-Task-ID: <task_uuid>   (debug convenience — not in OpenStack spec)
  {
    "server": {
      "id": "uuid",
      "status": "BUILD",
      "OS-EXT-STS:task_state": "scheduling"
    }
  }

GET /v2/{project}/servers/{id}
-> 200 OK  { "status": "ACTIVE" }    <- poll until this
```

**`X-O3K-Task-ID` response header**: The 202 response includes this non-standard
header for operator debugging (e.g., `o3k task list`, log correlation). It is never
required by API consumers and does not affect Terraform compatibility. Gophercloud
ignores unknown headers.

**Note**: `adminPass` is returned only in the 202 response to `POST /servers`. It
is not persisted and will not appear in subsequent GET responses. This matches
OpenStack Nova behavior.

**DELETE /servers with in-flight task**: If `DELETE /servers/{id}` arrives while a
task for that instance is in `dispatched` state, the handler must check for an
active task:

```sql
SELECT id FROM tasks WHERE resource_id = $instance_id AND status = 'dispatched';
```

If an active task is found, return **409 Conflict** with body:
`{"error": "instance has an in-flight task — retry after task completes or fails"}`.
The client must poll `GET /servers/{id}` and retry the DELETE once the instance
leaves `BUILD` state. Do not cancel the in-flight task silently — the VM may have
already been created on the agent and would become a zombie.

Nova handlers extract `X-Idempotency-Key` from the request header and pass it to
the task insert. If absent, `idempotency_key` is NULL (PostgreSQL allows multiple
NULLs in a UNIQUE column). Duplicate `X-Idempotency-Key` returns 202 with the
existing task's resource_id (not 409, not a new task).

**Task lifecycle in DB**:

```
pending -> dispatched -> completed
               |
               +-> pending (retries < 3, with next_retry_at delay, agent_id cleared)
               +-> failed  (retries exhausted, terminal)

pending -> failed  (max_pending_age exceeded with no eligible agent)
```

**max_pending_age**: Configurable (default `30m`). A background scan checks for
tasks in `pending` state where `now() > created_at + max_pending_age`. These tasks
are set to `failed` with `error = 'no eligible agent found within max_pending_age'`.
The associated instance is set to `ERROR` status. This prevents silent indefinite
hangs for API consumers. If the scan query fails, the failure increments the same
`consecutive_failures` counter used by the main worker loop — `/healthz` returns 503
after 5 combined consecutive failures from either source.

```yaml
task_timeouts:
  max_pending_age: 30m   # auto-fail if no agent picks up within this window
```

**Instance ERROR state**: When a task transitions to `failed` (either via retry
exhaustion or `max_pending_age`), the associated instance must be set to `ERROR`
in the same transaction:

```sql
-- In Tx2 on final failure, or in max_pending_age scan:
-- Use RETURNING id so concurrent scanner runs are idempotent:
-- only the scan that wins the UPDATE proceeds with the instance update and audit log.
UPDATE tasks SET status='failed', error=$err, error_history=..., completed_at=now()
WHERE id=$task_id AND status='pending'
RETURNING id;
-- Only if RETURNING returns a row:
UPDATE instances SET status='ERROR', task_state=NULL WHERE id=$resource_id;
```

This ensures `GET /servers/{id}` returns `{ "status": "ERROR" }` rather than
staying in `BUILD` indefinitely.

**inflight semaphore on reconnect**: When an agent reconnects, the server checks the
`tasks` table for any tasks in `dispatched` state with `agent_id = $node_id` to
initialize the inflight semaphore for that agent. This check must use
`SELECT FOR UPDATE` to prevent a race where the reconciler is simultaneously
transitioning those tasks back to `pending`:

```sql
-- In TunnelHub.Register(), before initializing semaphore:
BEGIN;
SELECT id FROM tasks
WHERE agent_id = $node_id AND status = 'dispatched'
FOR UPDATE;
-- Count of returned rows = initial semaphore value
COMMIT;
```

If the reconciler acquires the lock first and requeues the task before Register
reads it, the count is 0 — correct. If Register locks first, the reconciler waits
until Register completes and then sees `status = 'pending'` — correct.
This prevents the worker from dispatching a second task to an agent that is
still executing a task from before the reconnect.

New `tasks` table:

```sql
CREATE TABLE tasks (
  id              UUID PRIMARY KEY,
  type            TEXT NOT NULL,
  resource_id     UUID NOT NULL,
  project_id      UUID NOT NULL,
  agent_id        UUID REFERENCES compute_nodes(id) ON DELETE SET NULL,
  payload         JSONB NOT NULL,
  status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'dispatched', 'completed', 'failed')),
  retries         INT NOT NULL DEFAULT 0 CHECK (retries <= 3),
  timeout_sec     INT NOT NULL CHECK (timeout_sec > 0),
  req_vcpu        INT NOT NULL DEFAULT 0,
  req_ram_mb      BIGINT NOT NULL DEFAULT 0,
  req_disk_gb     BIGINT NOT NULL DEFAULT 0,
  next_retry_at   TIMESTAMPTZ,
  idempotency_key TEXT,
  error           TEXT,
  error_history   JSONB NOT NULL DEFAULT '[]',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  dispatched_at   TIMESTAMPTZ,
  completed_at    TIMESTAMPTZ,

  CONSTRAINT chk_agent_only_when_dispatched
    CHECK (agent_id IS NULL OR status IN ('dispatched', 'completed', 'failed')),
  CONSTRAINT chk_dispatched_has_timestamp
    CHECK (status != 'dispatched' OR dispatched_at IS NOT NULL),
  CONSTRAINT chk_completed_has_timestamp
    CHECK (status != 'completed' OR completed_at IS NOT NULL),
  CONSTRAINT uq_idempotency_per_project
    UNIQUE (project_id, idempotency_key)
);

CREATE INDEX idx_tasks_pending_retry
  ON tasks (next_retry_at) WHERE status = 'pending';

CREATE INDEX idx_tasks_dispatched_timeout
  ON tasks (dispatched_at) WHERE status = 'dispatched';

-- For GET /servers/{id} → task status join
CREATE INDEX idx_tasks_resource_id ON tasks (resource_id);
```

**Background worker** (runs on every server node):

**Worker concurrency**: Each server node runs `N` worker goroutines (default: one per
connected agent, capped at `server.max_workers`, default `10`). Each goroutine
independently polls for tasks using the Tx1/Tx2 loop below — `FOR UPDATE SKIP LOCKED`
ensures no two workers claim the same task. Under high load, multiple goroutines
drain the pending queue in parallel.

```yaml
server:
  max_workers: 10   # default — one goroutine per connected agent, capped at this value
```

At v1 launch with 3-10 agents and async_compute enabled, the default of 10 workers is
sufficient. Do not set this below the number of agents — it creates an artificial
bottleneck where agents sit idle while the queue builds.

The worker uses two separate transactions — never holds a DB transaction across
network I/O:

```
loop (woken by pg_notify('new_task') or 500ms fallback poll):

  -- Tx1: claim one task + reserve one agent (single transaction, two SKIP LOCKED SELECTs)
  BEGIN;
  task = SELECT id, type, payload, timeout_sec, retries, resource_id
         FROM tasks
         WHERE status='pending'
           AND (next_retry_at IS NULL OR next_retry_at <= now())
         FOR UPDATE SKIP LOCKED
         LIMIT 1;

  if no task: COMMIT; sleep 500ms; continue

  agent = SELECT id, reserved_vcpu, total_vcpu, reserved_ram_mb, total_ram_mb,
                 reserved_disk_gb, total_disk_gb, agent_stream_server_id
          FROM compute_nodes
          WHERE status = 'active'
            AND stats_updated_at > now() - interval '30 seconds'
            AND (total_vcpu - reserved_vcpu) >= $req_vcpu
            AND (total_ram_mb - reserved_ram_mb) >= $req_ram_mb
            AND (total_disk_gb - reserved_disk_gb) >= $req_disk_gb
          ORDER BY (total_vcpu - reserved_vcpu) DESC
          FOR UPDATE SKIP LOCKED
          LIMIT 1;

  if no agent:
    ROLLBACK;  -- task stays pending, no reservation made
    continue

  UPDATE tasks SET status='dispatched', agent_id=$agent.id, dispatched_at=now()
    WHERE id = $task.id;
  UPDATE compute_nodes
    SET reserved_vcpu    = reserved_vcpu    + $req_vcpu,
        reserved_ram_mb  = reserved_ram_mb  + $req_ram_mb,
        reserved_disk_gb = reserved_disk_gb + $req_disk_gb
    WHERE id = $agent.id;
  COMMIT;

  -- Dispatch check (outside any transaction)
  if agent.agent_stream_server_id != $this_server_id:
    -- Release reservation — two separate UPDATEs, no transaction wrapper.
    -- Both use GREATEST(0, ...) safety floor. If the second UPDATE (task reset)
    -- fails, the task stays 'dispatched' with decremented reservation. The
    -- reconciler detects and requeues after 2 * timeout_sec — no permanent leak.
    db.Exec(`UPDATE compute_nodes SET
               reserved_vcpu    = GREATEST(0, reserved_vcpu    - $1),
               reserved_ram_mb  = GREATEST(0, reserved_ram_mb  - $2),
               reserved_disk_gb = GREATEST(0, reserved_disk_gb - $3)
             WHERE id = $4`, reqVcpu, reqRam, reqDisk, agentID)
    db.Exec(`UPDATE tasks SET status='pending', agent_id=NULL, dispatched_at=NULL
             WHERE id = $1`, task.id)
    continue

  -- Local stream found: dispatch over gRPC
  -- context.WithTimeout MUST wrap both the stream Send and the Recv for the result.
  -- If task.timeout_sec is 0 (violates CHECK constraint — should never happen),
  -- treat as ERROR_PERMANENT rather than creating a zero-duration context.
  dispatchCtx, cancel = context.WithTimeout(ctx, time.Duration(task.timeout_sec) * time.Second)
  defer cancel()
  result = TunnelHub.Dispatch(dispatchCtx, agent.id, task)

  -- Tx2: record result (atomic: task + resource + reservation)
  BEGIN;
  if error:
    if result.code == ERROR_PERMANENT or task.retries >= 2:
      UPDATE tasks SET status='failed', error=$err,
        error_history = error_history || $entry, retries=retries+1,
        completed_at=now();
      UPDATE instances SET status='ERROR', task_state=NULL WHERE id=$resource_id;
    else:
      UPDATE tasks SET status='pending', agent_id=NULL,
        next_retry_at=now()+backoff, error=$err,
        error_history = error_history || $entry, retries=retries+1;

    UPDATE compute_nodes SET
      reserved_vcpu    = GREATEST(0, reserved_vcpu    - $req_vcpu),
      reserved_ram_mb  = GREATEST(0, reserved_ram_mb  - $req_ram_mb),
      reserved_disk_gb = GREATEST(0, reserved_disk_gb - $req_disk_gb)
    WHERE id = $agent_id;
  else:
    UPDATE tasks SET status='completed', completed_at=now();
    -- If instance was deleted mid-flight (user called DELETE /servers while dispatched),
    -- this UPDATE returns 0 rows. Mark task completed anyway (work was done on agent),
    -- then dispatch a VM_DELETE cleanup task to the agent to remove the libvirt domain.
    rows_updated = UPDATE instances SET status='ACTIVE', task_state=NULL WHERE id=$resource_id;
    if rows_updated == 0:
      log WARN 'instance deleted mid-flight', task_id=$task_id, resource_id=$resource_id
      -- queue VM_DELETE cleanup task for the agent (outside this Tx2 transaction)
      -- Note: VM_DELETE dispatch happens after Tx2 COMMIT to avoid nested transactions.
    UPDATE compute_nodes SET
      reserved_vcpu    = GREATEST(0, reserved_vcpu    - $req_vcpu),
      reserved_ram_mb  = GREATEST(0, reserved_ram_mb  - $req_ram_mb),
      reserved_disk_gb = GREATEST(0, reserved_disk_gb - $req_disk_gb)
    WHERE id = $agent_id;
  COMMIT;

  -- DB error handling:
  if any SELECT or COMMIT fails: log ERROR, increment consecutive_failures counter,
    expose /healthz as unhealthy after 5 consecutive failures,
    backoff before next poll.
  -- IMPORTANT: On Tx1 COMMIT failure, do NOT retry Tx1 inline. The COMMIT may
  -- have landed on the DB side despite the connection error. Retrying Tx1 risks
  -- double-reservation. Log CRITICAL and let the reconciler detect the dispatched
  -- task after 2 * timeout_sec and requeue it. This is the safe path.
  -- Note: if Tx2 DB write fails, the task stays 'dispatched'. The reconciler
  -- detects this after 2 * timeout_sec and requeues. This is acceptable.
```

**Resource requirement extraction**: Before Tx1, the worker decodes `$req_vcpu`,
`$req_ram_mb`, and `$req_disk_gb` from the task row:

```go
// ResourceRequirements is embedded in the tasks table as a separate column trio,
// not decoded from payload at schedule time:
//   req_vcpu     INT NOT NULL DEFAULT 0
//   req_ram_mb   BIGINT NOT NULL DEFAULT 0
//   req_disk_gb  BIGINT NOT NULL DEFAULT 0
// Nova/Neutron handlers write these columns when inserting the task row,
// derived from the flavor (VM tasks) or set to 0 (NET_*, VXLAN_*, IMAGE_PREFETCH).
// Worker reads them directly — no payload decode in the scheduler hot path.
```

Add these columns to the `tasks` DDL:

```sql
ALTER TABLE tasks ADD COLUMN req_vcpu     INT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN req_ram_mb   BIGINT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN req_disk_gb  BIGINT NOT NULL DEFAULT 0;
```

Non-compute tasks (NET_*, VXLAN_*, IMAGE_PREFETCH) have all three as 0 — they always
satisfy the capacity filter. VM tasks (VM_CREATE) copy from the flavor at insert time.

Update the Tx1 SELECT to read these:
```sql
task = SELECT id, type, payload, timeout_sec, retries, resource_id,
              req_vcpu, req_ram_mb, req_disk_gb
       FROM tasks
       WHERE status='pending'
         AND (next_retry_at IS NULL OR next_retry_at <= now())
       FOR UPDATE SKIP LOCKED
       LIMIT 1;
```

**Implementation note**: Both Tx1 and Tx2 must use the existing `database.WithTx`
helper (`internal/database/`). Do not open raw pgx transactions.

**Atomic instance + task insert**: Nova handler must insert the `instances` row and
the `tasks` row in the same transaction. `pg_notify('new_task', task_id)` is called
after COMMIT — never inside the transaction. If the transaction rolls back after the
instance row is inserted but before the task row is committed, there will be an orphan
instance with no task. This must not happen.

**Instance initial status at insert**: The `instances` row must be inserted with
`vm_state = 'building'`, `task_state = 'scheduling'`, `power_state = 0` (no state).
During `IMAGE_PREFETCH`, the worker updates `task_state = 'image_pending_upload'`.
During `VM_CREATE`, the worker updates `task_state = 'spawning'`. On successful
`VM_CREATE` completion (Tx2), the worker sets `vm_state = 'active'`, `task_state = NULL`,
`power_state = 1` (running). These values are what Horizon, the OpenStack CLI, and
gophercloud display to the operator — they must be populated at insert time, not left
NULL until completion.

```go
// Required pattern in Nova handler:
err = database.WithTx(ctx, func(tx pgx.Tx) error {
    if err := insertInstance(tx, instance); err != nil {
        return err
    }
    return insertTask(tx, task)
})
if err != nil {
    return err
}
// Only notify AFTER successful commit:
db.Exec(ctx, "SELECT pg_notify('new_task', $1)", taskID)
```

**Immediate task wakeup**: A single shared listener goroutine calls pgx
`WaitForNotification` on a dedicated connection (not from the pool). On receipt,
it broadcasts to all N worker goroutines via a `chan struct{}` (buffered, size N).
Workers select on both this channel and a 500ms ticker — the ticker is a reliability
backstop only. Do NOT give each worker goroutine its own LISTEN connection — that
creates N separate PostgreSQL connections and is incompatible with pgx connection
pooling semantics.

`FOR UPDATE SKIP LOCKED` on the tasks table ensures two server nodes never process
the same task. For agent-side idempotency, agents check `task_id` in local state
before executing — preventing double execution during server failover.

---

### 5. Scheduler — Atomic Resource Reservation

Stats from agents update the `compute_nodes` table (total capacity only — `total_*`
columns). `reserved_*` columns are managed exclusively by the scheduler transaction.
Free capacity is always computed as `total - reserved`, never stored directly.

```sql
ALTER TABLE compute_nodes ADD COLUMN total_vcpu      INT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN total_ram_mb    BIGINT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN total_disk_gb   BIGINT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN reserved_vcpu   INT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN reserved_ram_mb BIGINT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN reserved_disk_gb BIGINT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN stats_updated_at TIMESTAMPTZ;
ALTER TABLE compute_nodes ADD COLUMN agent_stream_server_id TEXT;

ALTER TABLE compute_nodes ADD CONSTRAINT chk_reservation_within_capacity
  CHECK (reserved_vcpu >= 0 AND reserved_vcpu <= total_vcpu
     AND reserved_ram_mb >= 0 AND reserved_ram_mb <= total_ram_mb
     AND reserved_disk_gb >= 0 AND reserved_disk_gb <= total_disk_gb);

CREATE INDEX idx_compute_nodes_scheduling
  ON compute_nodes (total_vcpu, total_ram_mb)
  WHERE status = 'active';
-- Scheduler selects from all active agents globally (DB-as-coordinator model).
-- No per-server-id index needed.
-- Note: ORDER BY (total_vcpu - reserved_vcpu) DESC is a computed expression —
-- PostgreSQL cannot use this index for ordering, only for filtering by status.
-- At v1 scale (3-10 agents) a sequential scan of the active set is fast.
-- For larger clusters, add a functional index: CREATE INDEX ON compute_nodes
-- ((total_vcpu - reserved_vcpu)) WHERE status = 'active';

-- Note: the chk_reservation_within_capacity constraint means any UPDATE that sets
-- reserved_vcpu > 0 while total_vcpu = 0 will fail. After migration, existing nodes
-- have total_vcpu = 0 and cannot accept reservations until they reconnect and report
-- stats via StatsStream. This is expected behavior during rolling upgrades.
```

**Note**: After migration, existing nodes have `total_vcpu = 0` and are invisible to
the scheduler until they reconnect and report stats via StatsStream. This is expected
behavior during rolling upgrades.

Scheduling transaction uses `FOR UPDATE SKIP LOCKED` on `compute_nodes` to avoid
blocking concurrent schedulers. If the best-fit node is locked by another scheduler,
the query skips it and selects the next-best node rather than blocking.

**DB-as-coordination-layer**: The scheduler selects the best-fit agent from **all**
active agents globally — no per-server filter. After reserving the agent in DB, the
worker checks whether it holds the agent's gRPC stream locally. If yes, it dispatches
immediately. If no (agent is connected to a different server node), the worker releases
the reservation and returns the task to `pending` — the owning server's worker claims
it on its next poll cycle (within 500ms). No cross-server RPC is needed. PostgreSQL
is the sole coordinator.

The full Tx1 (task claim + agent reservation in one transaction) and dispatch check
are specified in the worker loop in Section 4. Section 5 documents the resource
reservation model and correctness properties only.

**`agent_stream_server_id`**: Each server node writes this column when an agent's
gRPC connection is established, and clears it on disconnect. It is used only to
determine dispatch ownership — never to filter the scheduler SELECT.

**Server node identity (`$this_server_id`)**: Each server node generates a random UUID
on startup (not persisted — ephemeral per process). This UUID is written to
`agent_stream_server_id` when an agent connects. The dispatch check compares
`agent.agent_stream_server_id` to the current worker's in-memory UUID.
The column type is TEXT (UUID string, e.g., `"8f3c1a72-..."`) — consistent with
existing UUID columns in the schema that use TEXT rather than UUID type.

```go
// In server startup:
var thisServerID = uuid.NewString()  // ephemeral, in-memory only
```

**Reservation lifecycle**: The reservation decrement is part of the same DB
transaction as task completion (Tx2 in Section 4). It is never a separate call.

**Reconciliation goroutine**: Scans tasks in `dispatched` state where
`now() > dispatched_at + 2 * timeout_sec * interval '1 second'`. Uses
`SELECT FOR UPDATE` on the task row, re-checks status after acquiring lock,
then releases reservation with `GREATEST(0, reserved - req)` as a safety floor.
Adds a `reconciled_at` column to prevent double-reconciliation.

**Reconciler requeue vs. fail decision**: Before requeueing, the reconciler MUST
check `retries` against `max_retries` (3). If `retries >= max_retries`, mark the task
`failed` directly — do NOT set `status='pending', retries=retries+1`, which would
violate the `CHECK (retries <= 3)` DB constraint and leave the task stuck in
`dispatched` forever. Pseudocode:
```
if task.retries >= max_retries:
  UPDATE tasks SET status='failed', error='reconciler: max retries exceeded',
    completed_at=now()
  WHERE id=$task_id AND status='dispatched';
  UPDATE instances SET status='ERROR' WHERE id=$resource_id;
else:
  UPDATE tasks SET status='pending', agent_id=NULL, dispatched_at=NULL,
    next_retry_at=now()+backoff, retries=retries+1, error_history=...
  WHERE id=$task_id AND status='dispatched';
UPDATE compute_nodes SET reserved_vcpu=GREATEST(0,reserved_vcpu-$req), ...
  WHERE id=$agent_id;
```

**Reconciler run interval**: Default `30s`, configurable via
`task_timeouts.reconciler_interval`. Shorter intervals increase DB pressure with
no correctness benefit (the condition is `2 * timeout_sec`, not the scan
frequency). Add random jitter of ±10% per server node to prevent synchronized
thundering-herd scans from a multi-node cluster.

```yaml
task_timeouts:
  reconciler_interval: 30s  # default — jitter of ±10% applied automatically
```

**Reconciler requeue — clearing the server binding**: When the reconciler requeues a
task, it must clear both `agent_id` and `agent_stream_server_id` on the task row:

```sql
UPDATE tasks
SET status='pending', agent_id=NULL, dispatched_at=NULL, next_retry_at=now()+backoff,
    error_history = error_history || $reconcile_entry, retries=retries+1
WHERE id=$task_id AND status='dispatched';

UPDATE compute_nodes
SET reserved_vcpu    = GREATEST(0, reserved_vcpu    - $req_vcpu),
    reserved_ram_mb  = GREATEST(0, reserved_ram_mb  - $req_ram_mb),
    reserved_disk_gb = GREATEST(0, reserved_disk_gb - $req_disk_gb)
WHERE id=$agent_id;
```

The reconciler scans tasks by `dispatched_at` timeout regardless of which server
originally dispatched them. Once the task is back in `pending`, any server's worker
can claim it through the normal scheduling flow (DB-as-coordinator — no server binding
on requeued tasks).

**Race — reconciler vs reconnect result delivery**: If an agent reconnects and sends
a `TaskResult` at the same moment the reconciler fires, both paths do
`UPDATE tasks WHERE id=$task_id AND status='dispatched'`. One will get 0 rows
returned and back off cleanly. The `RETURNING id` guard makes this idempotent.

**Image-aware placement**: Scheduler prefers agents that already have the requested
image cached (from `cached_images` in AgentStats) when resources are otherwise equal.

---

### 6. Image Pre-Fetch — Decoupled from VM Create

Image pull is separated from VM creation. Two phases:

**Phase 1 — `IMAGE_PREFETCH` task** (timeout from config `image_prefetch_timeout`,
retryable):
- Dispatched by Nova when `POST /servers` is received, before `VM_CREATE`
- Agent checks local image cache (keyed by `image_id` + `checksum`)
- Cache hit is valid only if the local file checksum matches the current Glance
  image checksum. Stale cache entries (checksum mismatch) are evicted and re-pulled.
- Image download URL is a signed, time-limited token generated server-side —
  the raw Glance backend URL is never passed in the task payload (prevents SSRF).
  **Signed URL TTL MUST be at least `image_prefetch_timeout` + 60 seconds** (default:
  5m + 60s = 6 minutes). If the signed URL expires before the download completes,
  the agent receives an HTTP 4xx mid-stream and the task fails with `ERROR_TRANSIENT`.
  Server-side URL generation must enforce this minimum TTL at generation time.
  **Signed URL scope**: Signed URLs point to the configured storage backend only —
  either an S3 presigned URL (agent downloads direct from S3) or an o3k server proxy
  endpoint (`GET /v1/images/{id}/data?sig=...`). The URL scheme and host are validated
  server-side against configured backend endpoints before being included in the task
  payload. This prevents SSRF via malformed storage backend configuration.
- **Partial download cleanup**: Download is written to a `.tmp` file first
  (`{image_id}.tmp`). On disk-full or any I/O error, the `.tmp` file is deleted
  before the task fails. The permanent `image_id` file is written atomically via
  `os.Rename` only after the checksum is verified. This ensures a failed download
  never leaves a partial file that blocks future retries.
- On success: task completes. The `VM_CREATE` task insertion is part of the same
  transaction as the `IMAGE_PREFETCH` completion write (atomic).

**Phase 2 — `VM_CREATE` task** (timeout: 30s):
- Image is guaranteed local — no image download I/O
- `vm.create` payload includes `image_local_path` populated from the image cache
  entry recorded during prefetch completion.
- libvirt domain create only: fast, bounded

**Image prefetch timeout**: 5 minutes is an intentional exception to the fail-fast
rule. Image downloads are long-running data transfers that cannot complete in <1s.
The timeout is bounded and retryable, consistent with how container runtimes
(containerd, CRI-O) handle image pulls separately from container creation.

**Concurrent prefetch serialization**: If two `IMAGE_PREFETCH` tasks for the same
`image_id` are dispatched to the same agent simultaneously, the agent must serialize
them per-image using a per-image mutex (keyed by `image_id`). The second task acquires
the mutex, finds the image already in cache (downloaded by the first), skips the
download, and completes immediately. Without this serialization, both tasks write to
`{image_id}.tmp` simultaneously and corrupt the download. The mutex must be in-memory
only — not in SQLite — to avoid deadlocks between goroutines and SQLite WAL mode.

---

### 7. Agent Local State

Agent maintains state at `/var/lib/o3k/agent/state.db` using `modernc.org/sqlite`
(pure Go, CGO-free, required for static single-binary builds — do not use
`mattn/go-sqlite3`):

```sql
CREATE TABLE current_task (
  singleton   INTEGER PRIMARY KEY DEFAULT 1 CHECK (singleton = 1),
  task_id     TEXT NOT NULL,
  type        TEXT NOT NULL,
  payload     TEXT NOT NULL,
  status      TEXT NOT NULL CHECK (status IN ('executing', 'completed', 'failed')),
  result      TEXT,
  error       TEXT,
  started_at  INTEGER NOT NULL  -- Unix epoch seconds, UTC
);

CREATE TABLE image_cache (
  image_id    TEXT PRIMARY KEY,
  local_path  TEXT NOT NULL,
  checksum    TEXT NOT NULL,     -- md5 from Glance metadata
  size_bytes  INTEGER NOT NULL,
  cached_at   INTEGER NOT NULL,  -- Unix epoch seconds
  verified_at INTEGER,           -- NULL = never re-verified after caching
  last_used_at INTEGER           -- updated on every cache hit (for LRU eviction)
);
```

**Image cache eviction**: The agent enforces a configurable max cache size
(`agent.image_cache_max_gb`, default `50`). Before each `IMAGE_PREFETCH`, the
agent checks whether the new image would exceed the limit. If so, it evicts the
least-recently-used cached images (by `last_used_at`) until space is available.
Eviction deletes the local file and removes the `image_cache` row atomically.
If eviction cannot free enough space (e.g., all cached images are in use), the
task fails with `ERROR_TRANSIENT`.

```yaml
agent:
  image_cache_max_gb: 50  # default — LRU eviction when exceeded
```

The `singleton = 1` constraint enforces at most one row in `current_task`. Any
attempt to insert a second row fails immediately. All writes to `current_task`
must hold a mutex.

**Agent local result storage (HMAC re-sign on reconnect)**: The `current_task.result`
column stores the raw result bytes (not a pre-computed HMAC). On reconnect to any
server, the agent receives a new `server_nonce` in `HelloAck` and re-signs the result
at that point: `result_mac = HMAC(new_nonce || task_id || result_bytes, agent_key)`.
This makes server MAC verification identical for both live-session results and
reconnect results — the server always verifies against the nonce it issued.

**OrphanReport HMAC**: Results delivered via `OrphanReport` in the `Hello` message
are verified with the same HMAC scheme as `TaskResult`, using the `server_nonce` from
the `HelloAck` issued during this registration. The agent re-signs OrphanReport results
using the new nonce before including them in `Hello`.

**Reconnect recovery**:
1. Agent dials new server (via VIP).
2. Calls `Register(Hello)` with `node_id`, `hostname`, `cached_images`, and any
   `OrphanReport` entries from local state (HMAC-signed with the new nonce from HelloAck).
3. Agent queries `current_task` for status `completed` or `failed`.
4. If found: re-signs result with new `server_nonce`, sends `TaskResult` (with HMAC) to new server.
5. Server validates HMAC, then checks task ownership:
   ```sql
   UPDATE tasks SET status='completed', completed_at=now(), agent_id=$agent
   WHERE id=$task_id AND status='dispatched' AND agent_id=$original_agent
   RETURNING id;
   ```
   If the task was already completed by a different agent (retried during
   disconnect), the update returns 0 rows. Server sends a `VM_DELETE` cleanup
   task to the reconnecting agent for the orphaned domain.
6. If server doesn't know the task (server crashed before writing to DB): agent
   sends an `OrphanReport` in the `Hello` message. Server creates the missing
   task row as `completed`, updates the resource row, and releases reservation.
   If `OrphanReport` contains a `VM_CREATE` result, server reconciles the instance.
7. Agent clears local state after server acknowledges.

**Agent SQLite failure on startup**: If `state.db` cannot be opened or read on agent
startup (corruption, permissions error, filesystem failure), the agent must:
1. Log CRITICAL with the error.
2. Attempt SQLite WAL recovery (`PRAGMA wal_checkpoint`).
3. If unrecoverable: reconnect to server with a synthetic `OrphanReport` containing
   `error = 'RECOVERY_FAILED: local state unavailable'` for any task that may have
   been in flight. This signals the server to requeue the task and alerts the operator.
4. Do NOT silently skip the OrphanReport — a libvirt domain may be running with no
   task tracking it. The `RECOVERY_FAILED` error lets the server requeue and lets
   the operator know to manually inspect the agent's libvirt domains.

---

### 8. HA — Multiple Servers

Three or more server nodes behind a load balancer (or keepalived VIP). PostgreSQL
is the shared state store — no etcd required (already in use).

```
          +----------------------------------------------+
          |  Load Balancer / VIP                         |
          |  :6385  (agent gRPC tunnel, internal only)   |
          |  :35357 :8774 :9696 :8776 :9292 :8778 (API) |
          +------------+-----------------+---------------+
                       |                 |
               +-------v--+       +------v----+
               | Server 1 |       | Server 2  |   active/active
               | TunnelHub|       | TunnelHub |   shared PostgreSQL
               | Worker   |       | Worker    |   FOR UPDATE SKIP LOCKED
               +----------+       +-----------+
                       |                 |
               +-------v-----------------v------+
               |        PostgreSQL               |
               |  tasks, compute_nodes, ...      |
               +--------------------------------+
                       |
           +-----------+-----------+
       +---v---+   +---v---+   +---v---+
       |Agent 1|   |Agent 2|   |Agent 3|
       +-------+   +-------+   +-------+
```

**Port 6385 must be on a separate, internal-only listener** — not exposed to the
public network. Agent join/tunnel traffic is internal infrastructure, not
user-facing API.

Agent connections are sticky to one server until that server dies. On disconnect,
agent reconnects to VIP (any healthy server). No state loss because all task state
lives in PostgreSQL and agent local SQLite.

**Agent reconnect backoff**: Agent reconnect uses exponential backoff (1s, 2s, 4s, 8s,
cap 30s) with ±20% jitter per attempt. Each attempt goes to the VIP — the load balancer
directs it to a healthy server. The agent does not track which server it last connected
to (VIP is the single endpoint). If a reconnect attempt succeeds but the new connection
immediately fails (e.g., LB slow to drain a dying server), the agent counts it as a
failed attempt and applies the next backoff level.

Workers schedule from all active agents globally (DB-as-coordinator — see Section 5).
Each worker dispatches only to agents whose gRPC stream it holds locally. Tasks
reserved for a remotely-connected agent are released immediately and reclaimed by the
correct server node within 500ms.

---

### 9. Failure Mode Table

| Failure | Detection | Outcome |
|---------|-----------|---------|
| Agent HeartbeatStream drops | 5s timeout, stream EOF, or TCP close | Agent marked `offline`, new dispatch suppressed. In-flight tasks accepted until `task.timeout` expires, then retried. |
| Server crashes mid-dispatch | Task stays `dispatched` in DB | Reconciler detects after 2x `timeout_sec`, clears `agent_id` and `dispatched_at`, releases reservation, requeues to any eligible agent |
| Agent crashes mid-execution | Agent local SQLite records state | On restart/reconnect: sends result or OrphanReport. Server reconciles. |
| Double-booking race | `SELECT FOR UPDATE SKIP LOCKED` on compute_nodes | One scheduler wins, other selects next-best node |
| Image pull timeout | `IMAGE_PREFETCH` retried up to 3x with backoff | `VM_CREATE` queued only after successful prefetch (atomic) |
| All agents offline | Scheduler finds no eligible agent | Task stays `pending`. `max_pending_age` auto-fails after 30m (configurable). Instance set to `ERROR`. |
| Worker DB unavailable | Consecutive query failures | /healthz returns 503 after 5 failures, LB stops routing |
| Agent reconnect with stale result | Task already completed by another agent | Server rejects stale result (UPDATE returns 0 rows), sends cleanup task |
| Image deleted between prefetch and vm.create | Image row missing at vm.create dispatch | Instance set to ERROR, task set to failed immediately (no retry) |
| Reconciler vs reconnect race | Both attempt `UPDATE WHERE status='dispatched'` | `RETURNING id` guard: one wins, other gets 0 rows and backs off cleanly. Idempotent. |
| OrphanReport with fabricated task_id | `task_id` not found in `tasks` table | Rejected with structured error log. No instance row created. |
| Server node missing CA key for CertRenew | CA key not at `/var/lib/o3k/server/ca.key` | Returns gRPC `FAILED_PRECONDITION`. Agent retries via VIP to a node with CA key. |
| Worker goroutine parks on blocked stream Send | context.WithTimeout on Send and Recv | Timeout after task.timeout_sec, enter Tx2 error path, task retried with backoff |
| Reconciler tries to requeue at max retries | Check retries < max_retries before requeue | Mark task failed directly; no `pending` requeue that would violate CHECK constraint |
| max_pending_age scanner fires on two server nodes simultaneously | `RETURNING id` guard on UPDATE | Only one scan wins; second gets 0 rows, skips instance update and audit log |
| Duplicate agent registration (same node_id, live stream) | Register handler detects existing stream | Close old stream, replace atomically, log `agent.reconnect` audit event |
| Agent SQLite unreadable on startup | CRITICAL log + WAL recovery attempt | Reconnect with synthetic OrphanReport containing RECOVERY_FAILED error; operator alerted |
| Concurrent IMAGE_PREFETCH for same image on same agent | Per-image in-memory mutex in agent | Second task waits for in-progress download, uses cache on completion; no duplicate download |
| Signed URL TTL expires mid-download | HTTP 4xx from storage backend | Fail ERROR_TRANSIENT; server must generate URL TTL ≥ image_prefetch_timeout + 60s |
| Instance deleted while VM_CREATE is dispatched | Tx2 instance UPDATE returns 0 rows | Mark task completed, dispatch VM_DELETE cleanup to agent for orphaned libvirt domain |

**Post-deploy verification checklist**:
- [ ] `o3k node list` shows all expected agents with `status=active`
- [ ] `o3k task list --status=pending` shows queue depth is zero (or expected value)
- [ ] `/metrics` endpoint responds on each server node
- [ ] `o3k_cert_expiry_days` gauge is > 30 for all agents
- [ ] One test task dispatched end-to-end: `openstack server create ... && openstack server show ...` returns `ACTIVE`
- [ ] HA failover tested: kill Server 1, verify agent reconnects to Server 2, pending tasks complete

**Note**: This checklist is manual-only in v1.1.0. A follow-on spec will define
`o3k cluster-health` — a single command that runs all checklist items and exits
non-zero if any check fails, suitable for use in deployment pipelines.

---

### 10. Observability

**Structured audit log**: Separate from application debug logs. Append-only
`audit_events` table in PostgreSQL:

| Event | Fields |
|-------|--------|
| `agent.join` | node_id, source_ip, cert_serial, timestamp |
| `agent.leave` | node_id, reason (disconnect/deregister), timestamp |
| `agent.cert_issued` | node_id, cert_serial, expiry, timestamp |
| `agent.cert_revoked` | node_id, cert_serial, revoked_by, timestamp |
| `task.dispatched` | task_id, node_id, type, timestamp |
| `task.completed` | task_id, node_id, duration_ms, timestamp |
| `task.failed` | task_id, node_id, error, retry_count, timestamp |
| `reconciler.fired` | task_id, old_agent, action, timestamp |

**Table retention**: `audit_events` and `tasks` (completed/failed rows) grow
unboundedly. Archival strategy is a follow-on spec, but implementors must:
- Partition `audit_events` by month from the start (cheaper than retrofitting)
- Provide a `o3k task prune --older-than 90d` command for operator-initiated cleanup
- Document this in the deployment guide before v1.0 release

**`tasks` retention policy**: Completed and failed tasks older than 30 days should
be pruned or archived. The `o3k task prune --older-than 90d` command must implement:

```sql
-- Delete terminal tasks older than the configured threshold:
DELETE FROM tasks
WHERE status IN ('completed', 'failed')
  AND completed_at < now() - $threshold;
```

Default threshold: `90d` (configurable via `task_timeouts.prune_after`). Pending
and dispatched tasks are never pruned. The prune command must emit a count of
deleted rows and run in batches of 1000 to avoid long-running transactions.

```yaml
task_timeouts:
  prune_after: 90d   # default — tasks prune window for completed/failed rows
```

**`audit_events` DDL** (required from day one — do not create as a plain table):

```sql
CREATE TABLE audit_events (
  id          UUID    NOT NULL DEFAULT gen_random_uuid(),
  event_type  TEXT    NOT NULL,
  node_id     UUID,
  task_id     UUID,
  fields      JSONB   NOT NULL DEFAULT '{}',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY RANGE (created_at);

-- Create initial partitions at migration time:
CREATE TABLE audit_events_2026_04 PARTITION OF audit_events
  FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

-- New partitions added monthly (pg_cron job or operator script):
-- CREATE TABLE audit_events_2026_05 PARTITION OF audit_events
--   FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

CREATE INDEX ON audit_events (created_at);
CREATE INDEX ON audit_events (node_id) WHERE node_id IS NOT NULL;
CREATE INDEX ON audit_events (task_id) WHERE task_id IS NOT NULL;
```

**Note**: Monthly partitions allow `DROP TABLE audit_events_YYYY_MM` for fast
bulk archival without VACUUM overhead. A cron job or `o3k audit prune` must create
the next month's partition before the month rolls over.

**Automated partition creation**: If pg_cron is not available, server startup code
must create the current-month and next-month partitions using `CREATE TABLE IF NOT
EXISTS ... PARTITION OF audit_events FOR VALUES FROM (...) TO (...)`. This is
idempotent and costs negligible startup time. Without this, an operator who forgets
to pre-create the next-month partition will see all audit writes fail at month rollover.

**Structured log events**: Every task lifecycle state transition emits a structured
log entry with fields: `task_id`, `node_id`, `task_type`, `status`, `error` (if any).

**Inspection commands**:
- `o3k node list` — shows all agents, status, connected server, resource utilization
- `o3k node status <node-id>` — detailed agent info including in-flight tasks
- `o3k task list --status=pending` — query task queue state
- `o3k node reconcile` — scan for orphaned resources across all agents

**Prometheus metrics** (exposed at `/metrics` on each server node):

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `o3k_task_pending_duration_seconds` | Histogram | `task_type` | Time tasks spend in `pending` state |
| `o3k_task_dispatch_duration_seconds` | Histogram | `task_type`, `agent_id` | Time from dispatch to completion/failure |
| `o3k_agent_reconnects_total` | Counter | `node_id` | Agent reconnect events |
| `o3k_scheduler_no_eligible_agent_total` | Counter | `task_type` | Scheduling attempts with no eligible agent |
| `o3k_worker_consecutive_db_failures` | Gauge | — | Current consecutive DB failure count |
| `o3k_cert_expiry_days` | Gauge | `node_id` | Days until agent cert expires |
| `o3k_agent_last_heartbeat_timestamp` | Gauge | `node_id` | Unix timestamp of last heartbeat received from agent (updated on each HeartbeatStream ping) |
| `o3k_reconciler_runs_total` | Counter | `action=requeued\|failed` | Reconciler actions taken (requeued vs. marked failed due to max retries) |
| `o3k_reconciler_last_run_timestamp` | Gauge | — | Unix timestamp of last reconciler scan completion |

**Alerting rules** (add to Prometheus rules or equivalent):

```yaml
- alert: TaskPendingTooLong
  expr: histogram_quantile(0.95, rate(o3k_task_pending_duration_seconds_bucket[5m])) > 300
  for: 2m
  annotations:
    summary: "Tasks pending >5 minutes at p95 — check agent availability"
    runbook: "https://docs.o3k.io/runbooks/task-pending-too-long"

- alert: AgentOffline
  expr: time() - o3k_agent_last_heartbeat_timestamp > 600
  for: 1m
  labels:
    severity: warning
  annotations:
    summary: "Agent {{ $labels.node_id }} offline for >10 minutes"
    runbook: "https://docs.o3k.io/runbooks/agent-offline"

- alert: AgentCertExpiryImminent
  expr: o3k_cert_expiry_days < 30
  for: 1h
  labels:
    severity: warning
  annotations:
    summary: "Agent {{ $labels.node_id }} cert expires in {{ $value }} days"
    runbook: "https://docs.o3k.io/runbooks/agent-cert-renewal"

- alert: ReconcilerSilent
  expr: time() - o3k_reconciler_last_run_timestamp > 120
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "Reconciler has not run in >2 minutes — stuck dispatched tasks will not be recovered"
    runbook: "https://docs.o3k.io/runbooks/reconciler-silent"
```

**Note**: Runbook URLs are placeholders. The four runbooks must be written before
v1.0 release. At minimum, each runbook must cover: symptom, likely causes, first
diagnostic commands, resolution steps, and escalation path.

---

### 11. New Files and Changes

**New packages** (all require creation — none exist yet):
- `internal/tunnel/` — TunnelHub, gRPC server, stream management
- `internal/worker/` — background task worker, retry logic
- `internal/scheduler/` — atomic reservation, placement algorithm
- `internal/agent/` — agent main loop, task executor, local state
- `proto/agent.proto` — gRPC service definition
- `proto/payloads.proto` — typed payload structs per TaskType

**Modified**:
- `cmd/o3k/main.go` — add `server` / `agent` / `token` subcommand dispatch via
  `flag.NewFlagSet` (preserves backward compat for bare `o3k` invocation)
- `internal/nova/` — return 202 on create, write task to DB, add
  `OS-EXT-STS:task_state` to response, extract `X-Idempotency-Key` header
- `internal/neutron/` — same pattern for port/network operations that touch compute
- `internal/placement/` — update resource provider inventory from `compute_nodes`
  reservation columns to keep Placement in sync with scheduler state (see
  Placement Sync section below)
- `internal/compute/node_registry.go` — remove DB-polling heartbeat loop. Liveness
  now detected via HeartbeatStream drop. `last_heartbeat` column retained but
  updated by gRPC heartbeat handler, not the old ticker goroutine.

**New DB migrations**:
- `tasks` table (with CHECK constraints, indexes, FK)
- `compute_nodes` additions (resource columns, CHECK constraints, indexes)
- `revoked_agent_certs` table (`CREATE INDEX ON revoked_agent_certs(serial_number)` —
  looked up on every agent connection)
- `audit_events` table (`CREATE INDEX ON audit_events(created_at)` —
  required for time-range audit queries)

**Unchanged**: Keystone, Cinder, Glance, Metadata, middleware, existing config
structure (new `agent` and `server` sections added to `Config`).

---

### 11b. Placement Sync

`internal/placement/placement.go` is currently a full stub. When the scheduler
feature lands, Placement must serve real inventory data so Nova's flavor scheduling
(and any external Placement consumers) reflect actual cluster capacity.

**What to read**: Each time Placement receives a GET `/resource_providers/{uuid}/inventories`
or GET `/resource_providers` request, it reads the current row from `compute_nodes`:

```go
// Read for each connected agent
SELECT id, total_vcpu, reserved_vcpu,
       total_ram_mb, reserved_ram_mb,
       total_disk_gb, reserved_disk_gb
FROM compute_nodes
WHERE status = 'active'
  AND stats_updated_at > now() - interval '30 seconds';
```

**Mapping to Placement resource classes**:

| compute_nodes column | Placement resource class |
|----------------------|--------------------------|
| `total_vcpu - reserved_vcpu` | `VCPU` (available) |
| `total_ram_mb - reserved_ram_mb` | `MEMORY_MB` (available) |
| `total_disk_gb - reserved_disk_gb` | `DISK_GB` (available) |

**One resource provider per agent node**: Each `compute_nodes.id` maps to one
Placement resource provider UUID. The UUID is stable — it is the agent's `node_id`.

**No write-back**: Placement is read-only relative to `compute_nodes`. The scheduler
owns all reservation writes. Placement never writes to `compute_nodes`.

**Staleness**: Placement reads `stats_updated_at > now() - 30s` (same threshold as
the scheduler). Stale nodes are excluded from resource provider responses and do not
appear in allocation candidates. `stats_updated_at` is written by StatsStream (every
`stats_interval`, default 10s). HeartbeatStream liveness does NOT extend this window.
Implementors must ensure `stats_interval` remains well below 30s — if `stats_interval`
is raised above 30s in config, the agent becomes invisible to both Placement and the
scheduler even while heartbeating normally. Config `Validate()` must enforce
`stats_interval < 20s` (two-thirds of the staleness window as a safety margin).

**Caching**: Placement must cache the `compute_nodes` read for up to 5 seconds to
reduce DB load on Nova scheduling calls. Without caching, every `POST /servers`
triggers a Placement DB read — at 50 concurrent requests this is 50 queries/request
cycle. Cache invalidation on `pg_notify('compute_nodes_updated', node_id)` is
recommended to keep the cache warm without waiting for the 5s TTL.

---

### 12. Configuration

New sections in `config/o3k.yaml`:

```yaml
server:
  state_dir: "/var/lib/o3k/server"
  tunnel_bind_addr: "10.0.0.1"    # REQUIRED in production — internal interface only
  tunnel_port: 6385
  max_agent_inflight: 1
  max_workers: 10                  # worker goroutines per server node (1 per agent, capped)
  deregister_drain_timeout: 60s

agent:
  server_url: "https://10.0.0.1:6385"    # required
  token: ""                                # or O3K_TOKEN env var (required)
  token_file: ""                           # preferred over token
  node_id: "auto"                          # auto = UUID persisted to disk
  state_dir: "/var/lib/o3k/agent"
  heartbeat_interval: 5s
  stats_interval: 10s
  image_cache_dir: "/var/lib/o3k/agent/images"
  image_cache_max_gb: 50                   # LRU eviction when exceeded

task_timeouts:
  default: 30s
  IMAGE_PREFETCH: 5m
  max_pending_age: 30m       # auto-fail if no agent picks up within this window
  reconciler_interval: 30s   # how often the reconciler scans dispatched tasks
  prune_after: 90d           # completed/failed tasks pruned after this age
  retry_backoff_base: 10s    # backoff = base * 2^(retries-1): retry 1=10s, 2=20s
```

**Retry backoff formula**: `next_retry_at = now() + (retry_backoff_base * 2^(retries-1))`

| retries value when failure detected | next_retry_at offset |
|-------------------------------------|----------------------|
| 0 (first failure, becomes 1) | 10s |
| 1 (second failure, becomes 2) | 20s |
| 2 (third failure → terminal, becomes 3) | no next_retry_at (final) |

Configured via `task_timeouts.retry_backoff_base` (default `10s`). Backoff capped at
5 minutes regardless of base setting. The reconciler uses the same formula when
requeuing timed-out tasks.

**DB connection pool sizing**: Server nodes running TunnelHub + Worker + Reconciler
concurrently require more DB connections than single-node deployments. Increase
`database.max_connections` proportionally to agent count:

```yaml
database:
  max_connections: 50   # default 20 — increase to 50+ when running >5 agents
  # Rule of thumb: 20 + (4 * num_agents)
```

**PostgreSQL `max_connections`**: The pool sizing formula assumes PostgreSQL is
configured with `max_connections >= (pool_size * num_server_nodes) + 10` (for
superuser connections). Default PostgreSQL `max_connections` is 100. For a
3-server cluster with pool_size=50, PostgreSQL needs `max_connections >= 160`.
Set `max_connections = 200` in `postgresql.conf` for a 3-server cluster. Failure
to increase this setting causes connection pool exhaustion under load — workers
will queue and lag behind. Document in the deployment guide alongside the pool
sizing formula.

**Config validation**: Agent config `Validate()` checks at startup (fail-fast):
- `server_url` is non-empty and valid URL
- `token` or `token_file` or `O3K_TOKEN` is set (error if all empty)
- `heartbeat_interval > 0`, `stats_interval > 0`
- `stats_interval < 20s` — enforced because the scheduler/Placement staleness window
  is 30s; a higher value makes the agent invisible to both even while alive
- All duration fields parsed via `time.ParseDuration`

---

## What This Is Not

- Not a full scheduler (no anti-affinity, no availability zones in v1)
- Not live migration (VMs stay on their node)
- Not automatic recovery from agent death (VMs on dead agent stay in `ERROR` state,
  operator triggers evacuation manually)
- Not a replacement for Ceph (shared storage still required for image backends)

These are follow-on specs.

---

## Test Strategy

Per Constitution Article III, TDD is mandatory. All tests must be written and
confirmed RED before any implementation begins.

### Test Infrastructure

| Component | Tool | Rationale |
|-----------|------|-----------|
| Scheduler tests | `dockertest` (real PostgreSQL) | Must exercise `FOR UPDATE SKIP LOCKED` |
| TunnelHub unit tests | `google.golang.org/grpc/test/bufconn` | In-process gRPC, no network |
| Agent local state | `modernc.org/sqlite` with `:memory:` | Fast, CGO-free |
| Stream drop simulation | Cancel server-side context | Assert client detects within heartbeat interval |

### Required Tests (must be RED before implementation)

**internal/tunnel/tunnel_test.go**:
- `TestTunnelHub_AgentRegistersAndReceivesTask`
- `TestTunnelHub_RejectsWrongOrganization` — cert with `O=wrong-org` -> refused
- `TestTunnelHub_RejectsExpiredCert` — expired client cert -> refused
- `TestTunnelHub_RejectsInvalidToken` — bad token -> 401, not 500
- `TestTunnelHub_RejectsMismatchedNodeID` — HELLO node-id != cert CN -> refused
- `TestTunnelHub_HeartbeatTimeoutMarksOffline` — clean disconnect, partition, kill
- `TestTunnelHub_AcceptsResultFromOfflineAgent` — late TaskResult still processed
- `TestTunnelHub_MaxInflightEnforced` — second task blocked until first completes
- `TestTunnelHub_DuplicateNodeID_ReplacesOldStream` — register agent, keep stream open,
  register same node_id again; assert old stream receives close signal, new stream is
  active, DB shows new `agent_stream_server_id`, `agent.reconnect` audit event logged
- `TestTunnelHub_DispatchTimesOutWhenSendBlocks` — fill agent's receive buffer; assert
  `Dispatch` returns timeout error within `timeout_sec`; assert worker enters Tx2 error path

**internal/scheduler/scheduler_test.go** (requires real PostgreSQL via dockertest):
- `TestScheduler_NoConcurrentDoubleBooking` — 8 goroutines, 4 vCPU node, exactly 4 scheduled
- `TestScheduler_ReservationReleasedOnFailure`
- `TestScheduler_SkipsStaleStatsNodes` — stats_updated_at older than 30s
- `TestScheduler_PrefersImageWarmAgent`
- `TestScheduler_SkipsLockedAgent` — SKIP LOCKED behavior
- `TestWorker_ConcurrentTaskClaim_OnlyOneWins` — 8 goroutines racing to claim 1 task;
  assert exactly 1 worker transitions it to `dispatched`, others get no task from SKIP LOCKED

**internal/worker/worker_test.go** (requires real PostgreSQL):
- `TestWorker_TaskRetryStateMachine` (table-driven: retries=0/2/3, permanent error)
- `TestWorker_SeparateTransactions` — Tx1 and Tx2 are independent
- `TestWorker_ReconcilerReleasesStaleReservation`
- `TestWorker_ReconcilerDoesNotDoubleDecrement` — concurrent completion + reconciler
- `TestWorker_Reconciler_MaxRetriesReachedMarksFailed` — insert task with `retries=3`,
  let it go stale; assert reconciler marks `failed` (not `pending`), DB constraint not violated
- `TestWorker_PrefetchThenVmCreateAtomic` — crash between = no orphan
- `TestWorker_DBFailureExposesUnhealthyEndpoint`
- `TestWorker_MaxPendingAgeAutoFails` — insert task, advance clock past max_pending_age,
  assert background scan sets status='failed' and instance status='ERROR'
- `TestNova_IdempotentCreate_DuplicateKeyReturnsExistingResourceID` — POST /servers
  twice with same X-Idempotency-Key; assert second call returns 202 with original
  resource_id (not a new UUID)
- `TestWorker_PgNotifyWakesImmediately` — assert wake within 200ms of pg_notify;
  use a `select` with 500ms timeout to avoid flakiness on slow CI environments
- `TestWorker_DispatchCheckRelease_SecondExecFails` — simulate second db.Exec
  failure in dispatch-check path; assert reconciler correctly detects and requeues
  the task after 2 * timeout_sec with no reservation leak
- `TestNova_AtomicInstanceTaskInsert_PartialFailureRollsBack` — force-fail task
  INSERT after instance INSERT succeeds (via a mock/injected tx); assert no orphan
  instance row remains after the transaction rolls back (closes ISSUE-1)
- `TestNova_DeleteWithInFlightTask_Returns409` — create instance, put task in
  `dispatched` state, call `DELETE /servers/{id}`, assert 409 response and task
  unchanged (closes ISSUE-13)

**internal/agent/agent_test.go**:
- `TestAgent_JoinAndReceiveFirstTaskWithin5s`
- `TestAgent_Reconnect_DeliversCompletedResult`
- `TestAgent_Reconnect_ServerUnknownTask_SendsOrphanReport`
- `TestAgent_Reconnect_TaskAlreadyRetried_AcceptsCleanup`
- `TestAgent_TaskTimeoutCancelsExecution`
- `TestAgent_ImageCacheValidatesChecksum`
- `TestAgent_SingletonCurrentTaskEnforced`
- `TestAgent_ConcurrentCompleteAndReconnect`
- `TestAgent_ImageCacheEviction_LRU` — fill cache to limit, request new image; assert
  LRU entry evicted (file deleted, row removed), new image downloaded, task completes
- `TestAgent_ConcurrentPrefetch_SameImage_NoDuplicateDownload` — dispatch two IMAGE_PREFETCH
  tasks with same `image_id` concurrently; assert only one download occurs (file written
  once, no corruption), both tasks complete successfully
- `TestCertRenew_AllServersLackCAKey_AgentLockedOut` — all server nodes return
  `FAILED_PRECONDITION` for CertRenew; assert agent logs impending expiry prominently
  and does NOT fall back to accepting an expired cert (closes ISSUE-18)
- `TestAgent_ImagePrefetch_DiskFullCleansPartialFile` — inject disk-full error
  mid-download; assert the `.tmp` file is deleted, the permanent image file does not
  exist, and the task fails with `ERROR_TRANSIENT` so it retries (closes ISSUE-12)

**Contract tests** (Article IX — must pass before either side is implemented):
- `TestProto_TaskStreamRoundTrip` — serialize/deserialize all TaskTypes
- `TestProto_TaskResultRoundTrip` — success and all ErrorCode variants
- `TestProto_AgentStatsRoundTrip`
- `TestProto_AllPayloads_HaveVersionField` — assert every payload message has `version`
  field set to 1 by default; guards future schema migration compatibility
- `TestTaskResult_HMACEncoding` — fixed inputs (known nonce, task_id, data); assert
  HMAC output matches a hardcoded expected value (golden test); nails down exact
  length-prefix encoding so any implementation divergence fails immediately

**Integration tests** (`test/`):
- `TestBinaryBackwardCompat` — `o3k` with no args starts all services on correct ports
- `TestIdempotentServerCreate` — same key returns same server ID
- `TestHATaskPickup_CrossServer` — Server 1 crashes, Server 2 picks up task
- `TestNodeList_ShowsConnectedAgents` — `o3k node list` output includes all active agents
- `TestTokenGet_RequiresPrivilege` — `o3k token get` exits non-zero when run as unprivileged user
- `TestNova_POST_Returns202_NotBlocking` — `POST /servers` returns in <100ms with a mock scheduler
- `TestNova_GETServer_Returns_ERROR_OnTaskFailed` — after task transitions to `failed`,
  `GET /servers/{id}` returns `{ "status": "ERROR" }`
- `TestAgent_LibvirtIdempotency_DomainAlreadyExists` — `VM_CREATE` when domain already
  exists returns `completed` (not `ERROR_PERMANENT`)
- `TestTunnelHub_RejectsOrphanReportFabrication` — `OrphanReport` with unknown `task_id`
  is rejected; no instance row is created

---

## Success Criteria

1. `o3k agent --server ... --token-file ...` joins cluster and receives work within 5s
2. `POST /servers` returns 202 in < 100ms at p99 under 50 concurrent requests
3. Agent node failure detected within 10s (2x heartbeat interval)
4. No double-booking under concurrent load (verified by `TestScheduler_NoConcurrentDoubleBooking`)
5. Agent reconnect delivers in-flight task result to new server
6. Agent reconnect when server has no record produces OrphanReport (not silent loss)
7. Rolling server update causes zero task loss (tasks in DB survive)

---

## Documentation Updates Required

When this spec is implemented, the following docs must be updated:

- `docs/ARCHITECTURE.md`: Update "Design Philosophy" to note async exception for
  multi-node. Update service list to include Placement. Remove "No VXLAN in v1"
  (VXLAN is implemented). Remove "v2 - Future" label from multi-node.
- `docs/SCALING.md`: Add notice at top that server/agent architecture supersedes
  the HAProxy model described there.
- `docs/INDEX.md`: Add "Design Specs" section linking to this document.
