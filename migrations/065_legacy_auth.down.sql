ALTER TABLE application_credentials
DROP COLUMN IF EXISTS legacy_auth,
DROP COLUMN IF EXISTS updated_at;
