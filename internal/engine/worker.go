package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"ffuuzz/internal/anomaly"
	"ffuuzz/internal/endpoint"
	"ffuuzz/internal/metrics"
	"ffuuzz/internal/model"
	"ffuuzz/internal/mutate"
	"ffuuzz/internal/replayer"
	"ffuuzz/internal/triage"
)

// SeedTask is a unit of work for a fuzzing worker.
//
// When SessionMode is false (the default), the worker mutates only the
// exchange at index TargetExchange and replays the rest verbatim. This
// preserves session state (cookies, CSRF tokens) while letting the scheduler
// target a single endpoint. TargetEndpoint identifies that endpoint for the
// EndpointPlanner reward path; it is informational and may be the zero Key
// for tasks that bypass the planner.
//
// When SessionMode is true, the worker mutates every exchange in the session
// and may invoke the sequence mutator. TargetExchange is ignored in this
// mode.
type SeedTask struct {
	Session        model.RecordingSession
	MutationSeed   int64
	TargetEndpoint endpoint.Key
	TargetExchange int
	SessionMode    bool
}

// Worker executes fuzz tests: mutate -> replay -> detect -> triage.
type Worker struct {
	id               int
	campaignID       string
	baseURL          string
	pipeline         *mutate.Pipeline
	seqMutator       *mutate.SeqMutator
	detector         *anomaly.MultiDetector
	triager          *triage.Triager
	replayer         *replayer.Replayer
	findings         FindingStore
	artifacts        ArtifactStore
	campaigns        CampaignStore
	artifactDir      string
	anomalyCfg       model.AnomalyConfig
	triageCfg        model.TriageConfig
	baselines        map[string]*anomaly.BaselineEntry
	reqTimeoutMs     int64
	extractionRules  []replayer.ExtractionRule
	intensityTracker *IntensityTracker
	feedbackTracker  *SeedInterestTracker
	planner          *EndpointPlanner
	logger           zerolog.Logger
}

// WorkerConfig bundles dependencies for a Worker.
type WorkerConfig struct {
	ID               int
	CampaignID       string
	BaseURL          string
	Pipeline         *mutate.Pipeline
	SeqMutator       *mutate.SeqMutator
	Detector         *anomaly.MultiDetector
	Triager          *triage.Triager
	Replayer         *replayer.Replayer
	Findings         FindingStore
	Artifacts        ArtifactStore
	Campaigns        CampaignStore
	ArtifactDir      string
	AnomalyCfg       model.AnomalyConfig
	TriageCfg        model.TriageConfig
	Baselines        map[string]*anomaly.BaselineEntry
	ReqTimeoutMs     int64
	ExtractionRules  []replayer.ExtractionRule
	IntensityTracker *IntensityTracker
	FeedbackTracker  *SeedInterestTracker
	Planner          *EndpointPlanner
	Logger           zerolog.Logger
}

// NewWorker creates a Worker from the given configuration.
func NewWorker(cfg WorkerConfig) *Worker {
	return &Worker{
		id:               cfg.ID,
		campaignID:       cfg.CampaignID,
		baseURL:          cfg.BaseURL,
		pipeline:         cfg.Pipeline,
		seqMutator:       cfg.SeqMutator,
		detector:         cfg.Detector,
		triager:          cfg.Triager,
		replayer:         cfg.Replayer,
		findings:         cfg.Findings,
		artifacts:        cfg.Artifacts,
		campaigns:        cfg.Campaigns,
		artifactDir:      cfg.ArtifactDir,
		anomalyCfg:       cfg.AnomalyCfg,
		triageCfg:        cfg.TriageCfg,
		baselines:        cfg.Baselines,
		reqTimeoutMs:     cfg.ReqTimeoutMs,
		extractionRules:  cfg.ExtractionRules,
		intensityTracker: cfg.IntensityTracker,
		feedbackTracker:  cfg.FeedbackTracker,
		planner:          cfg.Planner,
		logger:           cfg.Logger.With().Int("worker", cfg.ID).Logger(),
	}
}

// deepCopyExchange creates a deep copy of an Exchange to avoid concurrent map access.
func deepCopyExchange(ex model.Exchange) model.Exchange {
	// Copy request headers map
	if ex.Request.Headers != nil {
		headersCopy := make(map[string][]string, len(ex.Request.Headers))
		for k, v := range ex.Request.Headers {
			valsCopy := make([]string, len(v))
			copy(valsCopy, v)
			headersCopy[k] = valsCopy
		}
		ex.Request.Headers = headersCopy
	}
	// Copy response headers map
	if ex.Response.Headers != nil {
		headersCopy := make(map[string][]string, len(ex.Response.Headers))
		for k, v := range ex.Response.Headers {
			valsCopy := make([]string, len(v))
			copy(valsCopy, v)
			headersCopy[k] = valsCopy
		}
		ex.Response.Headers = headersCopy
	}
	return ex
}

// extractMutationPayload extracts a truncated payload from the mutated exchange.
// It prioritizes body content, then query parameters.
//
// Deprecated: prefer buildMutationPayload, which produces a diff-based summary
// of the mutation that triggered a finding (so MutationType and the displayed
// payload always describe the same transformation). This function is retained
// to satisfy older tests and as a last-resort fallback when no diff is
// available.
func extractMutationPayload(ex model.Exchange, maxLen int) string {
	// Try body first
	if ex.Request.BodyB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
		if err == nil && len(decoded) > 0 {
			if len(decoded) > maxLen {
				return string(decoded[:maxLen])
			}
			return string(decoded)
		}
	}

	// Try query parameters
	if ex.Request.Query != "" {
		if len(ex.Request.Query) > maxLen {
			return ex.Request.Query[:maxLen]
		}
		return ex.Request.Query
	}

	return ""
}

// buildMutationPayload returns a compact human-readable diff between the
// original and mutated request, truncated to maxLen bytes. The output lists
// only the fields that actually changed, so the displayed payload aligns
// with the operator chain attributed to the finding.
func buildMutationPayload(orig, mutated model.RequestData, maxLen int) string {
	var parts []string
	if orig.Method != mutated.Method {
		parts = append(parts, fmt.Sprintf("method: %q -> %q", orig.Method, mutated.Method))
	}
	if orig.Path != mutated.Path {
		parts = append(parts, fmt.Sprintf("path: %q -> %q", orig.Path, mutated.Path))
	}
	if orig.Query != mutated.Query {
		parts = append(parts, fmt.Sprintf("query: %q -> %q", orig.Query, mutated.Query))
	}
	if orig.BodyB64 != mutated.BodyB64 {
		oBody, _ := base64.StdEncoding.DecodeString(orig.BodyB64)
		mBody, _ := base64.StdEncoding.DecodeString(mutated.BodyB64)
		parts = append(parts, fmt.Sprintf("body: %q -> %q", truncString(string(oBody), 80), truncString(string(mBody), 80)))
	}
	headerDiffs := diffHeaders(orig.Headers, mutated.Headers)
	parts = append(parts, headerDiffs...)

	if len(parts) == 0 {
		// No detectable change in the request fields we track. Fall back to
		// the post-mutation snapshot so the user still sees something useful
		// (this can happen for primitive operators that mutate response
		// expectations but not the request).
		return extractMutationPayload(model.Exchange{Request: mutated}, maxLen)
	}

	s := strings.Join(parts, "; ")
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

// truncString returns at most n bytes of s, marking truncation with an
// ellipsis so the diff stays readable.
func truncString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\u2026"
}

// diffHeaders enumerates header-level changes between the original and
// mutated request, sorting keys for deterministic output.
func diffHeaders(orig, mutated map[string][]string) []string {
	keys := make(map[string]struct{}, len(orig)+len(mutated))
	for k := range orig {
		keys[k] = struct{}{}
	}
	for k := range mutated {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	var out []string
	for _, k := range sorted {
		ov, oOK := orig[k]
		mv, mOK := mutated[k]
		switch {
		case !oOK && mOK:
			out = append(out, fmt.Sprintf("+%s: %v", k, mv))
		case oOK && !mOK:
			out = append(out, fmt.Sprintf("-%s: %v", k, ov))
		case oOK && mOK && !stringSlicesEqual(ov, mv):
			out = append(out, fmt.Sprintf("~%s: %v -> %v", k, ov, mv))
		}
	}
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Run processes tasks from taskCh until ctx is cancelled or taskCh is closed.
func (w *Worker) Run(ctx context.Context, taskCh <-chan SeedTask) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-taskCh:
			if !ok {
				return
			}
			w.processTask(ctx, task)
		}
	}
}

func (w *Worker) processTask(ctx context.Context, task SeedTask) {
	rng := rand.New(rand.NewSource(task.MutationSeed))

	// Apply sequence mutation only in session-mode tasks. Targeted (per-
	// endpoint) tasks must preserve the recorded sequence so non-target
	// exchanges replay verbatim and provide the state context (cookies,
	// CSRF, auth) the target exchange depends on.
	entries := task.Session.Entries
	var seqOps []string
	if task.SessionMode && w.seqMutator != nil && len(entries) > 1 {
		result := w.seqMutator.Mutate(entries, rng, 0.5)
		entries = result.Exchanges
		seqOps = result.Operators
	}

	// Apply per-exchange mutations and remember each exchange's pre-mutation
	// state plus its individual operator chain. Tracking ops per exchange (as
	// well as the flat allOps list used by intensity tracking) ensures the
	// MutationType displayed for a finding always describes the operators that
	// touched the matching exchange — never an unrelated mutation applied to a
	// different exchange in the same session.
	//
	// In targeted mode (SessionMode==false) only the exchange at index
	// task.TargetExchange is fed to the mutation pipeline; all others are
	// deep-copied verbatim. This keeps the planner's per-endpoint accounting
	// honest: one task touches one endpoint by mutation, even if other
	// exchanges echo state changes during replay.
	mutatedEntries := make([]model.Exchange, len(entries))
	originalEntries := make([]model.Exchange, len(entries))
	opsByExchange := make([][]string, len(entries))
	var allOps []string
	allOps = append(allOps, seqOps...)
	for i, ex := range entries {
		// Deep copy twice: one snapshot we keep as the pre-mutation reference
		// and one we hand to the pipeline so it can mutate freely without
		// affecting our diff baseline.
		originalEntries[i] = deepCopyExchange(ex)
		mutationInput := deepCopyExchange(ex)
		if task.SessionMode || i == task.TargetExchange {
			result := w.pipeline.Mutate(mutationInput, rng, w.pipeline.Intensity())
			mutatedEntries[i] = result.Exchange
			opsByExchange[i] = result.Operators
			allOps = append(allOps, result.Operators...)
		} else {
			mutatedEntries[i] = mutationInput
		}
	}

	// Record mutation operators for adaptive intensity tracking
	if w.intensityTracker != nil {
		w.intensityTracker.RecordApplication(allOps)
	}

	// Replay the mutated session
	mutatedSession := task.Session
	mutatedSession.Entries = mutatedEntries
	timeout := time.Duration(w.reqTimeoutMs) * time.Millisecond
	wctx := replayer.NewWorkerContext(timeout, w.logger)
	results, err := w.replayer.ReplaySession(ctx, mutatedSession, w.baseURL, wctx, w.extractionRules)
	if err != nil {
		w.logger.Debug().Err(err).Str("recording_id", task.Session.ID).Msg("replay session error")
		return
	}

	// Update session entries with actual responses from replay (default 64KB
	// head truncation). For regex-match findings handleHit later replaces the
	// matching entry's response with a window centered on the match offset so
	// the matching substring is preserved in the artifact.
	for i, result := range results {
		if i < len(mutatedSession.Entries) {
			mutatedSession.Entries[i].Response = buildResponseData(result)
			mutatedSession.Entries[i].DurationMs = result.DurationMs
		}
	}

	// Record responses for coverage-guided feedback tracking, and feed the
	// per-endpoint reward to the planner. The planner reward delta uses the
	// same novelty coefficients as the seed-level tracker so both signals
	// stay calibrated. Reward is attributed to the unmutated request's
	// endpoint key — this keeps URI-mutation noise out of the key derivation.
	if w.feedbackTracker != nil {
		for i, result := range results {
			// Only track error responses (4xx/5xx) for novelty
			errBody := ""
			if result.StatusCode >= 400 {
				errBody = string(result.RespBody)
			}
			delta := w.feedbackTracker.RecordResponse(task.Session.ID, result.StatusCode, errBody)
			if w.planner != nil && delta > 0 && i < len(originalEntries) {
				k := endpoint.KeyFromExchange(originalEntries[i])
				w.planner.Reward(k, delta)
			}
		}
	}

	// Check each result for anomalies. We attribute hits to the exchange
	// index that produced them and override hit.Method/hit.Endpoint with the
	// pre-mutation request so analysts see the original endpoint, not a
	// mutated path that may have been corrupted by URI fuzzing.
	for idx, result := range results {
		baselineKey := endpoint.NewKey(result.Exchange.Request.Method, task.Session.Target.Path).String()
		baseline := w.baselines[baselineKey]

		hits := w.detector.Detect(result.Exchange, result, baseline, w.anomalyCfg)
		for _, hit := range hits {
			hit.HitExchangeIndex = idx
			hit.OpsByExchange = opsByExchange
			hit.Details.ExchangeIndex = idx
			if idx < len(originalEntries) {
				hit.OriginalRequest = originalEntries[idx].Request
				hit.Method = originalEntries[idx].Request.Method
				hit.Endpoint = originalEntries[idx].Request.Path
			}

			// hitOps = sequence ops (campaign-level) + this exchange's ops.
			// Sequence ops are session-wide so they always appear; per-
			// exchange ops vary across the session.
			hitOps := make([]string, 0, len(seqOps)+len(opsByExchange[idx]))
			hitOps = append(hitOps, seqOps...)
			hitOps = append(hitOps, opsByExchange[idx]...)

			w.handleHit(ctx, hit, mutatedSession, results, task.Session.Target.Path, task.MutationSeed, hitOps)
		}
	}

	// Increment test counter. We label by HTTP method and normalised
	// endpoint. In targeted mode the planner already provides the exact key;
	// in session mode we derive the label from the first exchange so the
	// metric stays queryable rather than dropping into an empty-label
	// bucket.
	var testKey endpoint.Key
	switch {
	case !task.SessionMode && task.TargetEndpoint != (endpoint.Key{}):
		testKey = task.TargetEndpoint
	case len(task.Session.Entries) > 0:
		testKey = endpoint.KeyFromExchange(task.Session.Entries[0])
	}
	metrics.TestsTotal.WithLabelValues(testKey.Method, testKey.Path).Inc()
	if err := w.campaigns.IncrementStats(ctx, w.campaignID, 1, 0); err != nil {
		w.logger.Warn().Err(err).Str("campaign_id", w.campaignID).Msg("increment test stats failed")
	}
}

// buildResponseData converts replay result to model.ResponseData for artifact storage.
//
// The body is base64-encoded; bodies larger than maxArtifactBodySize are
// truncated from the head so artifacts stay under a sensible per-finding
// size budget. Use buildResponseDataAround to preserve a window around a
// regex match offset instead of unconditionally taking the head.
func buildResponseData(result replayer.ExchangeResult) model.ResponseData {
	resp := model.ResponseData{
		Status: result.StatusCode,
	}
	if result.RespHeaders != nil {
		resp.Headers = make(map[string][]string, len(result.RespHeaders))
		for k, v := range result.RespHeaders {
			resp.Headers[k] = v
		}
	}
	if len(result.RespBody) > 0 {
		body := result.RespBody
		if len(body) > maxArtifactBodySize {
			body = body[:maxArtifactBodySize]
			resp.BodyTruncated = true
		}
		resp.BodyB64 = base64.StdEncoding.EncodeToString(body)
	}
	return resp
}

// maxArtifactBodySize bounds the size of any single response body stored in
// an artifact. 64 KiB is enough for almost any HTML/JSON error page while
// keeping artifact files small enough to ship and reload quickly.
const maxArtifactBodySize = 64 * 1024

// buildResponseDataAround behaves like buildResponseData but, when the body
// exceeds maxArtifactBodySize, picks a window centered on aroundOffset so the
// regex match (or any other interesting offset) is preserved.  Pass a
// negative aroundOffset to get the default head-truncation behaviour.
func buildResponseDataAround(result replayer.ExchangeResult, aroundOffset int) model.ResponseData {
	resp := model.ResponseData{
		Status: result.StatusCode,
	}
	if result.RespHeaders != nil {
		resp.Headers = make(map[string][]string, len(result.RespHeaders))
		for k, v := range result.RespHeaders {
			resp.Headers[k] = v
		}
	}
	if len(result.RespBody) == 0 {
		return resp
	}
	body := result.RespBody
	if len(body) <= maxArtifactBodySize {
		resp.BodyB64 = base64.StdEncoding.EncodeToString(body)
		return resp
	}

	if aroundOffset < 0 {
		body = body[:maxArtifactBodySize]
		resp.BodyTruncated = true
		resp.BodyB64 = base64.StdEncoding.EncodeToString(body)
		return resp
	}

	// Center a maxArtifactBodySize window on aroundOffset, clamping to body
	// boundaries.
	half := maxArtifactBodySize / 2
	start := aroundOffset - half
	if start < 0 {
		start = 0
	}
	end := start + maxArtifactBodySize
	if end > len(result.RespBody) {
		end = len(result.RespBody)
		start = end - maxArtifactBodySize
		if start < 0 {
			start = 0
		}
	}
	resp.BodyTruncated = true
	resp.BodyB64 = base64.StdEncoding.EncodeToString(result.RespBody[start:end])
	return resp
}

func (w *Worker) handleHit(ctx context.Context, hit anomaly.AnomalyHit, session model.RecordingSession, results []replayer.ExchangeResult, endpointPattern string, seed int64, ops []string) {
	sig := w.triager.Signature(hit)

	// Check dedup
	exists, err := w.findings.ExistsBySignature(ctx, w.campaignID, sig)
	if err != nil {
		w.logger.Error().Err(err).Msg("check signature failed")
		return
	}
	if exists {
		return
	}

	// Derive mutation_type from the operator chain attributed to THIS hit's
	// exchange (passed in via ops). This is correct now that ops contains
	// only the operators applied to the matching exchange (plus session-wide
	// sequence ops). Before this refactor, the worker passed a flat list of
	// every operator applied to every exchange in the session, which made
	// MutationType and MutationPayload describe unrelated mutations — that
	// was the root cause of the “different mutators, identical payloads” bug.
	var mutationType, mutationPayload string
	if len(ops) > 0 {
		mutationType = ops[0]
	}
	// Diff the original and mutated request of the matching exchange so the
	// payload describes the same transformation as MutationType.
	if hit.HitExchangeIndex >= 0 && hit.HitExchangeIndex < len(session.Entries) {
		mutationPayload = buildMutationPayload(hit.OriginalRequest, session.Entries[hit.HitExchangeIndex].Request, 200)
	} else {
		mutationPayload = extractMutationPayload(hit.Exchange, 200)
	}

	// For regex-match findings, replace the matching entry's response with a
	// window centered on the match offset so the actual matching substring is
	// preserved in the artifact. Without this, the default 64 KiB head
	// truncation can drop the match entirely on large bodies, leaving an
	// analyst staring at an artifact that contains none of the regex hits.
	if hit.Type == model.FindingRegexMatch && hit.HitExchangeIndex >= 0 && hit.HitExchangeIndex < len(results) && hit.HitExchangeIndex < len(session.Entries) {
		session.Entries[hit.HitExchangeIndex].Response = buildResponseDataAround(results[hit.HitExchangeIndex], hit.Details.MatchOffset)
	}

	// Snapshot the regex patterns active at finding-creation time. Storing
	// them on the finding decouples the displayed regex set from any later
	// edits to the campaign config, so analysts always see the patterns that
	// actually produced the match.
	var regexPatterns []string
	if hit.Type == model.FindingRegexMatch && len(w.anomalyCfg.RegexPatterns) > 0 {
		regexPatterns = append(regexPatterns, w.anomalyCfg.RegexPatterns...)
	}

	findingID := uuid.New().String()
	finding := model.Finding{
		ID:              findingID,
		CampaignID:      w.campaignID,
		Type:            hit.Type,
		Status:          model.FindingUnconfirmed,
		Signature:       sig,
		CreatedAt:       time.Now().UTC(),
		Method:          hit.Method,
		Endpoint:        hit.Endpoint,
		Details:         hit.Details,
		MutationType:    mutationType,
		MutationPayload: mutationPayload,
		MutationOps:     ops,
		RegexPatterns:   regexPatterns,
		Severity: w.triager.ScoreSeverity(
			hit.Type, hit.Endpoint, hit.Method,
			mutationType, 0.0, hit.Details.HTTPStatus,
		),
		OWASPCategory: w.triager.CategorizeFinding(
			hit.Type, mutationType, hit.ResultBody, hit.Details.HTTPStatus,
		),
	}

	if err := w.findings.Create(ctx, finding); err != nil {
		w.logger.Error().Err(err).Str("finding_id", findingID).Msg("create finding failed")
		return
	}

	// Record finding for adaptive intensity tracking
	if w.intensityTracker != nil {
		w.intensityTracker.RecordFinding(ops)
	}

	// Record finding for coverage-guided feedback tracking
	if w.feedbackTracker != nil {
		w.feedbackTracker.RecordFinding(session.ID)
	}

	// Record finding for the endpoint planner. The planner uses this to
	// bias future Pick() decisions toward fruitful endpoints.
	if w.planner != nil {
		w.planner.RecordFinding(endpoint.NewKey(hit.Method, endpointPattern))
	}

	metrics.FindingsTotal.WithLabelValues(string(hit.Type), hit.Method, endpointPattern).Inc()
	if err := w.campaigns.IncrementStats(ctx, w.campaignID, 0, 1); err != nil {
		w.logger.Warn().Err(err).Str("campaign_id", w.campaignID).Msg("increment finding stats failed")
	}

	// Write artifact
	w.writeArtifact(ctx, findingID, session, hit, seed, ops)

	// Triage: confirmation
	if w.triageCfg.ConfirmRuns > 0 {
		timeout := time.Duration(w.reqTimeoutMs) * time.Millisecond
		confirmed, reproduced, err := w.triager.Confirm(
			ctx, session, w.baseURL, w.detector,
			w.anomalyCfg, w.baselines[endpoint.NewKey(hit.Method, endpointPattern).String()],
			w.replayer, w.triageCfg.ConfirmRuns,
			timeout, w.logger,
		)
		if err != nil {
			w.logger.Warn().Err(err).Str("finding_id", findingID).Msg("confirmation failed")
		} else if confirmed {
			if err := w.findings.UpdateStatus(ctx, findingID, model.FindingConfirmed); err != nil {
				w.logger.Error().Err(err).Str("finding_id", findingID).Msg("update finding status failed")
			}
			finding.Status = model.FindingConfirmed
			finding.Reproducibility = float64(reproduced) / float64(w.triageCfg.ConfirmRuns)
			finding.Severity = w.triager.ScoreSeverity(
				finding.Type, finding.Endpoint, finding.Method,
				mutationType, finding.Reproducibility, hit.Details.HTTPStatus,
			)
		}
	}

	// Triage: minimization
	if w.triageCfg.EnableMinimization && finding.Status == model.FindingConfirmed {
		baseline := w.baselines[endpoint.NewKey(hit.Method, endpointPattern).String()]
		workingSession := session
		changed := false

		// Phase 1: remove unnecessary exchanges
		timeout := time.Duration(w.reqTimeoutMs) * time.Millisecond
		minimized, err := w.triager.MinimizeSession(
			ctx, workingSession, w.baseURL, w.detector,
			w.anomalyCfg, baseline, w.replayer,
			timeout, w.logger,
		)
		if err != nil {
			w.logger.Warn().Err(err).Str("finding_id", findingID).Msg("session minimization failed")
		} else if minimized != nil && len(minimized.Entries) < len(workingSession.Entries) {
			w.logger.Info().
				Str("finding_id", findingID).
				Int("original_entries", len(workingSession.Entries)).
				Int("minimized_entries", len(minimized.Entries)).
				Msg("session minimized")
			workingSession = *minimized
			changed = true
		}

		// Phase 2: minimize request bodies in remaining exchanges
		for i := range workingSession.Entries {
			ct := triage.GetContentType(workingSession.Entries[i].Request)
			var bodyMin *model.RecordingSession
			var bodyErr error
			switch {
			case strings.Contains(ct, "json"):
				bodyMin, bodyErr = w.triager.MinimizeJSONBody(
					ctx, workingSession, i, w.baseURL,
					w.detector, w.anomalyCfg, baseline, w.replayer,
					timeout, w.logger,
				)
			case ct == "application/x-www-form-urlencoded":
				bodyMin, bodyErr = w.triager.MinimizeQueryParams(
					ctx, workingSession, i, w.baseURL,
					w.detector, w.anomalyCfg, baseline, w.replayer,
					timeout, w.logger,
				)
			case strings.Contains(ct, "xml"):
				bodyMin, bodyErr = w.triager.MinimizeXMLBody(
					ctx, workingSession, i, w.baseURL,
					w.detector, w.anomalyCfg, baseline, w.replayer,
					timeout, w.logger,
				)
			case strings.Contains(ct, "multipart/form-data"):
				bodyMin, bodyErr = w.triager.MinimizeMultipartBody(
					ctx, workingSession, i, w.baseURL,
					w.detector, w.anomalyCfg, baseline, w.replayer,
					timeout, w.logger,
				)
			default:
				continue
			}
			if bodyErr != nil {
				w.logger.Warn().Err(bodyErr).Str("finding_id", findingID).Int("exchange_idx", i).Msg("body minimization failed")
				continue
			}
			if bodyMin != nil {
				workingSession = *bodyMin
				changed = true
			}
		}

		if changed {
			w.writeArtifact(ctx, findingID, workingSession, hit, seed, ops)
		}
	}

	w.logger.Info().
		Str("finding_id", findingID).
		Str("type", string(hit.Type)).
		Str("method", hit.Method).
		Str("endpoint", hit.Endpoint).
		Str("signature", sig).
		Msg("new finding")
}

func (w *Worker) writeArtifact(ctx context.Context, findingID string, session model.RecordingSession, hit anomaly.AnomalyHit, mutationSeed int64, mutationOps []string) {
	dir := filepath.Join(w.artifactDir, w.campaignID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		w.logger.Error().Err(err).Msg("create artifact dir failed")
		return
	}

	// Pin the matched exchange index in the artifact so analysts can jump
	// straight to the request/response that triggered the anomaly without
	// scanning every entry. The flat MutationOps list remains for backward
	// compatibility, while OperatorsByExchange records the per-exchange
	// breakdown that proves *which* exchange got which operators.
	var matchedIdx *int
	if hit.HitExchangeIndex >= 0 && hit.HitExchangeIndex < len(session.Entries) {
		idx := hit.HitExchangeIndex
		matchedIdx = &idx
	}

	payload := model.ArtifactPayload{
		FindingID:  findingID,
		CampaignID: w.campaignID,
		Target:     model.TargetURL{BaseURL: w.baseURL},
		FailureCriterion: model.FailureCriterion{
			Type:      hit.Type,
			TimeoutMs: hit.Details.TimeoutMs,
		},
		Session:              session,
		MutationSeed:         mutationSeed,
		MutationOps:          mutationOps,
		OperatorsByExchange:  hit.OpsByExchange,
		MatchedExchangeIndex: matchedIdx,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		w.logger.Error().Err(err).Msg("marshal artifact failed")
		return
	}

	filePath := filepath.Join(dir, findingID+".json")
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		w.logger.Error().Err(err).Msg("write artifact tmp failed")
		return
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		w.logger.Error().Err(err).Msg("rename artifact failed")
		return
	}

	relPath := fmt.Sprintf("%s/%s.json", w.campaignID, findingID)
	artifact := model.Artifact{
		ID:        uuid.New().String(),
		FindingID: findingID,
		FilePath:  relPath,
		CreatedAt: time.Now().UTC(),
		SizeBytes: int64(len(data)),
	}
	if err := w.artifacts.Create(ctx, artifact); err != nil {
		w.logger.Error().Err(err).Msg("create artifact record failed")
	}
}
