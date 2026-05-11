-- Add Neutron RBAC policies table for resource sharing
CREATE TABLE IF NOT EXISTS rbac_policies (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    target_tenant TEXT NOT NULL,
    action TEXT NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_rbac_policies_object ON rbac_policies(object_type, object_id);
CREATE INDEX IF NOT EXISTS idx_rbac_policies_target ON rbac_policies(target_tenant);
CREATE INDEX IF NOT EXISTS idx_rbac_policies_project ON rbac_policies(project_id);
