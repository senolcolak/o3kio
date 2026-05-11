PRAGMA foreign_keys = ON;

-- Create image_tags table for tagging images
CREATE TABLE IF NOT EXISTS image_tags (
    id TEXT PRIMARY KEY,
    image_id TEXT NOT NULL REFERENCES images(id) ON DELETE CASCADE,
    tag TEXT NOT NULL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(image_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_image_tags_image_id ON image_tags(image_id);
CREATE INDEX IF NOT EXISTS idx_image_tags_tag ON image_tags(tag);
