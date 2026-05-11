-- Add SHA-512 hash columns to images for OpenStack Glance API v2 compliance
ALTER TABLE images ADD COLUMN IF NOT EXISTS os_hash_algo TEXT;
ALTER TABLE images ADD COLUMN IF NOT EXISTS os_hash_value TEXT;
