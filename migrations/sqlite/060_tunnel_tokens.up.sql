CREATE TABLE IF NOT EXISTS tunnel_tokens (
    id          TEXT PRIMARY KEY,
    token_hash  TEXT NOT NULL,
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at  TEXT,
    revoked     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_tunnel_tokens_hash ON tunnel_tokens(token_hash);
