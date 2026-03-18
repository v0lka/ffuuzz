-- Add composite index for ExistsBySignature query (campaign_id, signature)
CREATE INDEX IF NOT EXISTS idx_findings_campaign_signature ON findings(campaign_id, signature);

-- Remove unused single-column signature index (no queries filter by signature alone)
DROP INDEX IF EXISTS idx_findings_signature;
