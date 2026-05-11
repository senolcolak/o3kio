-- Add legacy_auth flag for backward compatibility with pre-bcrypt credentials
ALTER TABLE application_credentials
ADD COLUMN IF NOT EXISTS legacy_auth INTEGER DEFAULT 0;

ALTER TABLE application_credentials
ADD COLUMN IF NOT EXISTS updated_at TEXT DEFAULT CURRENT_TIMESTAMP;

-- Mark only truly legacy credentials (not already bcrypt-hashed)
UPDATE application_credentials
SET legacy_auth = 1
WHERE secret_hash NOT LIKE '$2a$%'
  AND secret_hash NOT LIKE '$2b$%';
