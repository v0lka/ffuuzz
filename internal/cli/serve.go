package cli

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"ffuuzz/internal/api"
	"ffuuzz/internal/config"
	"ffuuzz/internal/corpus"
	"ffuuzz/internal/db"
	"ffuuzz/internal/endpoint"
	"ffuuzz/internal/engine"
	"ffuuzz/internal/logging"
	"ffuuzz/internal/mitm"
	"ffuuzz/internal/recorder"
	"ffuuzz/internal/store"
	"ffuuzz/web"
)

func (c *CLI) runServe(args []string) int {
	logger := logging.New(c.stderr)

	cfg, err := config.Load(args)
	if err != nil {
		logger.Fatal().Err(err).Msg("config load error")
	}

	database, err := db.Open(cfg.DatabaseURI, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}
	// Fatal exits the process, so we don't need to return
	defer func() { _ = database.Close() }()

	recordingStore := db.NewRecordingStore(database.DB, logger)
	campaignStore := db.NewCampaignStore(database.DB, logger)
	findingStore := db.NewFindingStore(database.DB, logger)
	artifactStore := db.NewArtifactStore(database.DB, logger)

	corpusMgr := corpus.NewManager(recordingStore, campaignStore, logger)

	eng := engine.NewEngine(
		campaignStore,
		findingStore,
		artifactStore,
		corpusMgr,
		cfg.ArtifactDir,
		logger,
	)

	if err := os.MkdirAll(cfg.ArtifactDir, 0o750); err != nil {
		logger.Fatal().Err(err).Msg("failed to create artifact directory")
	}

	resolver := endpoint.NewResolver(recordingStore, logger)
	if err := resolver.RebuildFromDB(context.Background()); err != nil {
		logger.Error().Err(err).Msg("endpoint resolver rebuild failed (continuing without pre-existing collapses)")
	}

	rec := recorder.NewDBRecorder(recordingStore, resolver, logger)

	cs, err := store.NewCertStore(cfg.CertCache, cfg.TLS, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create cert store")
	}

	proxy := mitm.New(mitm.Config{
		ListenAddr:    cfg.ProxyAddress,
		CertStore:     cs,
		Recorder:      rec,
		MaxBodyBytes:  cfg.MaxBodyBytes,
		TLSSkipVerify: cfg.TLSSkipVerify,
		Logger:        logger,
	})

	errCh := make(chan error, 2)

	go func() {
		if _, srvErr := proxy.ListenAndServe(); srvErr != nil && !errors.Is(srvErr, http.ErrServerClosed) {
			logger.Error().Err(srvErr).Msg("proxy exited")
			errCh <- srvErr
		}
	}()

	webFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to prepare embedded web assets")
	}

	apiSrv := api.NewServer(api.ServerConfig{
		Addr:        cfg.APIAddress,
		Recordings:  recordingStore,
		Campaigns:   campaignStore,
		Findings:    findingStore,
		Artifacts:   artifactStore,
		Engine:      eng,
		Health:      database,
		ArtifactDir: cfg.ArtifactDir,
		WebFS:       webFS,
		Logger:      logger,
	})

	go func() {
		if srvErr := apiSrv.ListenAndServe(); srvErr != nil && !errors.Is(srvErr, http.ErrServerClosed) {
			logger.Error().Err(srvErr).Msg("api server exited")
			errCh <- srvErr
		}
	}()

	logger.Info().
		Str("proxy", cfg.ProxyAddress).
		Str("api", cfg.APIAddress).
		Str("db", sanitizeDSN(cfg.DatabaseURI)).
		Str("artifacts", cfg.ArtifactDir).
		Msg("ffuuzz started")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	eng.StartReproduceWorker(ctx)

	select {
	case <-ctx.Done():
		logger.Info().Msg("shutdown signal received")
	case srvErr := <-errCh:
		logger.Error().Err(srvErr).Msg("server error, initiating shutdown")
		stop()
	}

	shutCtx, shutCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutCancel()

	// 1. Shutdown API server (stop accepting new requests, drain in-flight)
	apiErr := apiSrv.Shutdown(shutCtx)
	if apiErr != nil {
		logger.Error().Err(apiErr).Msg("api server shutdown error")
	}

	// 2. Stop all running campaigns (no new campaigns can start since API is down)
	eng.StopAll(shutCtx)

	// 3. Shutdown proxy
	proxyErr := proxy.Shutdown(shutCtx)
	if proxyErr != nil {
		logger.Error().Err(proxyErr).Msg("proxy shutdown error")
	}

	normalClose := 0
	forcedClose := 0
	if apiErr == nil {
		normalClose++
	} else {
		forcedClose++
	}
	if proxyErr == nil {
		normalClose++
	} else {
		forcedClose++
	}

	logger.Info().
		Int("normal_close", normalClose).
		Int("forced_close", forcedClose).
		Msg("shutdown complete")

	return 0
}
