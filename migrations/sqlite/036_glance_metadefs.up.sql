PRAGMA foreign_keys = ON;

-- Add metadef namespaces table
CREATE TABLE IF NOT EXISTS metadef_namespaces (
    namespace TEXT PRIMARY KEY,
    display_name TEXT,
    description TEXT,
    visibility TEXT DEFAULT 'public',
    protected INTEGER DEFAULT 0,
    owner TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS metadef_resource_types (
    id TEXT PRIMARY KEY,
    namespace TEXT NOT NULL REFERENCES metadef_namespaces(namespace) ON DELETE CASCADE,
    name TEXT NOT NULL,
    prefix TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(namespace, name)
);

CREATE INDEX idx_metadef_namespaces_visibility ON metadef_namespaces(visibility);
CREATE INDEX idx_metadef_resource_types_namespace ON metadef_resource_types(namespace);
