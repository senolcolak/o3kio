CREATE TABLE IF NOT EXISTS revoked_tokens (
    token_hash TEXT PRIMARY KEY,
    expires_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_revoked_tokens_expires ON revoked_tokens (expires_at);
