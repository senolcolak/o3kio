-- Add access_rules TEXT column for fine-grained request restriction
ALTER TABLE application_credentials
ADD COLUMN IF NOT EXISTS access_rules TEXT DEFAULT NULL;

-- Index for access_rules queries (partial index converted to regular index)
CREATE INDEX IF NOT EXISTS idx_application_credentials_access_rules
ON application_credentials (access_rules);
