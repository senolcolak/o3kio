ALTER TABLE images ADD COLUMN IF NOT EXISTS cached_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_images_cached_at ON images(cached_at) WHERE cached_at IS NOT NULL;
