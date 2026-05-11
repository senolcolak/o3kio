PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS server_tags (
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    tag TEXT NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(instance_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_server_tags_instance ON server_tags(instance_id);
CREATE INDEX IF NOT EXISTS idx_server_tags_tag ON server_tags(tag);
