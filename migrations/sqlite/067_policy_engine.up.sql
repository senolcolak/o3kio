CREATE TABLE IF NOT EXISTS keystone_policies (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL DEFAULT 'application/json',
    blob TEXT NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Default policy rules
INSERT INTO keystone_policies (id, type, blob) VALUES (
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))),
    'application/json',
    '{
        "admin_required": "role:admin",
        "owner": "user_id:%(target.user_id)s",
        "admin_or_owner": "rule:admin_required or rule:owner",
        "compute:create": "role:member or role:admin",
        "compute:get": "rule:admin_or_owner",
        "compute:delete": "rule:admin_or_owner",
        "network:create_network": "role:member or role:admin",
        "network:delete_network": "rule:admin_or_owner",
        "volume:create": "role:member or role:admin",
        "volume:delete": "rule:admin_or_owner",
        "image:upload_image": "role:member or role:admin",
        "image:delete_image": "rule:admin_or_owner"
    }'
);
