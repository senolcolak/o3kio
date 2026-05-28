-- SCS-0300-v1 SSO identity federation (sqlite mirror).
-- See migrations/077_federation_identity.up.sql for full design notes.
--
-- Type mapping vs PostgreSQL:
--   UUID            -> TEXT (application supplies UUIDs on INSERT)
--   BOOLEAN         -> INTEGER (1 = true, 0 = false)
--   gen_random_uuid -> removed (application generates)

CREATE TABLE IF NOT EXISTS federation_providers (
    id              TEXT PRIMARY KEY,
    name            TEXT UNIQUE NOT NULL,
    protocol        TEXT NOT NULL,
    issuer          TEXT NOT NULL,
    client_id       TEXT,
    auto_provision  INTEGER NOT NULL DEFAULT 1,
    username_claim  TEXT NOT NULL DEFAULT 'preferred_username',
    groups_claim    TEXT NOT NULL DEFAULT 'groups',
    default_project TEXT,
    default_role    TEXT,
    created_at      TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at      TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_federation_providers_name
    ON federation_providers(name);

CREATE TABLE IF NOT EXISTS federation_role_mappings (
    id          TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL REFERENCES federation_providers(id) ON DELETE CASCADE,
    claim_name  TEXT NOT NULL,
    claim_value TEXT NOT NULL,
    role_id     TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    project_id  TEXT REFERENCES projects(id) ON DELETE CASCADE,
    priority    INTEGER NOT NULL DEFAULT 100,
    created_at  TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (provider_id, claim_name, claim_value, role_id, project_id)
);

CREATE INDEX IF NOT EXISTS idx_federation_role_mappings_provider
    ON federation_role_mappings(provider_id);

CREATE INDEX IF NOT EXISTS idx_federation_role_mappings_lookup
    ON federation_role_mappings(provider_id, claim_name, claim_value);
