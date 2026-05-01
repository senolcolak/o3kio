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
