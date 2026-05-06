package scheduler

import (
	"context"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/jackc/pgx/v5"
)

// Dispatcher sends a task to the agent identified by agentID and returns the
// raw result bytes, an application-level error message, or a transport error.
type Dispatcher interface {
	Dispatch(ctx context.Context, agentID string, taskType string, payload []byte, timeoutSec int) (result []byte, errMsg string, err error)
}

// Worker polls the tasks table, atomically claims one pending task alongside a
// capable compute node, dispatches the work via Dispatcher, and records the outcome.
type Worker struct {
	db         database.DBIF
	dispatcher Dispatcher
}

// NewWorker constructs a Worker backed by db and dispatcher.
func NewWorker(db database.DBIF, dispatcher Dispatcher) *Worker {
	return &Worker{db: db, dispatcher: dispatcher}
}

// Run blocks until ctx is cancelled, polling for pending tasks every 500 ms.
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

// ProcessOne claims and dispatches one pending task, if any.
// Exported for testing.
func (w *Worker) ProcessOne(ctx context.Context) {
	w.processOne(ctx)
}

func (w *Worker) processOne(ctx context.Context) {
	taskID, agentID, taskType, payload, timeoutSec, reqVcpu, reqRam, reqDisk, resourceID, retries, err := w.claimTask(ctx)
	if err != nil || taskID == "" {
		return
	}

	dispatchCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	result, errMsg, dispatchErr := w.dispatcher.Dispatch(dispatchCtx, agentID, taskType, payload, timeoutSec)
	w.recordResult(ctx, taskID, agentID, resourceID, retries, reqVcpu, reqRam, reqDisk, result, errMsg, dispatchErr)
}

// claimTask selects the oldest pending task and a matching compute node inside
// an explicit transaction so that the SELECT … FOR UPDATE SKIP LOCKED and the
// subsequent UPDATEs are atomic.  Both rows are locked with FOR UPDATE SKIP
// LOCKED so concurrent workers never pick the same pair.
func (w *Worker) claimTask(ctx context.Context) (taskID, agentID, taskType string, payload []byte, timeoutSec, reqVcpu int, reqRam, reqDisk int64, resourceID string, retries int, err error) {
	tx, err := w.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return
	}
	// Guard against MockDB which returns (nil, nil).
	if tx == nil {
		return
	}

	// Rollback on any failure path; a no-op after a successful Commit.
	defer func() {
		if err != nil || taskID == "" {
			_ = tx.Rollback(ctx)
		}
	}()

	row := tx.QueryRow(ctx, `
		SELECT id, type, payload, timeout_sec, req_vcpu, req_ram_mb, req_disk_gb, resource_id, retries
		FROM tasks
		WHERE status = 'pending'
		  AND (next_retry_at IS NULL OR next_retry_at <= now())
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1`)

	err = row.Scan(&taskID, &taskType, &payload, &timeoutSec, &reqVcpu, &reqRam, &reqDisk, &resourceID, &retries)
	if err != nil {
		if err == pgx.ErrNoRows {
			err = nil
		}
		return
	}

	agentRow := tx.QueryRow(ctx, `
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
		if err == pgx.ErrNoRows {
			err = nil
			taskID = ""
		}
		return
	}

	_, err = tx.Exec(ctx, `UPDATE tasks SET status='dispatched', agent_id=$1, dispatched_at=now() WHERE id=$2`, agentID, taskID)
	if err != nil {
		taskID = ""
		return
	}
	_, err = tx.Exec(ctx, `UPDATE compute_nodes SET reserved_vcpu=reserved_vcpu+$1, reserved_ram_mb=reserved_ram_mb+$2, reserved_disk_gb=reserved_disk_gb+$3 WHERE id=$4`, reqVcpu, reqRam, reqDisk, agentID)
	if err != nil {
		taskID = ""
		return
	}

	err = tx.Commit(ctx)
	if err != nil {
		taskID = ""
	}
	return
}

// recordResult persists the dispatch outcome and releases reserved resources on
// the compute node.  Failed tasks are retried up to 2 times with exponential
// back-off; after that the task and its linked instance are marked failed.
func (w *Worker) recordResult(ctx context.Context, taskID, agentID, resourceID string, retries, reqVcpu int, reqRam, reqDisk int64, result []byte, errMsg string, dispatchErr error) {
	if dispatchErr != nil || errMsg != "" {
		errorText := errMsg
		if dispatchErr != nil {
			errorText = dispatchErr.Error()
		}
		if retries >= maxTaskRetries {
			w.db.Exec(ctx, `UPDATE tasks SET status='failed', error=$1, completed_at=now(), retries=retries+1 WHERE id=$2`, errorText, taskID) //nolint:errcheck
			w.db.Exec(ctx, `UPDATE instances SET status='ERROR', task_state=NULL WHERE id=$1`, resourceID)                                     //nolint:errcheck
		} else {
			backoff := time.Duration((retries+1)*5) * time.Second
			w.db.Exec(ctx, `UPDATE tasks SET status='pending', agent_id=NULL, next_retry_at=$1, error=$2, retries=retries+1 WHERE id=$3`, //nolint:errcheck
				time.Now().Add(backoff), errorText, taskID)
		}
	} else {
		w.db.Exec(ctx, `UPDATE tasks SET status='completed', completed_at=now() WHERE id=$1`, taskID)                   //nolint:errcheck
		w.db.Exec(ctx, `UPDATE instances SET status='ACTIVE', task_state=NULL, power_state=1 WHERE id=$1`, resourceID) //nolint:errcheck
	}

	w.db.Exec(ctx, `UPDATE compute_nodes SET reserved_vcpu=GREATEST(0,reserved_vcpu-$1), reserved_ram_mb=GREATEST(0,reserved_ram_mb-$2), reserved_disk_gb=GREATEST(0,reserved_disk_gb-$3) WHERE id=$4`, //nolint:errcheck
		reqVcpu, reqRam, reqDisk, agentID)
}
