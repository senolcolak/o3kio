PRAGMA foreign_keys = ON;

-- Add domains table for Keystone v3
CREATE TABLE IF NOT EXISTS domains (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Add domain_id to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS domain_id TEXT REFERENCES domains(id) ON DELETE CASCADE;

-- Add domain_id to projects table
ALTER TABLE projects ADD COLUMN IF NOT EXISTS domain_id TEXT REFERENCES domains(id) ON DELETE CASCADE;

-- Create indexes for domain lookups
CREATE INDEX IF NOT EXISTS idx_users_domain_id ON users(domain_id);
CREATE INDEX IF NOT EXISTS idx_projects_domain_id ON projects(domain_id);
CREATE INDEX IF NOT EXISTS idx_users_domain_name ON users(domain_id, name);
CREATE INDEX IF NOT EXISTS idx_projects_domain_name ON projects(domain_id, name);
