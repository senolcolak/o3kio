package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/rs/zerolog/log"
)

const maxTaskRetries = 3

// Reconciler scans for tasks that have been stuck in dispatched state past 2x
// their timeout and either requeues them (if retries < 3) or marks them failed.
type Reconciler struct {
	db       database.DBIF
	interval time.Duration
}

// NewReconciler returns a Reconciler that polls every intervalSec seconds.
// If intervalSec is <= 0, it defaults to 30 seconds.
func NewReconciler(db database.DBIF, intervalSec int) *Reconciler {
	if intervalSec <= 0 {
		intervalSec = 30
	}
	return &Reconciler{
		db:       db,
		interval: time.Duration(intervalSec) * time.Second,
	}
}

// Run blocks until ctx is cancelled, reconciling stalled tasks on each tick.
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
	tx, err := r.db.BeginTx(ctx, database.TxOptions{})
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, agent_id, resource_id, retries, timeout_sec, req_vcpu, req_ram_mb, req_disk_gb
		FROM tasks
		WHERE status = 'dispatched'
		  AND dispatched_at < now() - (2 * timeout_sec * interval '1 second')
		FOR UPDATE SKIP LOCKED
		LIMIT 10`)
	if err != nil {
		return
	}

	// Collect all rows before closing the cursor. On SQLite (MaxOpenConns=1)
	// executing UPDATE statements while a rows cursor is open deadlocks because
	// both operations compete for the single connection.
	type stalledTask struct {
		taskID, agentID, resourceID string
		retries, timeoutSec, reqVcpu int
		reqRam, reqDisk             int64
	}
	var stalled []stalledTask
	for rows.Next() {
		var t stalledTask
		if err := rows.Scan(&t.taskID, &t.agentID, &t.resourceID, &t.retries, &t.timeoutSec, &t.reqVcpu, &t.reqRam, &t.reqDisk); err != nil {
			continue
		}
		stalled = append(stalled, t)
	}
	rows.Close()

	for _, t := range stalled {
		if t.retries >= maxTaskRetries {
			if _, err := tx.Exec(ctx, `UPDATE tasks SET status='failed', error='reconciler: max retries exceeded', completed_at=now() WHERE id=$1 AND status='dispatched'`, t.taskID); err != nil {
				log.Error().Err(err).Str("task_id", t.taskID).Msg("reconciler: failed to mark task failed")
			}
			if _, err := tx.Exec(ctx, `UPDATE instances SET status='ERROR', task_state=NULL WHERE id=$1`, t.resourceID); err != nil {
				log.Error().Err(err).Str("resource_id", t.resourceID).Msg("reconciler: failed to mark instance ERROR")
			}
		} else {
			backoff := time.Duration((t.retries+1)*10) * time.Second
			if _, err := tx.Exec(ctx, `UPDATE tasks SET status='pending', agent_id=NULL, dispatched_at=NULL, next_retry_at=$1, retries=retries+1 WHERE id=$2 AND status='dispatched'`,
				time.Now().Add(backoff), t.taskID); err != nil {
				log.Error().Err(err).Str("task_id", t.taskID).Msg("reconciler: failed to requeue task")
			}
		}

		if t.agentID != "" {
			if _, err := tx.Exec(ctx, `UPDATE compute_nodes SET reserved_vcpu=GREATEST(0,reserved_vcpu-$1), reserved_ram_mb=GREATEST(0,reserved_ram_mb-$2), reserved_disk_gb=GREATEST(0,reserved_disk_gb-$3) WHERE id=$4`,
				t.reqVcpu, t.reqRam, t.reqDisk, t.agentID); err != nil {
				log.Error().Err(err).Str("agent_id", t.agentID).Msg("reconciler: failed to release compute node resources")
			}
		}

		action := map[bool]string{true: "failed", false: "requeued"}[t.retries >= maxTaskRetries]
		fmt.Printf("reconciler: task %s retries=%d action=%s\n", t.taskID, t.retries, action)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("reconciler: failed to commit reconcile transaction")
	}
}
