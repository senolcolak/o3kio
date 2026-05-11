PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS services (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(type, name)
);

CREATE TABLE IF NOT EXISTS endpoints (
    id TEXT PRIMARY KEY,
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    interface TEXT NOT NULL,
    url TEXT NOT NULL,
    region TEXT,
    enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_endpoints_service ON endpoints(service_id);
CREATE INDEX IF NOT EXISTS idx_endpoints_interface ON endpoints(interface);

-- Seed default services
INSERT INTO services (id, type, name, description, enabled) VALUES
    ('00000000-0000-0000-0000-000000000010', 'identity', 'keystone', 'Identity Service', 1),
    ('00000000-0000-0000-0000-000000000011', 'compute', 'nova', 'Compute Service', 1),
    ('00000000-0000-0000-0000-000000000012', 'network', 'neutron', 'Network Service', 1),
    ('00000000-0000-0000-0000-000000000013', 'volumev3', 'cinderv3', 'Block Storage Service v3', 1),
    ('00000000-0000-0000-0000-000000000014', 'image', 'glance', 'Image Service', 1)
ON CONFLICT DO NOTHING;

-- Seed default endpoints
INSERT INTO endpoints (service_id, interface, url, region) VALUES
    ('00000000-0000-0000-0000-000000000010', 'public', 'http://localhost:35357/v3', 'RegionOne'),
    ('00000000-0000-0000-0000-000000000011', 'public', 'http://localhost:8774/v2.1', 'RegionOne'),
    ('00000000-0000-0000-0000-000000000012', 'public', 'http://localhost:9696/v2.0', 'RegionOne'),
    ('00000000-0000-0000-0000-000000000013', 'public', 'http://localhost:8776/v3', 'RegionOne'),
    ('00000000-0000-0000-0000-000000000014', 'public', 'http://localhost:9292', 'RegionOne')
ON CONFLICT DO NOTHING;
