CREATE TABLE IF NOT EXISTS server_tags (
    instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    tag VARCHAR(60) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY(instance_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_server_tags_instance ON server_tags(instance_id);
CREATE INDEX IF NOT EXISTS idx_server_tags_tag ON server_tags(tag);
