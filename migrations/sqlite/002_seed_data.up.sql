-- Insert default admin user (password: secret, bcrypt hash)
INSERT INTO users (id, name, password_hash, enabled)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'admin',
    '$2a$10$/DyBs44Yot8zgjC4j.n03Ox/QG94ftrksEXKKvadKcixLbGll0dR2', -- bcrypt hash of 'secret'
    1
) ON CONFLICT (name) DO NOTHING;

-- Insert default project
INSERT INTO projects (id, name, description, enabled)
VALUES (
    '00000000-0000-0000-0000-000000000002',
    'default',
    'Default project',
    1
) ON CONFLICT (name) DO NOTHING;

-- Insert default roles
INSERT INTO roles (id, name)
VALUES
    ('00000000-0000-0000-0000-000000000003', 'admin'),
    ('00000000-0000-0000-0000-000000000004', 'member'),
    ('00000000-0000-0000-0000-000000000005', 'reader')
ON CONFLICT (name) DO NOTHING;

-- Assign admin role to admin user on default project
INSERT INTO role_assignments (user_id, project_id, role_id)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000002',
    '00000000-0000-0000-0000-000000000003'
) ON CONFLICT (user_id, project_id, role_id) DO NOTHING;

-- Insert default flavors
INSERT INTO flavors (id, name, vcpus, ram_mb, disk_gb, is_public)
VALUES
    ('00000000-0000-0000-0000-000000000010', 'm1.tiny', 1, 512, 1, 1),
    ('00000000-0000-0000-0000-000000000011', 'm1.small', 1, 2048, 20, 1),
    ('00000000-0000-0000-0000-000000000012', 'm1.medium', 2, 4096, 40, 1),
    ('00000000-0000-0000-0000-000000000013', 'm1.large', 4, 8192, 80, 1),
    ('00000000-0000-0000-0000-000000000014', 'm1.xlarge', 8, 16384, 160, 1)
ON CONFLICT (name) DO NOTHING;

-- Insert default volume type
INSERT INTO volume_types (id, name, description, is_public)
VALUES (
    '00000000-0000-0000-0000-000000000020',
    'ceph-rbd',
    'Ceph RBD backed volumes',
    1
) ON CONFLICT (name) DO NOTHING;

-- Insert default security group for default project
INSERT INTO security_groups (id, name, project_id, description)
VALUES (
    '00000000-0000-0000-0000-000000000030',
    'default',
    '00000000-0000-0000-0000-000000000002',
    'Default security group'
) ON CONFLICT (project_id, name) DO NOTHING;

-- Insert default security group rules (allow all egress, SSH ingress)
INSERT INTO security_group_rules (security_group_id, direction, ethertype, protocol, port_range_min, port_range_max)
VALUES
    ('00000000-0000-0000-0000-000000000030', 'egress', 'IPv4', NULL, NULL, NULL),
    ('00000000-0000-0000-0000-000000000030', 'egress', 'IPv6', NULL, NULL, NULL),
    ('00000000-0000-0000-0000-000000000030', 'ingress', 'IPv4', 'tcp', 22, 22)
ON CONFLICT DO NOTHING;
