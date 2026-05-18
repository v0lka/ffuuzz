package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"ffuuzz/internal/anomaly"
	"ffuuzz/internal/metrics"
	"ffuuzz/internal/model"
	"ffuuzz/internal/mutate"
	"ffuuzz/internal/replayer"
	"ffuuzz/internal/triage"
)

// SeedTask is a unit of work for a fuzzing worker.
type SeedTask struct {
	Session      model.RecordingSession
	MutationSeed int64
}

// Worker executes fuzz tests: mutate -> replay -> detect -> triage.
type Worker struct {
	id              int
	campaignID      string
	baseURL         string
	pipeline        *mutate.Pipeline
	seqMutator      *mutate.SeqMutator
	detector        *anomaly.MultiDetector
	triager         *triage.Triager
	replayer        *replayer.Replayer
	findings        FindingStore
	artifacts       ArtifactStore
	campaigns       CampaignStore
	artifactDir     string
	anomalyCfg      model.AnomalyConfig
	triageCfg       model.TriageConfig
	baselines       map[string]*anomaly.BaselineEntry
	reqTimeoutMs    int64
	extractionRules []replayer.ExtractionRule
	logger          zerolog.Logger
}

// WorkerConfig bundles dependencies for a Worker.
type WorkerConfig struct {
	ID              int
	CampaignID      string
	BaseURL         string
	Pipeline        *mutate.Pipeline
	SeqMutator      *mutate.SeqMutator
	Detector        *anomaly.MultiDetector
	Triager         *triage.Triager
	Replayer        *replayer.Replayer
	Findings        FindingStore
	Artifacts       ArtifactStore
	Campaigns       CampaignStore
	ArtifactDir     string
	AnomalyCfg      model.AnomalyConfig
	TriageCfg       model.TriageConfig
	Baselines       map[string]*anomaly.BaselineEntry
	ReqTimeoutMs    int64
	ExtractionRules []replayer.ExtractionRule
	Logger          zerolog.Logger
}

// NewWorker creates a Worker from the given configuration.
func NewWorker(cfg WorkerConfig) *Worker {
	return &Worker{
		id:              cfg.ID,
		campaignID:      cfg.CampaignID,
		baseURL:         cfg.BaseURL,
		pipeline:        cfg.Pipeline,
		seqMutator:      cfg.SeqMutator,
		detector:        cfg.Detector,
		triager:         cfg.Triager,
		replayer:        cfg.Replayer,
		findings:        cfg.Findings,
		artifacts:       cfg.Artifacts,
		campaigns:       cfg.Campaigns,
		artifactDir:     cfg.ArtifactDir,
		anomalyCfg:      cfg.AnomalyCfg,
		triageCfg:       cfg.TriageCfg,
		baselines:       cfg.Baselines,
		reqTimeoutMs:    cfg.ReqTimeoutMs,
		extractionRules: cfg.ExtractionRules,
		logger:          cfg.Logger.With().Int("worker", cfg.ID).Logger(),
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

	// Apply sequence mutation if enabled
	entries := task.Session.Entries
	var seqOps []string
	if w.seqMutator != nil && len(entries) > 1 {
		result := w.seqMutator.Mutate(entries, rng, 0.5)
		entries = result.Exchanges
		seqOps = result.Operators
	}

	// Apply per-exchange mutations
	var allOps []string
	allOps = append(allOps, seqOps...)
	mutatedEntries := make([]model.Exchange, len(entries))
	for i, ex := range entries {
		// Deep copy the exchange to avoid concurrent map access issues
		ex = deepCopyExchange(ex)
		result := w.pipeline.Mutate(ex, rng, w.pipeline.Intensity())
		mutatedEntries[i] = result.Exchange
		allOps = append(allOps, result.Operators...)
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

	// Update session entries with actual responses from replay
	for i, result := range results {
		if i < len(mutatedSession.Entries) {
			mutatedSession.Entries[i].Response = buildResponseData(result)
			mutatedSession.Entries[i].DurationMs = result.DurationMs
		}
	}

	// Check each result for anomalies
	for _, result := range results {
		baselineKey := result.Exchange.Request.Method + "|" + task.Session.Target.Path
		baseline := w.baselines[baselineKey]

		hits := w.detector.Detect(result.Exchange, result, baseline, w.anomalyCfg)
		for _, hit := range hits {
			w.handleHit(ctx, hit, mutatedSession, task.Session.Target.Path, task.MutationSeed, allOps)
		}
	}

	// Increment test counter
	metrics.TestsTotal.Inc()
	if err := w.campaigns.IncrementStats(ctx, w.campaignID, 1, 0); err != nil {
		w.logger.Warn().Err(err).Str("campaign_id", w.campaignID).Msg("increment test stats failed")
	}
}

// buildResponseData converts replay result to model.ResponseData for artifact storage.
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
		const maxBodySize = 64 * 1024 // 64KB limit for artifact storage
		body := result.RespBody
		if len(body) > maxBodySize {
			body = body[:maxBodySize]
			resp.BodyTruncated = true
		}
		resp.BodyB64 = base64.StdEncoding.EncodeToString(body)
	}
	return resp
}

func (w *Worker) handleHit(ctx context.Context, hit anomaly.AnomalyHit, session model.RecordingSession, endpointPattern string, seed int64, ops []string) {
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

	// Extract mutation type (first operator) and payload from the mutated exchange
	var mutationType, mutationPayload string
	if len(ops) > 0 {
		mutationType = ops[0]
	}
	mutationPayload = extractMutationPayload(hit.Exchange, 200)

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
	}

	if err := w.findings.Create(ctx, finding); err != nil {
		w.logger.Error().Err(err).Str("finding_id", findingID).Msg("create finding failed")
		return
	}

	metrics.FindingsTotal.WithLabelValues(string(hit.Type)).Inc()
	if err := w.campaigns.IncrementStats(ctx, w.campaignID, 0, 1); err != nil {
		w.logger.Warn().Err(err).Str("campaign_id", w.campaignID).Msg("increment finding stats failed")
	}

	// Write artifact
	w.writeArtifact(ctx, findingID, session, hit, seed, ops)

	// Triage: confirmation
	if w.triageCfg.ConfirmRuns > 0 {
		timeout := time.Duration(w.reqTimeoutMs) * time.Millisecond
		confirmed, err := w.triager.Confirm(
			ctx, session, w.baseURL, w.detector,
			w.anomalyCfg, w.baselines[hit.Method+"|"+endpointPattern],
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
		}
	}

	// Triage: minimization
	if w.triageCfg.EnableMinimization && finding.Status == model.FindingConfirmed {
		baseline := w.baselines[hit.Method+"|"+endpointPattern]
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

		// Phase 2: minimize JSON bodies in remaining exchanges
		for i, ex := range workingSession.Entries {
			if !triage.HasJSONBody(ex) {
				continue
			}
			bodyMin, err := w.triager.MinimizeJSONBody(
				ctx, workingSession, i, w.baseURL,
				w.detector, w.anomalyCfg, baseline, w.replayer,
				timeout, w.logger,
			)
			if err != nil {
				w.logger.Warn().Err(err).Str("finding_id", findingID).Int("exchange_idx", i).Msg("json body minimization failed")
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

	payload := model.ArtifactPayload{
		FindingID:  findingID,
		CampaignID: w.campaignID,
		Target:     model.TargetURL{BaseURL: w.baseURL},
		FailureCriterion: model.FailureCriterion{
			Type:      hit.Type,
			TimeoutMs: hit.Details.TimeoutMs,
		},
		Session:      session,
		MutationSeed: mutationSeed,
		MutationOps:  mutationOps,
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
