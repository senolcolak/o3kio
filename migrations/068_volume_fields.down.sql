ALTER TABLE volumes
DROP COLUMN IF EXISTS availability_zone,
DROP COLUMN IF EXISTS encrypted;
