-- Seed default domain
INSERT INTO domains (id, name, description, enabled)
VALUES (
    '00000000-0000-0000-0000-000000000100',
    'Default',
    'Default domain',
    true
) ON CONFLICT (name) DO NOTHING;

-- Update existing users to belong to Default domain
UPDATE users SET domain_id = '00000000-0000-0000-0000-000000000100' WHERE domain_id IS NULL;

-- Update existing projects to belong to Default domain
UPDATE projects SET domain_id = '00000000-0000-0000-0000-000000000100' WHERE domain_id IS NULL;
