// Package triage provides finding analysis, confirmation, minimization,
// severity scoring, and LLM-assisted triage.
package triage

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

// LLMTriager orchestrates LLM-based analysis of fuzzing findings.
// It is a no-op when the provider is nil (LLM not configured).
type LLMTriager struct {
	provider LLMProvider
	logger   zerolog.Logger
}

// NewLLMTriager creates an LLMTriager. Pass nil for provider to disable LLM analysis
// (all methods become no-ops).
func NewLLMTriager(provider LLMProvider, logger zerolog.Logger) *LLMTriager {
	return &LLMTriager{provider: provider, logger: logger}
}

// AnalyzeFinding performs LLM-based analysis of a single finding.
// artifactPayload may be nil if no artifact is available.
func (t *LLMTriager) AnalyzeFinding(
	ctx context.Context,
	finding *model.Finding,
	artifactData *model.ArtifactPayload,
) (*model.LLMAnalysis, error) {
	if t.provider == nil {
		return nil, nil
	}

	var responseSnippet string
	if artifactData != nil && len(artifactData.Session.Entries) > 0 {
		// Use the last exchange's response body (the mutated/anomalous response)
		last := &artifactData.Session.Entries[len(artifactData.Session.Entries)-1]
		responseSnippet = last.Response.BodyB64
	}

	baselineStatus := 0
	anomalousStatus := finding.Details.HTTPStatus
	if artifactData != nil && len(artifactData.Session.Entries) > 1 {
		baselineStatus = artifactData.Session.Entries[0].Response.Status
	}

	req := LLMAnalysisRequest{
		FindingID:        finding.ID,
		FindingType:      string(finding.Type),
		Method:           finding.Method,
		Endpoint:         finding.Endpoint,
		MutationType:     finding.MutationType,
		MutationPayload:  finding.MutationPayload,
		BaselineStatus:   baselineStatus,
		AnomalousStatus:  anomalousStatus,
		ResponseSnippet:  responseSnippet,
		PreviousAnalysis: finding.LLMAnalysis,
	}

	result, err := t.provider.AnalyzeFinding(ctx, req)
	if err != nil {
		t.logger.Warn().Err(err).Str("finding_id", finding.ID).Msg("llm analysis failed")
		return nil, err
	}
	if result != nil {
		t.logger.Info().
			Str("finding_id", finding.ID).
			Str("classification", result.Classification).
			Float64("confidence", result.Confidence).
			Msg("llm analysis complete")
	}
	return result, nil
}

// GenerateDescription produces a human-readable description for a finding.
func (t *LLMTriager) GenerateDescription(ctx context.Context, finding *model.Finding) (string, error) {
	if t.provider == nil {
		return "", nil
	}

	classification := ""
	if finding.LLMAnalysis != nil {
		classification = finding.LLMAnalysis.Classification
	}

	req := LLMDescriptionRequest{
		Classification: classification,
		Endpoint:       finding.Endpoint,
		Severity:       string(finding.Severity),
		MutationType:   finding.MutationType,
	}
	return t.provider.GenerateDescription(ctx, req)
}

// BatchAnalyze processes multiple findings through LLM analysis.
// artifactGetter is called for each finding to load its artifact data.
// onResult is called for each successfully analyzed finding for persistence.
// Both callbacks receive ctx so they can respect cancellation and timeouts.
func (t *LLMTriager) BatchAnalyze(
	ctx context.Context,
	findings []model.Finding,
	artifactGetter func(ctx context.Context, findingID string) (*model.ArtifactPayload, error),
	onResult func(ctx context.Context, findingID string, analysis *model.LLMAnalysis),
) {
	if t.provider == nil {
		return
	}

	for i, f := range findings {
		select {
		case <-ctx.Done():
			t.logger.Info().Int("processed", i).Int("total", len(findings)).Msg("llm batch cancelled")
			return
		default:
		}

		var artifact *model.ArtifactPayload
		if artifactGetter != nil && f.ArtifactID != "" {
			var err error
			artifact, err = artifactGetter(ctx, f.ArtifactID)
			if err != nil {
				t.logger.Warn().Err(err).Str("finding_id", f.ID).Msg("llm batch: load artifact failed")
			}
		}

		finding := f
		analysis, err := t.AnalyzeFinding(ctx, &finding, artifact)
		if err != nil {
			t.logger.Warn().Err(err).Str("finding_id", f.ID).Msg("llm batch: analyze failed, skipping")
			continue
		}
		if analysis != nil && onResult != nil {
			onResult(ctx, f.ID, analysis)
		}

		if (i+1)%10 == 0 {
			t.logger.Info().Int("processed", i+1).Int("total", len(findings)).Msg("llm batch progress")
		}
	}
	t.logger.Info().Int("total", len(findings)).Msg("llm batch complete")
}

// GenerateReport produces an executive summary from a set of findings.
func (t *LLMTriager) GenerateReport(ctx context.Context, findings []model.Finding) (string, error) {
	if t.provider == nil {
		return "", nil
	}

	inputs := make([]LLMReportInput, 0, len(findings))
	for _, f := range findings {
		classification := ""
		if f.LLMAnalysis != nil {
			classification = f.LLMAnalysis.Classification
		}
		inputs = append(inputs, LLMReportInput{
			ID:             f.ID,
			Endpoint:       f.Endpoint,
			Type:           string(f.Type),
			Severity:       string(f.Severity),
			Classification: classification,
			Description:    "",
		})
	}

	return t.provider.GenerateReport(ctx, inputs)
}

// MarshalAnalysis serializes an LLMAnalysis to JSON for DB storage.
func MarshalAnalysis(a *model.LLMAnalysis) ([]byte, error) {
	if a == nil {
		return nil, nil
	}
	return json.Marshal(a)
}
