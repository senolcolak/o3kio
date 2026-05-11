-- Add image_members table for Glance image sharing
-- Part of Phase 1 Week 3-4: Glance image members implementation
-- Per Constitution: Backward compatible

CREATE TABLE IF NOT EXISTS image_members (
    id TEXT PRIMARY KEY,
    image_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(image_id, member_id)
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_image_members_image_id ON image_members(image_id);
CREATE INDEX IF NOT EXISTS idx_image_members_member_id ON image_members(member_id);

-- Note: No foreign key to images table as images may be external
-- member_id is a project_id (tenant ID) that the image is shared with
