package cli

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"ffuuzz/internal/config"
	"ffuuzz/internal/logging"
	"ffuuzz/internal/mitm"
	"ffuuzz/internal/recorder"
	"ffuuzz/internal/store"
)

func (c *CLI) runProxy(args []string) int {
	fs := flag.NewFlagSet("proxy", flag.ExitOnError)

	port := fs.Int("port", 8080, "port to listen on")
	out := fs.String("out", "log.jsonl", "jsonl output file")
	cadir := fs.String("cert-dir", "certs", "certs directory (for root CA and leaf certs)")
	maxBodyKB := fs.Int("maxbodykb", 64, "max body KB to record")

	logger := logging.New(c.stderr)

	if err := fs.Parse(args); err != nil {
		logger.Fatal().Err(err).Msg("parse flags")
	}

	rec, err := recorder.NewJSONL(*out)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize recorder")
	}
	defer func() {
		if err := rec.Close(); err != nil {
			logger.Warn().Err(err).Msg("error closing recorder")
		}
	}()

	certCfg := config.CertCacheConfig{
		MaxEntries: 1000,
		CertDir:    *cadir,
	}
	tlsCfg := config.TLSConfig{}

	cs, err := store.NewCertStore(certCfg, tlsCfg, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create certstore")
	}

	cfg := mitm.Config{
		ListenAddr:    ":" + strconv.Itoa(*port),
		CertStore:     cs,
		Recorder:      rec,
		MaxBodyBytes:  (*maxBodyKB) * 1024,
		TLSSkipVerify: true,
		Logger:        logger,
	}

	logger.Info().Str("addr", cfg.ListenAddr).Str("out", *out).Msg("starting ffuuzz proxy")
	proxy := mitm.New(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)

	go func() {
		if _, err := proxy.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("proxy exited")
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info().Msg("shutting down proxy")
	case srvErr := <-errCh:
		logger.Error().Err(srvErr).Msg("proxy error, initiating shutdown")
		stop()
	}
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()
	if err := proxy.Shutdown(shutCtx); err != nil {
		logger.Warn().Err(err).Msg("proxy shutdown error")
	}

	return 0
}
