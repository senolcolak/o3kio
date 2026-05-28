-- Reverse SCS-0300-v1 federation tables (sqlite mirror).
DROP INDEX IF EXISTS idx_federation_role_mappings_lookup;
DROP INDEX IF EXISTS idx_federation_role_mappings_provider;
DROP TABLE IF EXISTS federation_role_mappings;

DROP INDEX IF EXISTS idx_federation_providers_name;
DROP TABLE IF EXISTS federation_providers;
