-- Add server migrations tracking table
CREATE TABLE IF NOT EXISTS server_migrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_uuid UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    source_node VARCHAR(255),
    dest_node VARCHAR(255),
    old_flavor_id UUID,
    new_flavor_id UUID,
    status VARCHAR(50) DEFAULT 'queued',
    migration_type VARCHAR(50) DEFAULT 'migration',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_server_migrations_server ON server_migrations(server_uuid);
CREATE INDEX idx_server_migrations_status ON server_migrations(status);
