// Package model defines the core domain types shared across all ffuuzz packages.
package model

import "time"

// TargetInfo describes the scheme, host, port, and path of a recorded target.
type TargetInfo struct {
	Scheme string `json:"scheme" db:"target_scheme"`
	Host   string `json:"host" db:"target_host"`
	Port   int    `json:"port" db:"target_port"`
	Path   string `json:"path" db:"target_path"`
}

// RecordingSession is a recorded HTTP session containing one or more exchanges.
type RecordingSession struct {
	SchemaVersion int        `json:"schema_version" db:"schema_version"`
	ID            string     `json:"id" db:"id"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	Target        TargetInfo `json:"target"`
	Entries       []Exchange `json:"entries,omitempty"`
	EntryCount    int        `json:"entry_count,omitempty" db:"entry_count"`
}

// RequestData holds the captured HTTP request fields.
type RequestData struct {
	Method        string              `json:"method"`
	Path          string              `json:"path"`
	Query         string              `json:"query,omitempty"`
	Headers       map[string][]string `json:"headers,omitempty"`
	BodyB64       string              `json:"body_b64,omitempty"`
	BodyTruncated bool                `json:"body_truncated"`
}

// ResponseData holds the captured HTTP response fields.
type ResponseData struct {
	Status        int                 `json:"status"`
	Headers       map[string][]string `json:"headers,omitempty"`
	BodyB64       string              `json:"body_b64,omitempty"`
	BodyTruncated bool                `json:"body_truncated"`
}

// Exchange is a single request/response pair within a recording session.
type Exchange struct {
	RequestID  string       `json:"request_id"`
	StartedAt  time.Time    `json:"started_at"`
	DurationMs int64        `json:"duration_ms"`
	Request    RequestData  `json:"request"`
	Response   ResponseData `json:"response"`
}

// CampaignStatus represents the lifecycle state of a fuzzing campaign.
type CampaignStatus string

const (
	CampaignCreated  CampaignStatus = "CREATED"
	CampaignStarting CampaignStatus = "STARTING"
	CampaignRunning  CampaignStatus = "RUNNING"
	CampaignStopping CampaignStatus = "STOPPING"
	CampaignStopped  CampaignStatus = "STOPPED"
	CampaignFinished CampaignStatus = "FINISHED"
	CampaignFailed   CampaignStatus = "FAILED"
)

// TargetURL holds the base URL for a fuzzing target.
type TargetURL struct {
	BaseURL string `json:"base_url"`
}

// CampaignLimits defines resource and duration constraints for a campaign.
type CampaignLimits struct {
	Workers      int   `json:"workers"`
	RPS          int   `json:"rps"`
	MaxTests     int   `json:"max_tests"`
	DurationSec  int   `json:"duration_sec"`
	ReqTimeoutMs int64 `json:"req_timeout_ms"`
}

// MutationConfig controls which mutation strategies are enabled and their intensity.
// Operator lists (URI, Header, JSON, Param, Primitive, Sequence) allow fine-grained
// control over individual mutation operators within each category. When nil or empty,
// all operators in that category are enabled.
type MutationConfig struct {
	PathQuery   bool    `json:"path_query"`
	Headers     bool    `json:"headers"`
	JSONBody    bool    `json:"json_body"`
	Params      bool    `json:"params"`
	Sequence    bool    `json:"sequence"`
	Intensity   float64 `json:"intensity"`

	// Per-category operator filters. Only listed operators are enabled.
	// Nil/empty means all operators in that category are enabled.
	URI       []string `json:"uri_operators,omitempty"`
	Header    []string `json:"header_operators,omitempty"`
	JSON      []string `json:"json_operators,omitempty"`
	Param     []string `json:"param_operators,omitempty"`
	Primitive []string `json:"primitive_operators,omitempty"`
	Seq       []string `json:"sequence_operators,omitempty"`
}

// AnomalyConfig controls which anomaly detectors are active.
type AnomalyConfig struct {
	Detect5xx         bool     `json:"detect_5xx"`
	LatencyMultiplier float64  `json:"latency_multiplier"`
	RegexPatterns     []string `json:"regex_patterns,omitempty"`
}

// TriageConfig controls finding confirmation and minimization behaviour.
type TriageConfig struct {
	ConfirmRuns        int  `json:"confirm_runs"`
	EnableMinimization bool `json:"enable_minimization"`
}

// CampaignConfig aggregates all configuration for a fuzzing campaign.
type CampaignConfig struct {
	Target          TargetURL        `json:"target"`
	Limits          CampaignLimits   `json:"limits"`
	Mutations       MutationConfig   `json:"mutations"`
	Anomaly         AnomalyConfig    `json:"anomaly"`
	Triage          TriageConfig     `json:"triage"`
	ExtractionRules []ExtractionRule `json:"extraction_rules,omitempty"`
}

// ExtractionRule defines how to extract a variable from a response for
// stateful replay (Spec 9.2: variable extraction via regex).
type ExtractionRule struct {
	Name   string `json:"name"`             // variable name for {{var}} substitution
	Source string `json:"source"`           // "body" or "header"
	Header string `json:"header,omitempty"` // header name (if Source == "header")
	Regex  string `json:"regex"`            // regex with a capture group
}

// CampaignProgress tracks the runtime progress counters for a campaign.
type CampaignProgress struct {
	TestsDone     int `json:"tests_done"`
	FindingsTotal int `json:"findings_total"`
}

// Campaign is the top-level entity representing a fuzzing campaign.
type Campaign struct {
	ID           string            `json:"id" db:"id"`
	Name         string            `json:"name" db:"name"`
	Status       CampaignStatus    `json:"status" db:"status"`
	CreatedAt    time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at" db:"updated_at"`
	StartedAt    *time.Time        `json:"started_at,omitempty" db:"started_at"`
	FinishedAt   *time.Time        `json:"finished_at,omitempty" db:"finished_at"`
	RecordingIDs []string          `json:"recording_ids,omitempty"`
	Config       CampaignConfig    `json:"-"`
	Progress     *CampaignProgress `json:"progress,omitempty"`

	// DB-only fields for JSONB storage
	ConfigJSON []byte `json:"-" db:"config"`
	TestsDone  int    `json:"-" db:"tests_done"`
	FindingsN  int    `json:"-" db:"findings_total"`
}

// FindingType classifies the kind of anomaly a finding represents.
type FindingType string

const (
	FindingTimeout           FindingType = "TIMEOUT"
	FindingServerError       FindingType = "SERVER_ERROR"
	FindingLatencyRegression FindingType = "LATENCY_REGRESSION"
	FindingRegexMatch        FindingType = "REGEX_MATCH"
)

// FindingStatus tracks whether a finding has been confirmed by triage.
type FindingStatus string

const (
	FindingUnconfirmed FindingStatus = "UNCONFIRMED"
	FindingConfirmed   FindingStatus = "CONFIRMED"
)

// ReproduceStatus tracks the state of a reproduction attempt.
type ReproduceStatus string

const (
	ReproducePending       ReproduceStatus = "PENDING"
	ReproduceEnqueued      ReproduceStatus = "ENQUEUED"
	ReproduceRunning       ReproduceStatus = "RUNNING"
	ReproduceFailed        ReproduceStatus = "FAILED"
	ReproduceNotReproduced ReproduceStatus = "NOT_REPRODUCED"
	ReproduceConfirmed     ReproduceStatus = "CONFIRMED"
)

// FindingDetails carries type-specific metrics for a finding.
type FindingDetails struct {
	BaselineMs int64 `json:"baseline_ms,omitempty"`
	ObservedMs int64 `json:"observed_ms,omitempty"`
	TimeoutMs  int64 `json:"timeout_ms,omitempty"`
	HTTPStatus int   `json:"http_status,omitempty"`

	// Regex detector context (populated for FindingRegexMatch).
	RegexPattern  string `json:"regex_pattern,omitempty"`
	MatchOffset   int    `json:"match_offset,omitempty"`
	MatchSnippet  string `json:"match_snippet,omitempty"`
	BodyTotalSize int    `json:"body_total_size,omitempty"`

	// ExchangeIndex identifies which exchange in the artifact session
	// produced this finding (0-based). The companion field on the artifact
	// payload is MatchedExchangeIndex.
	ExchangeIndex int `json:"exchange_index,omitempty"`
}

// LLMAnalysis holds the structured result of an LLM-based triage analysis.
type LLMAnalysis struct {
	// Classification is the vulnerability class, e.g. "SQL Injection" or "No Vulnerability".
	Classification string `json:"classification"`
	// Severity is the LLM's severity assessment.
	Severity Severity `json:"severity"`
	// Confidence is the LLM's certainty score from 0.0 to 1.0.
	Confidence float64 `json:"confidence"`
	// Exploitability is a free-text assessment of exploitability.
	Exploitability string `json:"exploitability"`
	// Remediation is the recommended fix.
	Remediation string `json:"remediation"`
	// Description is a natural-language summary of the finding.
	Description string `json:"description"`
	// AnalyzedAt is when the analysis was performed.
	AnalyzedAt time.Time `json:"analyzed_at"`
	// ModelUsed is the LLM model that produced this analysis, for audit trails.
	ModelUsed string `json:"model_used"`
}

// Finding is an anomaly discovered during a fuzzing campaign.
type Finding struct {
	ID                  string         `json:"id" db:"id"`
	CampaignID          string         `json:"campaign_id" db:"campaign_id"`
	Type                FindingType    `json:"type" db:"type"`
	Status              FindingStatus  `json:"status" db:"status"`
	Signature           string         `json:"signature" db:"signature"`
	CreatedAt           time.Time      `json:"created_at" db:"created_at"`
	ConfirmedAt         *time.Time     `json:"confirmed_at,omitempty" db:"confirmed_at"`
	Method              string         `json:"method" db:"method"`
	Endpoint            string         `json:"endpoint" db:"endpoint"`
	Details             FindingDetails `json:"details,omitempty"`
	SeedRecordingID     string         `json:"seed_recording_id,omitempty" db:"seed_recording_id"`
	ArtifactID          string         `json:"artifact_id,omitempty"`
	Minimized           bool           `json:"minimized" db:"minimized"`
	ReproduceStatus     string         `json:"reproduce_status,omitempty" db:"reproduce_status"`
	ReproduceEnqueuedAt *time.Time     `json:"reproduce_enqueued_at,omitempty" db:"reproduce_enqueued_at"`
	ReproduceRuns       int            `json:"reproduce_runs,omitempty" db:"reproduce_runs"`
	MutationType        string         `json:"mutation_type,omitempty" db:"mutation_type"`
	MutationPayload     string         `json:"mutation_payload,omitempty" db:"mutation_payload"`
	MutationOps         []string       `json:"mutation_ops,omitempty"`
	RegexPatterns       []string       `json:"regex_patterns,omitempty"`
	Severity            Severity       `json:"severity" db:"severity"`
	OWASPCategory       OWASPCategory  `json:"owasp_category" db:"owasp_category"`
	GroupID             *string        `json:"group_id,omitempty" db:"group_id"`
	Reproducibility     float64        `json:"reproducibility" db:"reproducibility"`

	// DB-only JSONB
	DetailsJSON []byte `json:"-" db:"details"`

	// LLM analysis (optional enrichment from AI triage)
	LLMAnalysis     *LLMAnalysis `json:"llm_analysis,omitempty"`
	LLMAnalysisJSON []byte       `json:"-" db:"llm_analysis"`
}

// Artifact references a stored file containing the full reproduction payload.
type Artifact struct {
	ID        string    `json:"id" db:"id"`
	FindingID string    `json:"finding_id" db:"finding_id"`
	FilePath  string    `json:"file_path" db:"file_path"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	SizeBytes int64     `json:"size_bytes" db:"size_bytes"`
}

// Severity represents the assessed severity of a finding.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
	SeverityInfo     Severity = "INFO"
)

// OWASPCategory maps a finding to an OWASP Top 10 2025 category.
type OWASPCategory string

const (
	OWASPCatA01BrokenAccessControl      OWASPCategory = "A01_BROKEN_ACCESS_CONTROL"
	OWASPCatA02SecurityMisconfiguration OWASPCategory = "A02_SECURITY_MISCONFIGURATION"
	OWASPCatA03SoftwareSupplyChain      OWASPCategory = "A03_SOFTWARE_SUPPLY_CHAIN"
	OWASPCatA04CryptographicFailures    OWASPCategory = "A04_CRYPTOGRAPHIC_FAILURES"
	OWASPCatA05Injection                OWASPCategory = "A05_INJECTION"
	OWASPCatA06InsecureDesign           OWASPCategory = "A06_INSECURE_DESIGN"
	OWASPCatA07AuthenticationFailures   OWASPCategory = "A07_AUTHENTICATION_FAILURES"
	OWASPCatA08SoftwareDataIntegrity    OWASPCategory = "A08_SOFTWARE_DATA_INTEGRITY"
	OWASPCatA09SecurityLoggingAlerting  OWASPCategory = "A09_SECURITY_LOGGING_ALERTING"
	OWASPCatA10ExceptionalConditions    OWASPCategory = "A10_EXCEPTIONAL_CONDITIONS"
	OWASPCatUncategorized               OWASPCategory = "UNCATEGORIZED"
)

// ArtifactPayload is the JSON structure written to artifact files.
type ArtifactPayload struct {
	FindingID            string           `json:"finding_id"`
	CampaignID           string           `json:"campaign_id"`
	Target               TargetURL        `json:"target"`
	FailureCriterion     FailureCriterion `json:"failure_criterion"`
	Session              RecordingSession `json:"session"`
	MutationSeed         int64            `json:"mutation_seed"`
	MutationOps          []string         `json:"mutation_ops,omitempty"`
	OperatorsByExchange  [][]string       `json:"operators_by_exchange,omitempty"`
	MatchedExchangeIndex *int             `json:"matched_exchange_index,omitempty"`
}

// FailureCriterion describes the expected anomaly type for reproduction.
type FailureCriterion struct {
	Type      FindingType `json:"type"`
	TimeoutMs int64       `json:"timeout_ms,omitempty"`
}

// APIError is the standard error response returned by the Control API.
type APIError struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// CampaignStats is the aggregated statistics snapshot for a campaign.
type CampaignStats struct {
	CampaignID         string         `json:"campaign_id"`
	Status             CampaignStatus `json:"status"`
	TestsTotal         int            `json:"tests_total"`
	TestsPerSec        float64        `json:"tests_per_sec"`
	Timeouts           int            `json:"timeouts"`
	ServerErrors       int            `json:"server_errors"`
	LatencyRegressions int            `json:"latency_regressions"`
	RegexMatches       int            `json:"regex_matches"`
	LastActivityAt     time.Time      `json:"last_activity_at"`
	Seeds              SeedStats      `json:"seeds"`
}

// SeedStats summarises recording seed usage within a campaign.
type SeedStats struct {
	SessionsTotal int `json:"sessions_total"`
	SessionsUsed  int `json:"sessions_used"`
	ExchangesSent int `json:"exchanges_sent"`
}

// BaselineEntry stores the baseline latency and status for an endpoint.
type BaselineEntry struct {
	Method     string
	Endpoint   string
	P50Ms      int64
	StatusCode int
}

// TreeEntry holds a single row from the recordings tree aggregation query.
type TreeEntry struct {
	Scheme string `db:"target_scheme"`
	Host   string `db:"target_host"`
	Port   int    `db:"target_port"`
	Path   string `db:"target_path"`
	Count  int    `db:"cnt"`
}
