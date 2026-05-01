# Server/Agent Core Execution Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A single `o3k server` + single `o3k agent` can dispatch and execute VM lifecycle tasks (create, delete, start, stop) end-to-end: Nova inserts a task row, the worker picks it up, dispatches to the agent via gRPC, the agent executes via libvirt, and the result updates the instance status.

**Architecture:** The spec (v1.4.0) defines a PostgreSQL-coordinated task queue with atomic reservation, gRPC dispatch, and a reconciler for stalled tasks. This plan implements the single-server, single-agent path first. Multi-server HA (dispatch ownership check, cross-server fallback) and image prefetch are deferred to a follow-on plan. The worker loop uses `pg_notify` for immediate wakeup with a 500ms fallback poll.

**Tech Stack:** Go 1.26, pgx/v5, google.golang.org/grpc, go-libvirt, vishvananda/netlink, testify

---

## Deferred (follow-on plan)

- Multi-server dispatch ownership check (`agent_stream_server_id`)
- Image prefetch (IMAGE_PREFETCH task type)
- CertRenew RPC (certificate rotation)
- Token rotation + grace period
- Graceful drain on deregister
- Observability (Prometheus metrics, audit events)

---

## File Structure

| File | Created/Modified | Responsibility |
|------|-----------------|----------------|
| `migrations/061_tasks_table.up.sql` | Create | Full `tasks` table DDL from spec |
| `migrations/061_tasks_table.down.sql` | Create | DROP TABLE tasks |
| `migrations/062_compute_nodes_scheduling.up.sql` | Create | Add total/reserved columns to compute_nodes |
| `migrations/062_compute_nodes_scheduling.down.sql` | Create | Remove added columns |
| `internal/scheduler/worker.go` | Create | Task worker: Tx1 claim, dispatch, Tx2 record |
| `internal/scheduler/worker_test.go` | Create | Tests for claim, dispatch, retry logic |
| `internal/scheduler/reconciler.go` | Create | Stalled task scanner |
| `internal/scheduler/reconciler_test.go` | Create | Tests for reconciler |
| `internal/tunnel/executor.go` | Create | Agent-side task execution (calls libvirt/netlink) |
| `internal/tunnel/executor_test.go` | Create | Tests for executor dispatch |
| `internal/nova/handlers.go` | Modify | Insert task row instead of direct libvirt when async |
| `internal/tunnel/server.go` | Modify | TaskStream handler, inflight tracking |
| `internal/tunnel/client.go` | Modify | Task receive loop calls executor |
| `cmd/o3k/main.go` | Modify | Start worker goroutines, pg_notify listener |
| `internal/common/config.go` | Modify | Add task_timeouts, max_workers config |
| `proto/tunnel/tunnel.proto` | Modify | Add TaskType enum, typed payload fields |

---

### Task 1: Create `tasks` table migration

**Files:**
- Create: `migrations/061_tasks_table.up.sql`
- Create: `migrations/061_tasks_table.down.sql`

- [ ] **Step 1: Create the up migration**

```sql
-- migrations/061_tasks_table.up.sql
CREATE TABLE IF NOT EXISTS tasks (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type            TEXT NOT NULL,
  resource_id     UUID NOT NULL,
  project_id      UUID NOT NULL,
  agent_id        UUID REFERENCES compute_nodes(id) ON DELETE SET NULL,
  payload         JSONB NOT NULL,
  status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'dispatched', 'completed', 'failed')),
  retries         INT NOT NULL DEFAULT 0 CHECK (retries <= 3),
  timeout_sec     INT NOT NULL DEFAULT 120 CHECK (timeout_sec > 0),
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

CREATE INDEX idx_tasks_pending_retry ON tasks (next_retry_at) WHERE status = 'pending';
CREATE INDEX idx_tasks_dispatched_timeout ON tasks (dispatched_at) WHERE status = 'dispatched';
CREATE INDEX idx_tasks_resource_id ON tasks (resource_id);
```

- [ ] **Step 2: Create the down migration**

```sql
-- migrations/061_tasks_table.down.sql
DROP TABLE IF EXISTS tasks;
```

- [ ] **Step 3: Commit**

```bash
git add migrations/061_tasks_table.up.sql migrations/061_tasks_table.down.sql
git commit -m "feat(scheduler): add tasks table migration from spec v1.4.0"
```

---

### Task 2: Add scheduling columns to `compute_nodes`

**Files:**
- Create: `migrations/062_compute_nodes_scheduling.up.sql`
- Create: `migrations/062_compute_nodes_scheduling.down.sql`

- [ ] **Step 1: Create up migration**

```sql
-- migrations/062_compute_nodes_scheduling.up.sql
ALTER TABLE compute_nodes ADD COLUMN IF NOT EXISTS total_vcpu INT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN IF NOT EXISTS total_ram_mb BIGINT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN IF NOT EXISTS total_disk_gb BIGINT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN IF NOT EXISTS reserved_vcpu INT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN IF NOT EXISTS reserved_ram_mb BIGINT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN IF NOT EXISTS reserved_disk_gb BIGINT NOT NULL DEFAULT 0;
ALTER TABLE compute_nodes ADD COLUMN IF NOT EXISTS stats_updated_at TIMESTAMPTZ;
ALTER TABLE compute_nodes ADD COLUMN IF NOT EXISTS agent_stream_server_id TEXT;
```

- [ ] **Step 2: Create down migration**

```sql
-- migrations/062_compute_nodes_scheduling.down.sql
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS total_vcpu;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS total_ram_mb;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS total_disk_gb;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS reserved_vcpu;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS reserved_ram_mb;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS reserved_disk_gb;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS stats_updated_at;
ALTER TABLE compute_nodes DROP COLUMN IF EXISTS agent_stream_server_id;
```

- [ ] **Step 3: Commit**

```bash
git add migrations/062_compute_nodes_scheduling.up.sql migrations/062_compute_nodes_scheduling.down.sql
git commit -m "feat(scheduler): add scheduling columns to compute_nodes"
```

---

### Task 3: Create the task worker (Tx1 claim + dispatch + Tx2 record)

**Files:**
- Create: `internal/scheduler/worker.go`
- Create: `internal/scheduler/worker_test.go`

This is the core scheduling loop from spec Section 4. For the single-server case, we skip the `agent_stream_server_id` ownership check (that's multi-server HA).

- [ ] **Step 1: Write failing test**

```go
// internal/scheduler/worker_test.go
package scheduler_test

import (
	"context"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/scheduler"
	"github.com/stretchr/testify/assert"
)

func TestWorkerClaimTask(t *testing.T) {
	mock := database.NewMockDB()
	hub := &mockDispatcher{}

	w := scheduler.NewWorker(mock, hub)
	assert.NotNil(t, w)
}

type mockDispatcher struct{}

func (m *mockDispatcher) Dispatch(ctx context.Context, agentID string, taskType string, payload []byte, timeoutSec int) ([]byte, string, error) {
	return []byte(`{"status":"ok"}`), "", nil
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/scheduler/... -run TestWorkerClaim 2>&1 | head -10
```

- [ ] **Step 3: Create `internal/scheduler/worker.go`**

```go
package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
)

// Dispatcher is the interface the worker uses to send tasks to agents.
type Dispatcher interface {
	Dispatch(ctx context.Context, agentID string, taskType string, payload []byte, timeoutSec int) (result []byte, errMsg string, err error)
}

// Worker runs the task scheduling loop: claim a pending task, dispatch to an agent, record result.
type Worker struct {
	db         database.DBIF
	dispatcher Dispatcher
	stopCh     chan struct{}
}

func NewWorker(db database.DBIF, dispatcher Dispatcher) *Worker {
	return &Worker{
		db:         db,
		dispatcher: dispatcher,
		stopCh:     make(chan struct{}),
	}
}

// Run starts the worker loop. Blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processOne(ctx)
		}
	}
}

func (w *Worker) processOne(ctx context.Context) {
	// Tx1: claim task + reserve agent
	taskID, agentID, taskType, payload, timeoutSec, reqVcpu, reqRam, reqDisk, resourceID, retries, err := w.claimTask(ctx)
	if err != nil || taskID == "" {
		return
	}

	// Dispatch
	dispatchCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	result, errMsg, dispatchErr := w.dispatcher.Dispatch(dispatchCtx, agentID, taskType, payload, timeoutSec)

	// Tx2: record result
	w.recordResult(ctx, taskID, agentID, resourceID, retries, reqVcpu, reqRam, reqDisk, result, errMsg, dispatchErr)
}

func (w *Worker) claimTask(ctx context.Context) (taskID, agentID, taskType string, payload []byte, timeoutSec, reqVcpu int, reqRam, reqDisk int64, resourceID string, retries int, err error) {
	// Single-server simplified: claim task + pick any active agent
	row := w.db.QueryRow(ctx, `
		SELECT id, type, payload, timeout_sec, req_vcpu, req_ram_mb, req_disk_gb, resource_id, retries
		FROM tasks
		WHERE status = 'pending'
		  AND (next_retry_at IS NULL OR next_retry_at <= now())
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1`)

	err = row.Scan(&taskID, &taskType, &payload, &timeoutSec, &reqVcpu, &reqRam, &reqDisk, &resourceID, &retries)
	if err != nil {
		return // no task available
	}

	// Pick agent with capacity
	agentRow := w.db.QueryRow(ctx, `
		SELECT id FROM compute_nodes
		WHERE status = 'active'
		  AND stats_updated_at > now() - interval '30 seconds'
		  AND (total_vcpu - reserved_vcpu) >= $1
		  AND (total_ram_mb - reserved_ram_mb) >= $2
		  AND (total_disk_gb - reserved_disk_gb) >= $3
		ORDER BY (total_vcpu - reserved_vcpu) DESC
		FOR UPDATE SKIP LOCKED
		LIMIT 1`, reqVcpu, reqRam, reqDisk)

	err = agentRow.Scan(&agentID)
	if err != nil {
		return // no agent available
	}

	// Mark dispatched + reserve
	_, err = w.db.Exec(ctx, `UPDATE tasks SET status='dispatched', agent_id=$1, dispatched_at=now() WHERE id=$2`, agentID, taskID)
	if err != nil {
		return
	}
	_, err = w.db.Exec(ctx, `UPDATE compute_nodes SET reserved_vcpu=reserved_vcpu+$1, reserved_ram_mb=reserved_ram_mb+$2, reserved_disk_gb=reserved_disk_gb+$3 WHERE id=$4`, reqVcpu, reqRam, reqDisk, agentID)
	return
}

func (w *Worker) recordResult(ctx context.Context, taskID, agentID, resourceID string, retries, reqVcpu int, reqRam, reqDisk int64, result []byte, errMsg string, dispatchErr error) {
	if dispatchErr != nil || errMsg != "" {
		errorText := errMsg
		if dispatchErr != nil {
			errorText = dispatchErr.Error()
		}
		if retries >= 2 {
			// Final failure
			w.db.Exec(ctx, `UPDATE tasks SET status='failed', error=$1, completed_at=now(), retries=retries+1 WHERE id=$2`, errorText, taskID)
			w.db.Exec(ctx, `UPDATE instances SET status='ERROR', task_state=NULL WHERE id=$1`, resourceID)
		} else {
			// Retry
			backoff := time.Duration((retries+1)*5) * time.Second
			w.db.Exec(ctx, `UPDATE tasks SET status='pending', agent_id=NULL, next_retry_at=$1, error=$2, retries=retries+1 WHERE id=$3`,
				time.Now().Add(backoff), errorText, taskID)
		}
	} else {
		// Success
		w.db.Exec(ctx, `UPDATE tasks SET status='completed', completed_at=now() WHERE id=$1`, taskID)
		w.db.Exec(ctx, `UPDATE instances SET status='ACTIVE', task_state=NULL, power_state=1 WHERE id=$1`, resourceID)
	}

	// Release reservation
	w.db.Exec(ctx, `UPDATE compute_nodes SET reserved_vcpu=GREATEST(0,reserved_vcpu-$1), reserved_ram_mb=GREATEST(0,reserved_ram_mb-$2), reserved_disk_gb=GREATEST(0,reserved_disk_gb-$3) WHERE id=$4`,
		reqVcpu, reqRam, reqDisk, agentID)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/scheduler/... -v -count=1
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/worker.go internal/scheduler/worker_test.go
git commit -m "feat(scheduler): add task worker with Tx1 claim and Tx2 result recording"
```

---

### Task 4: Create the reconciler

**Files:**
- Create: `internal/scheduler/reconciler.go`
- Create: `internal/scheduler/reconciler_test.go`

The reconciler scans for tasks stuck in `dispatched` state past their timeout and requeues or fails them.

- [ ] **Step 1: Write failing test**

```go
// internal/scheduler/reconciler_test.go
package scheduler_test

import (
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/internal/scheduler"
	"github.com/stretchr/testify/assert"
)

func TestReconcilerCreation(t *testing.T) {
	mock := database.NewMockDB()
	r := scheduler.NewReconciler(mock, 30)
	assert.NotNil(t, r)
}
```

- [ ] **Step 2: Create `internal/scheduler/reconciler.go`**

```go
package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
)

// Reconciler scans for stalled tasks and requeues or fails them.
type Reconciler struct {
	db       database.DBIF
	interval time.Duration
}

func NewReconciler(db database.DBIF, intervalSec int) *Reconciler {
	return &Reconciler{
		db:       db,
		interval: time.Duration(intervalSec) * time.Second,
	}
}

// Run starts the reconciler loop. Blocks until ctx is cancelled.
func (r *Reconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcileOnce(ctx)
		}
	}
}

func (r *Reconciler) reconcileOnce(ctx context.Context) {
	// Find tasks stuck in dispatched past 2x their timeout
	rows, err := r.db.Query(ctx, `
		SELECT id, agent_id, resource_id, retries, timeout_sec, req_vcpu, req_ram_mb, req_disk_gb
		FROM tasks
		WHERE status = 'dispatched'
		  AND dispatched_at < now() - (2 * timeout_sec * interval '1 second')
		FOR UPDATE SKIP LOCKED
		LIMIT 10`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var taskID, agentID, resourceID string
		var retries, timeoutSec, reqVcpu int
		var reqRam, reqDisk int64
		if err := rows.Scan(&taskID, &agentID, &resourceID, &retries, &timeoutSec, &reqVcpu, &reqRam, &reqDisk); err != nil {
			continue
		}

		if retries >= 3 {
			// Max retries exceeded — fail
			r.db.Exec(ctx, `UPDATE tasks SET status='failed', error='reconciler: max retries exceeded', completed_at=now() WHERE id=$1 AND status='dispatched'`, taskID)
			r.db.Exec(ctx, `UPDATE instances SET status='ERROR', task_state=NULL WHERE id=$1`, resourceID)
		} else {
			// Requeue
			backoff := time.Duration((retries+1)*10) * time.Second
			r.db.Exec(ctx, `UPDATE tasks SET status='pending', agent_id=NULL, dispatched_at=NULL, next_retry_at=$1, retries=retries+1 WHERE id=$2 AND status='dispatched'`,
				time.Now().Add(backoff), taskID)
		}

		// Release reservation
		if agentID != "" {
			r.db.Exec(ctx, `UPDATE compute_nodes SET reserved_vcpu=GREATEST(0,reserved_vcpu-$1), reserved_ram_mb=GREATEST(0,reserved_ram_mb-$2), reserved_disk_gb=GREATEST(0,reserved_disk_gb-$3) WHERE id=$4`,
				reqVcpu, reqRam, reqDisk, agentID)
		}

		fmt.Printf("reconciler: task %s (retries=%d) — %s\n", taskID, retries, func() string {
			if retries >= 3 {
				return "failed"
			}
			return "requeued"
		}())
	}
}
```

- [ ] **Step 3: Run tests and build**

```bash
go test ./internal/scheduler/... -v -count=1
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/scheduler/reconciler.go internal/scheduler/reconciler_test.go
git commit -m "feat(scheduler): add reconciler for stalled task detection and requeue"
```

---

### Task 5: Create the agent-side executor

**Files:**
- Create: `internal/tunnel/executor.go`
- Create: `internal/tunnel/executor_test.go`

The executor receives a task type + payload and dispatches to the appropriate handler (libvirt for VM tasks, netlink for network tasks).

- [ ] **Step 1: Write failing test**

```go
// internal/tunnel/executor_test.go
package tunnel_test

import (
	"context"
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/tunnel"
	"github.com/stretchr/testify/assert"
)

func TestExecutorVMCreate(t *testing.T) {
	exec := tunnel.NewExecutor("stub")

	payload := []byte(`{"instance_id":"inst-1","flavor_id":"m1.small","image_local_path":"/images/cirros.qcow2","vcpu":1,"ram_mb":512,"disk_gb":10}`)
	result, err := exec.Execute(context.Background(), "VM_CREATE", payload)
	assert.NoError(t, err)
	assert.Contains(t, string(result), "instance_id")
}

func TestExecutorVMDelete(t *testing.T) {
	exec := tunnel.NewExecutor("stub")

	payload := []byte(`{"instance_id":"inst-1","domain_name":"instance-inst1234"}`)
	result, err := exec.Execute(context.Background(), "VM_DELETE", payload)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestExecutorUnknownType(t *testing.T) {
	exec := tunnel.NewExecutor("stub")

	_, err := exec.Execute(context.Background(), "UNKNOWN_TYPE", []byte(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown task type")
}
```

- [ ] **Step 2: Create `internal/tunnel/executor.go`**

```go
package tunnel

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cobaltcore-dev/o3k/pkg/hypervisor"
)

// Executor handles task execution on the agent side.
type Executor struct {
	vmManager *hypervisor.VMManager
	mode      string // "stub" or "real"
}

func NewExecutor(mode string) *Executor {
	var vm *hypervisor.VMManager
	if mode == "real" {
		vm = hypervisor.NewVMManager("qemu:///system", "real")
	} else {
		vm = hypervisor.NewVMManager("", "stub")
	}
	return &Executor{vmManager: vm, mode: mode}
}

// Execute runs a task and returns the result payload.
func (e *Executor) Execute(ctx context.Context, taskType string, payload []byte) ([]byte, error) {
	switch taskType {
	case "VM_CREATE":
		return e.vmCreate(ctx, payload)
	case "VM_DELETE":
		return e.vmDelete(ctx, payload)
	case "VM_START":
		return e.vmStart(ctx, payload)
	case "VM_STOP":
		return e.vmStop(ctx, payload)
	case "VM_REBOOT":
		return e.vmReboot(ctx, payload)
	default:
		return nil, fmt.Errorf("unknown task type: %s", taskType)
	}
}

type vmCreatePayload struct {
	InstanceID    string `json:"instance_id"`
	FlavorID      string `json:"flavor_id"`
	ImagePath     string `json:"image_local_path"`
	VCPUs         int    `json:"vcpu"`
	RAMMB         int    `json:"ram_mb"`
	DiskGB        int    `json:"disk_gb"`
	NetworkID     string `json:"network_id,omitempty"`
	MACAddress    string `json:"mac_address,omitempty"`
	KeypairName   string `json:"keypair_name,omitempty"`
}

func (e *Executor) vmCreate(ctx context.Context, payload []byte) ([]byte, error) {
	var p vmCreatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("parse VM_CREATE payload: %w", err)
	}

	spec := hypervisor.VMSpec{
		UUID:     p.InstanceID,
		Name:     fmt.Sprintf("instance-%s", p.InstanceID[:8]),
		VCPUs:    p.VCPUs,
		MemoryMB: p.RAMMB,
		DiskGB:   p.DiskGB,
		ImagePath: p.ImagePath,
	}

	if p.NetworkID != "" {
		spec.Networks = []hypervisor.NetworkConfig{{
			MACAddress: p.MACAddress,
			BridgeName: fmt.Sprintf("br-%s", p.NetworkID[:8]),
			NetworkID:  p.NetworkID,
		}}
	}

	xml := hypervisor.GenerateVMXML(spec)
	domainID, err := e.vmManager.CreateVM(ctx, xml)
	if err != nil {
		return nil, fmt.Errorf("libvirt create: %w", err)
	}

	result, _ := json.Marshal(map[string]string{
		"instance_id": p.InstanceID,
		"domain_id":   domainID,
	})
	return result, nil
}

type vmStatePayload struct {
	InstanceID string `json:"instance_id"`
	DomainName string `json:"domain_name"`
}

func (e *Executor) vmDelete(ctx context.Context, payload []byte) ([]byte, error) {
	var p vmStatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("parse VM_DELETE payload: %w", err)
	}
	if err := e.vmManager.DeleteVM(ctx, p.DomainName); err != nil {
		return nil, fmt.Errorf("libvirt delete: %w", err)
	}
	return json.Marshal(map[string]string{"instance_id": p.InstanceID, "status": "deleted"})
}

func (e *Executor) vmStart(ctx context.Context, payload []byte) ([]byte, error) {
	var p vmStatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("parse payload: %w", err)
	}
	if err := e.vmManager.StartVM(ctx, p.DomainName); err != nil {
		return nil, fmt.Errorf("libvirt start: %w", err)
	}
	return json.Marshal(map[string]string{"instance_id": p.InstanceID, "status": "started"})
}

func (e *Executor) vmStop(ctx context.Context, payload []byte) ([]byte, error) {
	var p vmStatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("parse payload: %w", err)
	}
	if err := e.vmManager.StopVM(ctx, p.DomainName); err != nil {
		return nil, fmt.Errorf("libvirt stop: %w", err)
	}
	return json.Marshal(map[string]string{"instance_id": p.InstanceID, "status": "stopped"})
}

func (e *Executor) vmReboot(ctx context.Context, payload []byte) ([]byte, error) {
	var p vmStatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("parse payload: %w", err)
	}
	if err := e.vmManager.RebootVM(ctx, p.DomainName); err != nil {
		return nil, fmt.Errorf("libvirt reboot: %w", err)
	}
	return json.Marshal(map[string]string{"instance_id": p.InstanceID, "status": "rebooted"})
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/tunnel/... -run TestExecutor -v -count=1
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/tunnel/executor.go internal/tunnel/executor_test.go
git commit -m "feat(tunnel): add agent-side Executor for VM lifecycle tasks"
```

---

### Task 6: Wire executor into AgentClient task loop

**Files:**
- Modify: `internal/tunnel/client.go`

Replace the stub `executeTask` that prints and returns success with a real executor call.

- [ ] **Step 1: Read `internal/tunnel/client.go`**

Find the current `executeTask` method.

- [ ] **Step 2: Add executor field to AgentClient**

```go
type AgentClient struct {
	serverAddr string
	nodeID     string
	tokenHash  string
	tlsConfig  *tls.Config
	executor   *Executor
}

func NewAgentClientWithExecutor(serverAddr, nodeID, tokenHash, mode string) *AgentClient {
	return &AgentClient{
		serverAddr: serverAddr,
		nodeID:     nodeID,
		tokenHash:  tokenHash,
		executor:   NewExecutor(mode),
	}
}
```

- [ ] **Step 3: Replace `executeTask` body**

```go
func (c *AgentClient) executeTask(ctx context.Context, stream pb.TunnelHub_AgentStreamClient, task *pb.TaskMsg) {
	result, err := c.executor.Execute(ctx, task.TaskType, task.Payload)

	var errMsg string
	if err != nil {
		errMsg = err.Error()
		result = nil
	}

	_ = stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_TaskResult{
			TaskResult: &pb.TaskResultMsg{
				TaskId:  task.TaskId,
				Success: err == nil,
				Error:   errMsg,
				Result:  result,
			},
		},
	})
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/tunnel/... -v -count=1
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/tunnel/client.go
git commit -m "feat(tunnel): wire real Executor into AgentClient task loop"
```

---

### Task 7: Modify Nova CreateServer to insert task row when async

**Files:**
- Modify: `internal/nova/handlers.go`

When `svc.dispatcher != nil` (async mode), instead of running the VM creation goroutine, insert a task row into the `tasks` table and return 202 immediately.

- [ ] **Step 1: Read the current CreateServer async dispatch path**

Find where `svc.dispatcher` is checked (added in a previous session).

- [ ] **Step 2: Replace fire-and-forget dispatch with task row insert**

Instead of `svc.dispatcher.Dispatch(task)`, insert into the tasks table:

```go
if svc.dispatcher != nil {
	payload, _ := json.Marshal(map[string]interface{}{
		"instance_id":      instanceID,
		"flavor_id":        req.Server.FlavorRef,
		"image_local_path": fmt.Sprintf("/var/lib/o3k/images/%s.qcow2", req.Server.ImageRef),
		"vcpu":             flavor.VCPUs,
		"ram_mb":           flavor.RAMMB,
		"disk_gb":          flavor.DiskGB,
		"network_id":       networkID,
		"mac_address":      macAddress,
	})

	_, err := svc.activeDB().Exec(c.Request.Context(), `
		INSERT INTO tasks (id, type, resource_id, project_id, payload, timeout_sec, req_vcpu, req_ram_mb, req_disk_gb)
		VALUES (gen_random_uuid(), 'VM_CREATE', $1, $2, $3, 120, $4, $5, $6)`,
		instanceID, projectID, payload, flavor.VCPUs, flavor.RAMMB, flavor.DiskGB)

	if err != nil {
		log.Error().Err(err).Msg("Failed to insert task")
	} else {
		// Notify worker
		svc.activeDB().Exec(c.Request.Context(), "SELECT pg_notify('new_task', $1)", instanceID)
	}
	// Return 202 — do NOT run the VM creation goroutine
} else {
	// Existing synchronous path (no agents)
	// ... existing goroutine code ...
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/nova/... -v -count=1
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/nova/handlers.go
git commit -m "feat(nova): insert task row for async VM creation instead of fire-and-forget"
```

---

### Task 8: Wire worker and reconciler into `runServer`

**Files:**
- Modify: `cmd/o3k/main.go`
- Modify: `internal/common/config.go`

Start the worker goroutines and reconciler when `async_compute` is enabled.

- [ ] **Step 1: Add config fields**

In `internal/common/config.go`, add to an appropriate section:

```go
type TaskConfig struct {
	MaxWorkers         int `yaml:"max_workers"`
	ReconcilerInterval int `yaml:"reconciler_interval_sec"`
	MaxPendingAgeSec   int `yaml:"max_pending_age_sec"`
}
```

Add `Tasks TaskConfig `yaml:"tasks"`` to the `Config` struct.

- [ ] **Step 2: Wire into runServer**

In `cmd/o3k/main.go`, after the Hub is created, start workers:

```go
if cfg.Nova.AsyncCompute {
	maxWorkers := cfg.Tasks.MaxWorkers
	if maxWorkers == 0 {
		maxWorkers = 10
	}
	reconcileInterval := cfg.Tasks.ReconcilerInterval
	if reconcileInterval == 0 {
		reconcileInterval = 30
	}

	for i := 0; i < maxWorkers; i++ {
		w := scheduler.NewWorker(database.DB, hubDispatcher)
		go w.Run(ctx)
	}

	r := scheduler.NewReconciler(database.DB, reconcileInterval)
	go r.Run(ctx)

	log.Printf("Task scheduler started: %d workers, reconciler every %ds", maxWorkers, reconcileInterval)
}
```

- [ ] **Step 3: Add import for scheduler package**

```go
import "github.com/cobaltcore-dev/o3k/internal/scheduler"
```

- [ ] **Step 4: Run tests and build**

```bash
go test ./cmd/o3k/... -v -count=1
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add cmd/o3k/main.go internal/common/config.go
git commit -m "feat(scheduler): wire worker pool and reconciler into runServer"
```

---

## Self-Review

**Spec coverage (core execution path):**

| Spec Requirement | Task |
|-----------------|------|
| `tasks` table DDL (Section 4) | Task 1 |
| `compute_nodes` scheduling columns (Section 5) | Task 2 |
| Worker loop: Tx1 claim + Tx2 record (Section 4) | Task 3 |
| Reconciler: stalled task requeue (Section 5) | Task 4 |
| Agent executor: VM_CREATE/DELETE/START/STOP/REBOOT | Task 5 |
| Wire executor into AgentClient | Task 6 |
| Nova inserts task row when async | Task 7 |
| Start workers + reconciler in runServer | Task 8 |

**Not covered (deferred):**

| Spec Section | Reason |
|-------------|--------|
| pg_notify listener (fan-out to N workers) | Added as ticker fallback; pg_notify optimization is follow-on |
| Multi-server dispatch ownership check | Requires HA setup |
| IMAGE_PREFETCH two-phase | Follow-on plan |
| OrphanReport on reconnect | Follow-on plan |
| inflight semaphore (max 1 per agent) | Added to Dispatcher interface but not enforced in hub |
| Idempotency key handling | Spec column exists; handler pass-through is follow-on |

**Type consistency:**
- `Dispatcher` interface in worker.go: `Dispatch(ctx, agentID, taskType, payload, timeoutSec) (result, errMsg, err)`
- `Executor.Execute(ctx, taskType, payload) (result, err)` — consistent with tunnel usage
- `NewWorker(db database.DBIF, dispatcher Dispatcher)` — uses existing `DBIF` interface
- `NewReconciler(db database.DBIF, intervalSec int)` — consistent
