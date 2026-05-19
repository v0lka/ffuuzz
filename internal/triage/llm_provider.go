// Package triage provides finding analysis, confirmation, minimization,
// severity scoring, and LLM-assisted triage.
package triage

import (
	"context"

	"ffuuzz/internal/model"
)

// LLMProvider defines the interface for LLM-based finding analysis.
// Consumer (internal/triage) owns this interface; implementations live
// in internal/llm/.
type LLMProvider interface {
	// AnalyzeFinding sends finding context to an LLM and returns a structured
	// vulnerability analysis. The response includes classification, severity,
	// confidence, exploitability, and remediation.
	AnalyzeFinding(ctx context.Context, req LLMAnalysisRequest) (*model.LLMAnalysis, error)

	// GenerateDescription produces a concise human-readable description of a
	// finding for display in the UI.
	GenerateDescription(ctx context.Context, req LLMDescriptionRequest) (string, error)

	// GenerateReport produces an executive summary and recommendations from a
	// set of finding summaries.
	GenerateReport(ctx context.Context, findings []LLMReportInput) (string, error)
}

// LLMAnalysisRequest carries the data needed to classify a single finding.
type LLMAnalysisRequest struct {
	FindingID       string
	FindingType     string
	Method          string
	Endpoint        string
	MutationType    string
	MutationPayload string
	BaselineStatus  int
	AnomalousStatus int
	ResponseSnippet string // truncated response body (~2000 chars)
}

// LLMDescriptionRequest carries the data needed to generate a finding description.
type LLMDescriptionRequest struct {
	Classification string
	Endpoint       string
	Severity       string
	MutationType   string
}

// LLMReportInput carries per-finding context for report generation.
type LLMReportInput struct {
	ID             string
	Endpoint       string
	Type           string
	Severity       string
	Classification string
	Description    string
}
