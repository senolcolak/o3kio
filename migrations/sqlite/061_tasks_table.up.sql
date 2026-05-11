PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS tasks (
  id              TEXT PRIMARY KEY,
  type            TEXT NOT NULL,
  resource_id     TEXT NOT NULL,
  project_id      TEXT NOT NULL,
  agent_id        TEXT REFERENCES compute_nodes(id) ON DELETE SET NULL,
  payload         TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'dispatched', 'completed', 'failed')),
  retries         INTEGER NOT NULL DEFAULT 0 CHECK (retries <= 3),
  timeout_sec     INTEGER NOT NULL DEFAULT 120 CHECK (timeout_sec > 0),
  req_vcpu        INTEGER NOT NULL DEFAULT 0,
  req_ram_mb      INTEGER NOT NULL DEFAULT 0,
  req_disk_gb     INTEGER NOT NULL DEFAULT 0,
  next_retry_at   TEXT,
  idempotency_key TEXT,
  error           TEXT,
  error_history   TEXT NOT NULL DEFAULT '[]',
  created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  dispatched_at   TEXT,
  completed_at    TEXT,

  CONSTRAINT chk_agent_only_when_dispatched
    CHECK (agent_id IS NULL OR status IN ('dispatched', 'completed', 'failed')),
  CONSTRAINT chk_dispatched_has_timestamp
    CHECK (status != 'dispatched' OR dispatched_at IS NOT NULL),
  CONSTRAINT chk_completed_has_timestamp
    CHECK (status != 'completed' OR completed_at IS NOT NULL),
  CONSTRAINT uq_idempotency_per_project
    UNIQUE (project_id, idempotency_key)
);

CREATE INDEX idx_tasks_pending_retry ON tasks (next_retry_at);
CREATE INDEX idx_tasks_dispatched_timeout ON tasks (dispatched_at);
CREATE INDEX idx_tasks_resource_id ON tasks (resource_id);
