// TypeScript types mirroring Go models in internal/model/model.go

export type CampaignStatus =
    | "CREATED"
    | "STARTING"
    | "RUNNING"
    | "STOPPING"
    | "STOPPED"
    | "FINISHED"
    | "FAILED";

export type FindingType =
    | "TIMEOUT"
    | "SERVER_ERROR"
    | "LATENCY_REGRESSION"
    | "REGEX_MATCH";

export type FindingStatus = "UNCONFIRMED" | "CONFIRMED";

export interface TargetInfo {
    scheme: string;
    host: string;
    port: number;
    path: string;
}

export interface RequestData {
    method: string;
    path: string;
    query?: string;
    headers?: Record<string, string[]>;
    body_b64?: string;
    body_truncated: boolean;
}

export interface ResponseData {
    status: number;
    headers?: Record<string, string[]>;
    body_b64?: string;
    body_truncated: boolean;
}

export interface Exchange {
    request_id: string;
    started_at: string;
    duration_ms: number;
    request: RequestData;
    response: ResponseData;
}

export interface RecordingSession {
    schema_version: number;
    id: string;
    created_at: string;
    target: TargetInfo;
    entries?: Exchange[];
    entry_count?: number;
}

export interface TargetURL {
    base_url?: string;
}

export interface CampaignLimits {
    workers: number;
    rps: number;
    max_tests: number;
    duration_sec: number;
    req_timeout_ms: number;
}

export interface MutationConfig {
    path_query: boolean;
    headers: boolean;
    json_body: boolean;
    params: boolean;
    sequence: boolean;
    intensity: number;
    // Per-category operator filters. Nil/empty means all operators enabled.
    uri_operators?: string[];
    header_operators?: string[];
    json_operators?: string[];
    param_operators?: string[];
    primitive_operators?: string[];
    sequence_operators?: string[];
}

export interface AnomalyConfig {
    detect_5xx: boolean;
    latency_multiplier: number;
    regex_patterns?: string[];
}

export interface TriageConfig {
    confirm_runs: number;
    enable_minimization: boolean;
}

export interface CampaignConfig {
    target: TargetURL;
    limits: CampaignLimits;
    mutations: MutationConfig;
    anomaly: AnomalyConfig;
    triage: TriageConfig;
}

export interface CampaignProgress {
    tests_done: number;
    findings_total: number;
}

export interface Campaign {
    id: string;
    name: string;
    status: CampaignStatus;
    created_at: string;
    updated_at: string;
    started_at?: string;
    finished_at?: string;
    recording_ids?: string[];
    progress?: CampaignProgress;
}

export interface SeedStats {
    sessions_total: number;
    sessions_used: number;
    exchanges_sent: number;
}

export interface CampaignStats {
    campaign_id: string;
    status: CampaignStatus;
    tests_total: number;
    tests_per_sec: number;
    timeouts: number;
    server_errors: number;
    latency_regressions: number;
    regex_matches: number;
    last_activity_at: string;
    seeds: SeedStats;
}

export interface FindingDetails {
    baseline_ms?: number;
    observed_ms?: number;
    timeout_ms?: number;
    http_status?: number;
}

export interface LLMAnalysis {
    classification: string;
    severity: string;
    confidence: number;
    exploitability: string;
    remediation: string;
    description: string;
    analyzed_at: string;
    model_used: string;
}

export interface Finding {
    id: string;
    campaign_id: string;
    type: FindingType;
    status: FindingStatus;
    signature: string;
    created_at: string;
    confirmed_at?: string;
    method: string;
    endpoint: string;
    details: FindingDetails;
    seed_recording_id?: string;
    artifact_id?: string;
    minimized: boolean;
    reproduce_status?: string;
    reproduce_enqueued_at?: string;
    mutation_type?: string;
    mutation_payload?: string;
    llm_analysis?: LLMAnalysis;
}

export interface FailureCriterion {
    type: FindingType;
    timeout_ms?: number;
}

export interface ArtifactPayload {
    finding_id: string;
    campaign_id: string;
    target: TargetURL;
    failure_criterion: FailureCriterion;
    session: RecordingSession;
    mutation_seed: number;
    mutation_ops?: string[];
}

export interface APIError {
    error: string;
    message: string;
    request_id: string;
}

export interface ImportResult {
    imported: number;
    skipped: number;
    failed: number;
    total: number;
    session_ids: string[];
    skipped_session_ids?: string[];
    errors?: string[];
}

export interface ReproduceResponse {
    finding_id: string;
    reproduce_status: string;
    runs: number;
    enqueued_at: string;
}

export interface HealthResponse {
    status: string;
    db: string;
    version?: string;
    time: string;
}

export interface CreateCampaignRequest {
    name: string;
    recording_ids: string[];
    config: CampaignConfig;
}

export interface UpdateCampaignRequest {
    name?: string;
    recording_ids?: string[];
    config?: CampaignConfig;
}

export interface TreePathNode {
    segment: string;
    full_path: string;
    recording_count: number;
    children: TreePathNode[];
}

export interface TreeOrigin {
    origin: string;
    scheme: string;
    host: string;
    port: number;
    recording_count: number;
    paths: TreePathNode[];
}

export interface DeleteByPrefixResponse {
    deleted: number;
}

export interface AddRecordingsResponse {
    added: number;
}

// --- Application configuration types (mirrors internal/config/config.go) ---

export interface TLSConfigResponse {
    min_version: "1.2" | "1.3";
    handshake_timeout: string;
    disable_session_tickets: boolean;
}

export interface CertCacheConfigResponse {
    max_entries: number;
    memory_only: boolean;
    cert_dir: string;
}

export interface LLMConfigResponse {
    enabled: boolean;
    provider: string;
    api_key: string;       // masked as "••••••••" if set
    base_url: string;
    model: string;
    max_tokens: number;
    timeout: string;
}

export interface ConfigResponse {
    api_address: string;
    proxy_address: string;
    database_uri: string;
    artifact_dir: string;
    req_timeout: string;
    shutdown_timeout: string;
    workers: number;
    rps: number;
    max_body_bytes: number;
    tls_skip_verify: boolean;
    tls: TLSConfigResponse;
    cert_cache: CertCacheConfigResponse;
    llm: LLMConfigResponse;
}

export interface ConfigUpdateRequest {
    api_address?: string;
    proxy_address?: string;
    database_uri?: string;
    artifact_dir?: string;
    req_timeout?: string;
    shutdown_timeout?: string;
    workers?: number;
    rps?: number;
    max_body_bytes?: number;
    tls_skip_verify?: boolean;
    tls?: Partial<TLSConfigResponse>;
    cert_cache?: Partial<CertCacheConfigResponse>;
    llm?: Partial<LLMConfigResponse>;
}

export interface ConfigSaveResponse {
    message: string;
}

export interface FieldError {
    field: string;
    message: string;
}

export interface ConfigValidationError {
    error: string;
    message: string;
    request_id: string;
    fields?: FieldError[];
}
