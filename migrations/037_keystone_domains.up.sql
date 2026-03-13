-- Add domains table
CREATE TABLE IF NOT EXISTS domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Seed default domain (using well-known UUID for 'default')
INSERT INTO domains (id, name, description, enabled, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default', 'Default domain', TRUE, NOW(), NOW())
ON CONFLICT (name) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_domains_name ON domains(name);
CREATE INDEX IF NOT EXISTS idx_domains_enabled ON domains(enabled);
