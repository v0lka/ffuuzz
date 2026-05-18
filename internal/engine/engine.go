// Package engine orchestrates fuzzing campaigns, coordinating workers,
// anomaly detection, triage, and finding persistence.
package engine

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/anomaly"
	"ffuuzz/internal/corpus"
	"ffuuzz/internal/model"
	"ffuuzz/internal/mutate"
	"ffuuzz/internal/replayer"
	"ffuuzz/internal/triage"
)

// CampaignStore defines the campaign operations needed by the engine.
type CampaignStore interface {
	UpdateStatus(ctx context.Context, id string, oldStatus, newStatus model.CampaignStatus) (bool, error)
	IncrementStats(ctx context.Context, id string, testsDelta, findingsDelta int) error
}

// FindingStore defines the finding operations needed by the engine.
type FindingStore interface {
	ExistsBySignature(ctx context.Context, campaignID, signature string) (bool, error)
	Create(ctx context.Context, f model.Finding) error
	UpdateStatus(ctx context.Context, id string, status model.FindingStatus) error
	GetByID(ctx context.Context, id string) (*model.Finding, error)
	ClaimNextReproduceJob(ctx context.Context) (string, int, bool, error)
	SetReproduceStatus(ctx context.Context, id, status string) error
}

// ArtifactStore defines the artifact operations needed by the engine.
type ArtifactStore interface {
	Create(ctx context.Context, a model.Artifact) error
	GetByFindingID(ctx context.Context, findingID string) (*model.Artifact, error)
}

// Engine orchestrates campaign lifecycle and worker pools.
type Engine struct {
	campaigns   CampaignStore
	findings    FindingStore
	artifacts   ArtifactStore
	corpus      *corpus.Manager
	artifactDir string
	logger      zerolog.Logger

	mu      sync.Mutex
	running map[string]context.CancelFunc // campaignID -> cancel

	reproduceCancel context.CancelFunc
	reproduceWG     sync.WaitGroup
}

// NewEngine creates an Engine with the given stores, corpus, artifact directory, and logger.
func NewEngine(
	campaigns CampaignStore,
	findings FindingStore,
	artifacts ArtifactStore,
	corpus *corpus.Manager,
	artifactDir string,
	logger zerolog.Logger,
) *Engine {
	return &Engine{
		campaigns:   campaigns,
		findings:    findings,
		artifacts:   artifacts,
		corpus:      corpus,
		artifactDir: artifactDir,
		logger:      logger,
		running:     make(map[string]context.CancelFunc),
	}
}

// StartCampaign transitions the campaign to STARTING, loads seeds, spawns workers,
// and transitions to RUNNING.
func (e *Engine) StartCampaign(ctx context.Context, campaign *model.Campaign) error {
	// Transition CREATED -> STARTING
	ok, err := e.campaigns.UpdateStatus(ctx, campaign.ID, campaign.Status, model.CampaignStarting)
	if err != nil {
		return fmt.Errorf("update status to STARTING: %w", err)
	}
	if !ok {
		return fmt.Errorf("campaign %s is not in expected state %s", campaign.ID, campaign.Status)
	}

	// Load seeds
	seeds, err := e.corpus.GetSeeds(ctx, campaign.ID)
	if err != nil {
		e.failCampaign(ctx, campaign.ID, model.CampaignStarting, err)
		return fmt.Errorf("load seeds: %w", err)
	}
	if len(seeds) == 0 {
		e.failCampaign(ctx, campaign.ID, model.CampaignStarting, fmt.Errorf("no seeds"))
		return fmt.Errorf("campaign %s has no recording sessions", campaign.ID)
	}

	// Compute baselines
	baselineMap := corpus.ComputeBaseline(seeds)
	baselines := make(map[string]*anomaly.BaselineEntry)
	for k, v := range baselineMap {
		entry := &anomaly.BaselineEntry{
			Method:     v.Method,
			Endpoint:   v.Endpoint,
			P50Ms:      v.P50Ms,
			StatusCode: v.StatusCode,
		}
		baselines[k] = entry
	}

	// Create campaign context - use Background() to decouple from HTTP request lifecycle
	campCtx, cancel := context.WithCancel(context.Background())

	e.mu.Lock()
	e.running[campaign.ID] = cancel
	e.mu.Unlock()

	// Transition STARTING -> RUNNING
	ok, err = e.campaigns.UpdateStatus(ctx, campaign.ID, model.CampaignStarting, model.CampaignRunning)
	if err != nil {
		cancel()
		e.mu.Lock()
		delete(e.running, campaign.ID)
		e.mu.Unlock()
		e.failCampaign(ctx, campaign.ID, model.CampaignStarting, fmt.Errorf("transition to RUNNING failed: %w", err))
		return fmt.Errorf("update status to RUNNING: %w", err)
	}
	if !ok {
		cancel()
		e.mu.Lock()
		delete(e.running, campaign.ID)
		e.mu.Unlock()
		e.failCampaign(ctx, campaign.ID, model.CampaignStarting, fmt.Errorf("campaign not in STARTING state"))
		return fmt.Errorf("campaign %s is not in STARTING state", campaign.ID)
	}

	cfg := campaign.Config
	go e.runCampaign(campCtx, campaign.ID, cfg, seeds, baselines)

	return nil
}

func (e *Engine) runCampaign(
	ctx context.Context,
	campaignID string,
	cfg model.CampaignConfig,
	seeds []model.RecordingSession,
	baselines map[string]*anomaly.BaselineEntry,
) {
	defer func() {
		e.mu.Lock()
		delete(e.running, campaignID)
		e.mu.Unlock()
	}()

	logger := e.logger.With().Str("campaign_id", campaignID).Logger()

	// Setup mutation pipeline
	mutateCfg := mutate.Config{
		PathQuery: cfg.Mutations.PathQuery,
		Headers:   cfg.Mutations.Headers,
		JSONBody:  cfg.Mutations.JSONBody,
		Params:    cfg.Mutations.Params,
		Sequence:  cfg.Mutations.Sequence,
		Intensity: cfg.Mutations.Intensity,
	}
	pipeline := mutate.NewPipeline(mutateCfg)
	var seqMutator *mutate.SeqMutator
	if cfg.Mutations.Sequence {
		seqMutator = &mutate.SeqMutator{}
	}

	// Extract real header values from recorded traffic into the dictionary
	if dict := pipeline.Dict(); dict != nil {
		dict.ExtractFromTraffic(seeds)
	}

	// Setup adaptive intensity tracking
	intensityTracker := NewIntensityTracker()
	pipeline.SetIntensityCallback(func(prefix string) float64 {
		return intensityTracker.GetMultiplier(prefix)
	})

	// Setup coverage-guided feedback tracking
	feedbackTracker := NewSeedInterestTracker()

	// Setup detectors
	detector := anomaly.NewMultiDetector(cfg.Anomaly, logger)
	triager := triage.NewTriager()
	rep := replayer.New(nil, logger)

	// Convert extraction rules from config model to replayer model
	var extractionRules []replayer.ExtractionRule
	for _, r := range cfg.ExtractionRules {
		extractionRules = append(extractionRules, replayer.ExtractionRule{
			Name:   r.Name,
			Source: r.Source,
			Header: r.Header,
			Regex:  r.Regex,
		})
	}

	// Setup rate limiter
	limiter := NewLimiter(cfg.Limits.RPS)
	defer limiter.Close()

	// Create task channel
	numWorkers := cfg.Limits.Workers
	if numWorkers <= 0 {
		numWorkers = 4
	}
	taskCh := make(chan SeedTask, numWorkers*2)

	// Spawn workers
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		w := NewWorker(WorkerConfig{
			ID:               i,
			CampaignID:       campaignID,
			BaseURL:          cfg.Target.BaseURL,
			Pipeline:         pipeline,
			SeqMutator:       seqMutator,
			Detector:         detector,
			Triager:          triager,
			Replayer:         rep,
			Findings:         e.findings,
			Artifacts:        e.artifacts,
			Campaigns:        e.campaigns,
			ArtifactDir:      e.artifactDir,
			AnomalyCfg:       cfg.Anomaly,
			TriageCfg:        cfg.Triage,
			Baselines:        baselines,
			ReqTimeoutMs:     cfg.Limits.ReqTimeoutMs,
			ExtractionRules:  extractionRules,
			IntensityTracker: intensityTracker,
			FeedbackTracker:  feedbackTracker,
			Logger:           logger,
		})
		go func() {
			defer wg.Done()
			w.Run(ctx, taskCh)
		}()
	}

	// Task generator
	maxTests := cfg.Limits.MaxTests
	durationSec := cfg.Limits.DurationSec
	var deadline <-chan time.Time
	if durationSec > 0 {
		timer := time.NewTimer(time.Duration(durationSec) * time.Second)
		defer timer.Stop()
		deadline = timer.C
	}

	testsGenerated := 0
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	logger.Info().
		Int("workers", numWorkers).
		Int("seeds", len(seeds)).
		Int("max_tests", maxTests).
		Int("duration_sec", durationSec).
		Msg("campaign running")

	// Build seed ID index for weighted selection
	seedIDs := make([]string, len(seeds))
	for i, s := range seeds {
		seedIDs[i] = s.ID
	}

	func() {
		defer close(taskCh)
		for {
			if maxTests > 0 && testsGenerated >= maxTests {
				logger.Info().Msg("max_tests reached")
				return
			}

			select {
			case <-ctx.Done():
				return
			default:
			}

			if deadline != nil {
				select {
				case <-deadline:
					logger.Info().Msg("duration_sec reached")
					return
				default:
				}
			}

			// Rate limit
			if err := limiter.Acquire(ctx); err != nil {
				return
			}

			// Pick a seed: epsilon-greedy — 20% exploration (uniform random), 80% exploitation (weighted by interest)
			var session model.RecordingSession
			if len(seeds) > 1 && rng.Float64() < 0.8 {
				weights := feedbackTracker.NormalizedWeights(seedIDs)
				session = weightedPick(seeds, weights, rng)
			} else {
				session = seeds[rng.Intn(len(seeds))]
			}
			seed := rng.Int63()

			select {
			case taskCh <- SeedTask{Session: session, MutationSeed: seed}:
				testsGenerated++
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for workers to finish
	wg.Wait()

	// Determine final status
	finalStatus := model.CampaignFinished
	if ctx.Err() != nil {
		finalStatus = model.CampaignStopped
	}

	_, err := e.campaigns.UpdateStatus(context.Background(), campaignID, model.CampaignRunning, finalStatus)
	if err != nil {
		// Try from STOPPING state (if StopCampaign was called)
		_, err2 := e.campaigns.UpdateStatus(context.Background(), campaignID, model.CampaignStopping, finalStatus)
		if err2 != nil {
			logger.Error().Err(err).Str("campaign_id", campaignID).
				Str("target_status", string(finalStatus)).
				Msg("failed to set final campaign status from both RUNNING and STOPPING")
		}
	}

	logger.Info().Str("final_status", string(finalStatus)).Msg("campaign ended")
}

// StopCampaign cancels a running campaign.
func (e *Engine) StopCampaign(ctx context.Context, id string) error {
	e.mu.Lock()
	cancel, ok := e.running[id]
	e.mu.Unlock()

	if !ok {
		return fmt.Errorf("campaign %s is not running", id)
	}

	// Transition to STOPPING
	_, err := e.campaigns.UpdateStatus(ctx, id, model.CampaignRunning, model.CampaignStopping)
	if err != nil {
		// Try from STARTING
		if _, err := e.campaigns.UpdateStatus(ctx, id, model.CampaignStarting, model.CampaignStopping); err != nil {
			e.logger.Warn().Err(err).Str("campaign_id", id).Msg("fallback status transition to STOPPING failed")
		}
	}

	cancel()
	return nil
}

// StopAll cancels all running campaigns. Used during graceful shutdown.
func (e *Engine) StopAll(ctx context.Context) {
	// Stop reproduce worker
	e.mu.Lock()
	reproduceCancel := e.reproduceCancel
	e.mu.Unlock()

	if reproduceCancel != nil {
		reproduceCancel()
		e.reproduceWG.Wait()
	}

	e.mu.Lock()
	cancels := make(map[string]context.CancelFunc)
	for k, v := range e.running {
		cancels[k] = v
	}
	e.mu.Unlock()

	for id, cancel := range cancels {
		if _, err := e.campaigns.UpdateStatus(ctx, id, model.CampaignRunning, model.CampaignStopping); err != nil {
			e.logger.Warn().Err(err).Str("campaign_id", id).Msg("status transition to STOPPING failed during shutdown")
		}
		cancel()
	}
}

// IsRunning checks if a campaign is currently managed by the engine.
func (e *Engine) IsRunning(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.running[id]
	return ok
}

func (e *Engine) failCampaign(ctx context.Context, id string, fromStatus model.CampaignStatus, reason error) {
	e.logger.Error().Err(reason).Str("campaign_id", id).Msg("campaign failed")
	if _, err := e.campaigns.UpdateStatus(ctx, id, fromStatus, model.CampaignFailed); err != nil {
		e.logger.Warn().Err(err).Str("campaign_id", id).Msg("status transition to FAILED failed")
	}
}

// StartReproduceWorker spawns the background reproduce-finding poller.
func (e *Engine) StartReproduceWorker(ctx context.Context) {
	rCtx, cancel := context.WithCancel(ctx)

	e.mu.Lock()
	e.reproduceCancel = cancel
	e.mu.Unlock()

	e.reproduceWG.Add(1)

	w := NewReproduceWorker(e.findings, e.artifacts, e.artifactDir, e.logger)
	go func() {
		defer e.reproduceWG.Done()
		w.Run(rCtx)
	}()
}

// weightedPick selects an item from items using a probability distribution.
// Weights must be normalized (sum to 1.0). Uses cumulative distribution sampling.
func weightedPick[T any](items []T, weights []float64, rng *rand.Rand) T {
	if len(weights) != len(items) || len(items) == 0 {
		var zero T
		return zero
	}
	r := rng.Float64()
	cumulative := 0.0
	for i, w := range weights {
		cumulative += w
		if r < cumulative {
			return items[i]
		}
	}
	// Fallback: return last item
	return items[len(items)-1]
}
