-- SCS-0300-v1 SSO identity federation.
--
-- Two tables back the federated-identity feature:
--
--   federation_providers      — runtime registry of configured IdPs. Mirrors
--                               the keystone.federation.providers YAML block
--                               so that operators can also list/inspect IdPs
--                               via SQL and (in a later slice) via an admin
--                               API. The YAML config is authoritative on
--                               startup; rows here are kept in sync.
--
--   federation_role_mappings  — claim-to-role mapping rules. Translate IdP
--                               claims (typically `groups` membership) into
--                               O3K role assignments scoped to a project.
--                               Evaluated in priority order on each
--                               federated login; a match grants the role on
--                               the named project for the duration of the
--                               issued token.
--
-- A federated user record itself does NOT live here — JIT provisioning
-- writes a normal row into `users` with a deterministic UUID derived from
-- (issuer, subject), so existing user-lookup code paths work unchanged.

CREATE TABLE IF NOT EXISTS federation_providers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) UNIQUE NOT NULL,
    protocol        VARCHAR(32) NOT NULL,
    issuer          TEXT NOT NULL,
    client_id       TEXT,
    -- client_secret intentionally NOT stored here — secrets stay in YAML
    -- config (or env) so a DB dump never leaks IdP credentials.
    auto_provision  BOOLEAN NOT NULL DEFAULT true,
    username_claim  VARCHAR(64) NOT NULL DEFAULT 'preferred_username',
    groups_claim    VARCHAR(64) NOT NULL DEFAULT 'groups',
    default_project VARCHAR(255),
    default_role    VARCHAR(255),
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_federation_providers_name
    ON federation_providers(name);

CREATE TABLE IF NOT EXISTS federation_role_mappings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES federation_providers(id) ON DELETE CASCADE,
    -- claim_name names the OIDC claim to inspect (typically "groups").
    claim_name  VARCHAR(64) NOT NULL,
    -- claim_value is the value within that claim that triggers the rule.
    -- For array claims (groups), match if the value is present in the array.
    -- For scalar claims, match on equality.
    claim_value VARCHAR(255) NOT NULL,
    role_id     UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    -- project_id is optional — when NULL, the role is granted on the user's
    -- default project (from federation_providers.default_project) or the
    -- project named in the auth scope.
    project_id  UUID REFERENCES projects(id) ON DELETE CASCADE,
    -- priority orders rules within a provider. Lower numbers evaluate first.
    -- All matching rules apply (a user with multiple matching groups gets
    -- multiple roles); priority only affects evaluation order for logging
    -- and conflict resolution.
    priority    INTEGER NOT NULL DEFAULT 100,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (provider_id, claim_name, claim_value, role_id, project_id)
);

CREATE INDEX IF NOT EXISTS idx_federation_role_mappings_provider
    ON federation_role_mappings(provider_id);

CREATE INDEX IF NOT EXISTS idx_federation_role_mappings_lookup
    ON federation_role_mappings(provider_id, claim_name, claim_value);
