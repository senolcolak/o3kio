-- Add access_rules JSONB column for fine-grained request restriction
ALTER TABLE application_credentials
ADD COLUMN IF NOT EXISTS access_rules JSONB DEFAULT NULL;

-- GIN index for access_rules queries
CREATE INDEX IF NOT EXISTS idx_application_credentials_access_rules
ON application_credentials USING GIN (access_rules)
WHERE access_rules IS NOT NULL;
