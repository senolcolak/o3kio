-- Rollback for 011_glance_image_members.up.sql

DROP INDEX IF EXISTS idx_image_members_member_id;
DROP INDEX IF EXISTS idx_image_members_image_id;
DROP TABLE IF EXISTS image_members;
