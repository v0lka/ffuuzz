package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/anomaly"
	"ffuuzz/internal/model"
	"ffuuzz/internal/replayer"
)

const reproducePollInterval = 5 * time.Second

// ReproduceWorker polls for ENQUEUED findings and replays them to confirm reproducibility.
type ReproduceWorker struct {
	findings    FindingStore
	artifacts   ArtifactStore
	artifactDir string
	logger      zerolog.Logger
}

// NewReproduceWorker creates a reproduce worker.
func NewReproduceWorker(findings FindingStore, artifacts ArtifactStore, artifactDir string, logger zerolog.Logger) *ReproduceWorker {
	return &ReproduceWorker{
		findings:    findings,
		artifacts:   artifacts,
		artifactDir: artifactDir,
		logger:      logger.With().Str("component", "reproduce_worker").Logger(),
	}
}

// Run polls for enqueued reproduce jobs and processes them until ctx is cancelled.
func (w *ReproduceWorker) Run(ctx context.Context) {
	w.logger.Info().Msg("reproduce worker started")
	ticker := time.NewTicker(reproducePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info().Msg("reproduce worker stopped")
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *ReproduceWorker) poll(ctx context.Context) {
	findingID, runs, found, err := w.findings.ClaimNextReproduceJob(ctx)
	if err != nil {
		w.logger.Error().Err(err).Msg("claim reproduce job failed")
		return
	}
	if !found {
		return
	}

	w.logger.Info().Str("finding_id", findingID).Int("runs", runs).Msg("claimed reproduce job")
	w.processJob(ctx, findingID, runs)
}

func (w *ReproduceWorker) processJob(ctx context.Context, findingID string, runs int) {
	if runs <= 0 {
		runs = 3
	}

	// Load finding
	finding, err := w.findings.GetByID(ctx, findingID)
	if err != nil || finding == nil {
		w.logger.Error().Err(err).Str("finding_id", findingID).Msg("load finding failed")
		w.failJob(ctx, findingID)
		return
	}

	// Load artifact
	artifact, err := w.artifacts.GetByFindingID(ctx, findingID)
	if err != nil || artifact == nil {
		w.logger.Error().Err(err).Str("finding_id", findingID).Msg("load artifact failed")
		w.failJob(ctx, findingID)
		return
	}

	// Read artifact file
	filePath := filepath.Join(w.artifactDir, artifact.FilePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		w.logger.Error().Err(err).Str("path", filePath).Msg("read artifact file failed")
		w.failJob(ctx, findingID)
		return
	}

	var payload model.ArtifactPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		w.logger.Error().Err(err).Str("finding_id", findingID).Msg("unmarshal artifact payload failed")
		w.failJob(ctx, findingID)
		return
	}

	// Setup detector matching the finding type
	anomalyCfg := model.AnomalyConfig{
		Detect5xx:         finding.Type == model.FindingServerError,
		LatencyMultiplier: 2.0,
	}
	if finding.Type == model.FindingTimeout {
		anomalyCfg.Detect5xx = false
	}
	detector := anomaly.NewMultiDetector(anomalyCfg, w.logger)

	// Replay N times and count reproductions
	rep := replayer.New(nil, w.logger)
	baseURL := payload.Target.BaseURL
	session := payload.Session
	timeout := 10 * time.Second
	if payload.FailureCriterion.TimeoutMs > 0 {
		timeout = time.Duration(payload.FailureCriterion.TimeoutMs) * time.Millisecond
	}

	reproduced := 0
	for i := 0; i < runs; i++ {
		select {
		case <-ctx.Done():
			w.failJob(ctx, findingID)
			return
		default:
		}

		wctx := replayer.NewWorkerContext(timeout, w.logger)
		results, err := rep.ReplaySession(ctx, session, baseURL, wctx, nil)
		if err != nil {
			w.logger.Debug().Err(err).Str("finding_id", findingID).Int("run", i+1).Msg("replay failed")
			continue
		}

		triggered := false
		for _, result := range results {
			hits := detector.Detect(result.Exchange, result, nil, anomalyCfg)
			if len(hits) > 0 {
				triggered = true
				break
			}
		}
		if triggered {
			reproduced++
		}
	}

	// Majority threshold: reproduced in at least half the runs
	status := "NOT_REPRODUCED"
	if reproduced >= (runs+1)/2 {
		status = "CONFIRMED"
	}

	w.logger.Info().
		Str("finding_id", findingID).
		Int("reproduced", reproduced).
		Int("runs", runs).
		Str("status", status).
		Msg("reproduce job completed")

	if err := w.findings.SetReproduceStatus(ctx, findingID, status); err != nil {
		w.logger.Error().Err(err).Str("finding_id", findingID).Msg("update reproduce status failed")
	}
}

func (w *ReproduceWorker) failJob(ctx context.Context, findingID string) {
	if err := w.findings.SetReproduceStatus(ctx, findingID, "FAILED"); err != nil {
		w.logger.Error().Err(err).Str("finding_id", findingID).Msg("fail reproduce job status update failed")
	}
}
