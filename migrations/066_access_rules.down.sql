DROP INDEX IF EXISTS idx_application_credentials_access_rules;
ALTER TABLE application_credentials DROP COLUMN IF EXISTS access_rules;
