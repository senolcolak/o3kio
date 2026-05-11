ALTER TABLE images ADD COLUMN IF NOT EXISTS cached_at TEXT;

CREATE INDEX IF NOT EXISTS idx_images_cached_at ON images(cached_at);
