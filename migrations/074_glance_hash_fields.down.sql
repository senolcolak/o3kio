-- Rollback: remove SHA-512 hash columns from images
ALTER TABLE images DROP COLUMN IF EXISTS os_hash_algo;
ALTER TABLE images DROP COLUMN IF EXISTS os_hash_value;
