-- Restore the original single-column signature index
CREATE INDEX IF NOT EXISTS idx_findings_signature ON findings(signature);

-- Remove the composite index
DROP INDEX IF EXISTS idx_findings_campaign_signature;
