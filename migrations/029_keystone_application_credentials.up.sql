-- Add Keystone application credentials for non-expiring authentication
CREATE TABLE IF NOT EXISTS application_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID,
    secret_hash VARCHAR(255) NOT NULL,
    description TEXT,
    unrestricted BOOLEAN DEFAULT FALSE,
    expires_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(user_id, name)
);

CREATE INDEX idx_application_credentials_user ON application_credentials(user_id);
CREATE INDEX idx_application_credentials_project ON application_credentials(project_id);
