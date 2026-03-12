DROP INDEX IF EXISTS idx_recordings_endpoint;
ALTER TABLE recordings DROP COLUMN IF EXISTS target_path;
