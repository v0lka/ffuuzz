DROP INDEX IF EXISTS idx_findings_group_id;
ALTER TABLE findings DROP COLUMN IF EXISTS reproducibility;
ALTER TABLE findings DROP COLUMN IF EXISTS group_id;
ALTER TABLE findings DROP COLUMN IF EXISTS owasp_category;
ALTER TABLE findings DROP COLUMN IF EXISTS severity;
