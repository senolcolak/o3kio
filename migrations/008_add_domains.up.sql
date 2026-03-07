-- Add domains table for Keystone v3
CREATE TABLE IF NOT EXISTS domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Add domain_id to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS domain_id UUID REFERENCES domains(id) ON DELETE CASCADE;

-- Add domain_id to projects table
ALTER TABLE projects ADD COLUMN IF NOT EXISTS domain_id UUID REFERENCES domains(id) ON DELETE CASCADE;

-- Create indexes for domain lookups
CREATE INDEX IF NOT EXISTS idx_users_domain_id ON users(domain_id);
CREATE INDEX IF NOT EXISTS idx_projects_domain_id ON projects(domain_id);
CREATE INDEX IF NOT EXISTS idx_users_domain_name ON users(domain_id, name);
CREATE INDEX IF NOT EXISTS idx_projects_domain_name ON projects(domain_id, name);
