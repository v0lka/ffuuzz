ALTER TABLE findings ADD COLUMN IF NOT EXISTS mutation_ops TEXT[];
ALTER TABLE findings ADD COLUMN IF NOT EXISTS regex_patterns TEXT[];
