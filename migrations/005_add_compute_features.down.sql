-- Migration 005: Rollback - Remove volume attachments and advanced compute features

DROP TABLE IF EXISTS interface_attachments CASCADE;
DROP TABLE IF EXISTS instance_snapshots CASCADE;
DROP TABLE IF EXISTS instance_userdata CASCADE;
DROP TABLE IF EXISTS instance_metadata CASCADE;
DROP TABLE IF EXISTS volume_attachments CASCADE;

ALTER TABLE instances DROP COLUMN IF EXISTS console_vnc_port;
ALTER TABLE instances DROP COLUMN IF EXISTS console_vnc_password;
ALTER TABLE instances DROP COLUMN IF EXISTS task_state;
