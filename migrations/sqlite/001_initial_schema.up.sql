PRAGMA foreign_keys = ON;

-- Keystone tables
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS roles (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS role_assignments (
    id TEXT PRIMARY KEY,
    user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    role_id TEXT REFERENCES roles(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, project_id, role_id)
);

-- Nova tables
CREATE TABLE IF NOT EXISTS flavors (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    vcpus INTEGER NOT NULL,
    ram_mb INTEGER NOT NULL,
    disk_gb INTEGER NOT NULL,
    is_public INTEGER DEFAULT 1,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS instances (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    flavor_id TEXT REFERENCES flavors(id),
    image_id TEXT,
    status TEXT NOT NULL DEFAULT 'BUILD',
    power_state INTEGER DEFAULT 0,
    libvirt_domain_id TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    launched_at TEXT,
    terminated_at TEXT
);

CREATE TABLE IF NOT EXISTS keypairs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    public_key TEXT NOT NULL,
    fingerprint TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name)
);

-- Neutron tables
CREATE TABLE IF NOT EXISTS networks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    admin_state_up INTEGER DEFAULT 1,
    status TEXT DEFAULT 'ACTIVE',
    shared INTEGER DEFAULT 0,
    mtu INTEGER DEFAULT 1500,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS subnets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    network_id TEXT REFERENCES networks(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    cidr TEXT NOT NULL,
    gateway_ip TEXT,
    ip_version INTEGER DEFAULT 4,
    enable_dhcp INTEGER DEFAULT 1,
    dns_nameservers TEXT DEFAULT '[]',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ports (
    id TEXT PRIMARY KEY,
    name TEXT,
    network_id TEXT REFERENCES networks(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    device_id TEXT,
    device_owner TEXT,
    mac_address TEXT,
    admin_state_up INTEGER DEFAULT 1,
    status TEXT DEFAULT 'DOWN',
    fixed_ips TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS security_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    description TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, name)
);

CREATE TABLE IF NOT EXISTS security_group_rules (
    id TEXT PRIMARY KEY,
    security_group_id TEXT REFERENCES security_groups(id) ON DELETE CASCADE,
    direction TEXT NOT NULL, -- 'ingress' or 'egress'
    ethertype TEXT DEFAULT 'IPv4',
    protocol TEXT, -- 'tcp', 'udp', 'icmp', or NULL for all
    port_range_min INTEGER,
    port_range_max INTEGER,
    remote_ip_prefix TEXT,
    remote_group_id TEXT REFERENCES security_groups(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Cinder tables
CREATE TABLE IF NOT EXISTS volumes (
    id TEXT PRIMARY KEY,
    name TEXT,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    size_gb INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'creating',
    bootable INTEGER DEFAULT 0,
    attached_to_instance_id TEXT REFERENCES instances(id) ON DELETE SET NULL,
    rbd_pool TEXT,
    rbd_image TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS volume_types (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    is_public INTEGER DEFAULT 1,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS snapshots (
    id TEXT PRIMARY KEY,
    name TEXT,
    volume_id TEXT REFERENCES volumes(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
    size_gb INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'creating',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Glance tables
CREATE TABLE IF NOT EXISTS images (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    visibility TEXT DEFAULT 'private', -- 'public', 'private', 'shared', 'community'
    size_bytes INTEGER,
    disk_format TEXT, -- 'qcow2', 'raw', 'iso', etc.
    container_format TEXT, -- 'bare', 'ovf', etc.
    min_disk_gb INTEGER DEFAULT 0,
    min_ram_mb INTEGER DEFAULT 0,
    checksum TEXT,
    rbd_pool TEXT,
    rbd_image TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_instances_project_id ON instances(project_id);
CREATE INDEX IF NOT EXISTS idx_instances_status ON instances(status);
CREATE INDEX IF NOT EXISTS idx_networks_project_id ON networks(project_id);
CREATE INDEX IF NOT EXISTS idx_ports_network_id ON ports(network_id);
CREATE INDEX IF NOT EXISTS idx_ports_device_id ON ports(device_id);
CREATE INDEX IF NOT EXISTS idx_volumes_project_id ON volumes(project_id);
CREATE INDEX IF NOT EXISTS idx_volumes_attached_to ON volumes(attached_to_instance_id);
CREATE INDEX IF NOT EXISTS idx_images_project_id ON images(project_id);
CREATE INDEX IF NOT EXISTS idx_images_visibility ON images(visibility);
CREATE INDEX IF NOT EXISTS idx_role_assignments_user_project ON role_assignments(user_id, project_id);
