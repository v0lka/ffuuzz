CREATE TABLE IF NOT EXISTS recordings (
    id UUID PRIMARY KEY,
    schema_version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL,
    target_scheme TEXT NOT NULL,
    target_host TEXT NOT NULL,
    target_port INT NOT NULL,
    entry_count INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS exchanges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recording_id UUID NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
    request_id TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    duration_ms INT NOT NULL,
    req_method TEXT NOT NULL,
    req_path TEXT NOT NULL,
    req_query TEXT NOT NULL DEFAULT '',
    req_headers JSONB,
    req_body_b64 TEXT NOT NULL DEFAULT '',
    req_body_truncated BOOL NOT NULL DEFAULT FALSE,
    resp_status INT NOT NULL,
    resp_headers JSONB,
    resp_body_b64 TEXT NOT NULL DEFAULT '',
    resp_body_truncated BOOL NOT NULL DEFAULT FALSE,
    seq_order INT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_exchanges_recording ON exchanges(recording_id);

CREATE TABLE IF NOT EXISTS campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'CREATED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    config JSONB NOT NULL DEFAULT '{}',
    tests_done INT NOT NULL DEFAULT 0,
    findings_total INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS campaign_recordings (
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    recording_id UUID NOT NULL REFERENCES recordings(id),
    PRIMARY KEY (campaign_id, recording_id)
);

CREATE TABLE IF NOT EXISTS findings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id),
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'UNCONFIRMED',
    signature TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ,
    method TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    details JSONB,
    seed_recording_id UUID REFERENCES recordings(id),
    minimized BOOL NOT NULL DEFAULT FALSE,
    reproduce_status TEXT,
    reproduce_enqueued_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_findings_campaign ON findings(campaign_id);
CREATE INDEX IF NOT EXISTS idx_findings_signature ON findings(signature);

CREATE TABLE IF NOT EXISTS artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    finding_id UUID NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    size_bytes BIGINT NOT NULL DEFAULT 0
);
