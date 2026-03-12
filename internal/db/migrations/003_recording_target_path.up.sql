-- Clean existing data (user confirmed not important)
DELETE FROM campaign_recordings;
UPDATE findings SET seed_recording_id = NULL;
DELETE FROM recordings;  -- cascades to exchanges

-- Add path column
ALTER TABLE recordings ADD COLUMN target_path TEXT NOT NULL DEFAULT '';

-- Unique constraint for find-or-create by endpoint
CREATE UNIQUE INDEX idx_recordings_endpoint
  ON recordings(target_scheme, target_host, target_port, target_path);
