-- Seed test-sg security group for contract tests
INSERT INTO security_groups (
    id,
    name,
    description,
    project_id,
    created_at,
    updated_at
) VALUES (
    '00000000-0000-0000-0000-000000000099',
    'test-sg',
    'Test security group for contract tests',
    '00000000-0000-0000-0000-000000000002',  -- Default project
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
) ON CONFLICT (id) DO NOTHING;
