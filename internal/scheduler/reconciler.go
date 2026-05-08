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
	defer rows.Close()

	for rows.Next() {
		var taskID, agentID, resourceID string
		var retries, timeoutSec, reqVcpu int
		var reqRam, reqDisk int64
		if err := rows.Scan(&taskID, &agentID, &resourceID, &retries, &timeoutSec, &reqVcpu, &reqRam, &reqDisk); err != nil {
			continue
		}

		if retries >= maxTaskRetries {
			if _, err := tx.Exec(ctx, `UPDATE tasks SET status='failed', error='reconciler: max retries exceeded', completed_at=now() WHERE id=$1 AND status='dispatched'`, taskID); err != nil {
				log.Error().Err(err).Str("task_id", taskID).Msg("reconciler: failed to mark task failed")
			}
			if _, err := tx.Exec(ctx, `UPDATE instances SET status='ERROR', task_state=NULL WHERE id=$1`, resourceID); err != nil {
				log.Error().Err(err).Str("resource_id", resourceID).Msg("reconciler: failed to mark instance ERROR")
			}
		} else {
			backoff := time.Duration((retries+1)*10) * time.Second
			if _, err := tx.Exec(ctx, `UPDATE tasks SET status='pending', agent_id=NULL, dispatched_at=NULL, next_retry_at=$1, retries=retries+1 WHERE id=$2 AND status='dispatched'`,
				time.Now().Add(backoff), taskID); err != nil {
				log.Error().Err(err).Str("task_id", taskID).Msg("reconciler: failed to requeue task")
			}
		}

		if agentID != "" {
			if _, err := tx.Exec(ctx, `UPDATE compute_nodes SET reserved_vcpu=GREATEST(0,reserved_vcpu-$1), reserved_ram_mb=GREATEST(0,reserved_ram_mb-$2), reserved_disk_gb=GREATEST(0,reserved_disk_gb-$3) WHERE id=$4`,
				reqVcpu, reqRam, reqDisk, agentID); err != nil {
				log.Error().Err(err).Str("agent_id", agentID).Msg("reconciler: failed to release compute node resources")
			}
		}

		action := map[bool]string{true: "failed", false: "requeued"}[retries >= maxTaskRetries]
		fmt.Printf("reconciler: task %s retries=%d action=%s\n", taskID, retries, action)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("reconciler: failed to commit reconcile transaction")
	}
}
