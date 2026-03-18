// Package api implements the Control API server for managing campaigns,
// recordings, findings, and real-time SSE streaming.
package api

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"ffuuzz/internal/engine"
	"ffuuzz/internal/httputil"
	"ffuuzz/internal/metrics"
	"ffuuzz/internal/model"
)

// RecordingStore defines the recording operations needed by the API.
type RecordingStore interface {
	GetByID(ctx context.Context, id string, includeEntries bool, maxBodyBytes int) (*model.RecordingSession, error)
	GetByIDs(ctx context.Context, ids []string) ([]model.RecordingSession, error)
	Upsert(ctx context.Context, sess model.RecordingSession) (bool, error)
	List(ctx context.Context, limit, offset int, hostFilter, pathPrefix string) ([]model.RecordingSession, error)
	ListAll(ctx context.Context, hostFilter, pathPrefix string) ([]model.RecordingSession, error)
	Delete(ctx context.Context, id string) (bool, error)
	IsUsedByActiveCampaign(ctx context.Context, id string) (bool, error)
	GetTree(ctx context.Context) ([]model.TreeEntry, error)
	DeleteByPrefix(ctx context.Context, scheme, host string, port int, pathPrefix string) (int64, error)
}

// CampaignStore defines the campaign operations needed by the API.
type CampaignStore interface {
	GetByID(ctx context.Context, id string) (*model.Campaign, error)
	Create(ctx context.Context, c model.Campaign) error
	List(ctx context.Context, statusFilter string, limit, offset int) ([]model.Campaign, error)
	AddRecordingsByFilter(ctx context.Context, campaignID, scheme, host string, port int, pathPrefix string) (int, error)
}

// FindingStore defines the finding operations needed by the API.
type FindingStore interface {
	ListAll(ctx context.Context, campaignID, typeFilter, statusFilter string, since *time.Time, limit, offset int) ([]model.Finding, error)
	GetByID(ctx context.Context, id string) (*model.Finding, error)
	UpdateReproduceStatus(ctx context.Context, id, status string, runs int) error
	CountByType(ctx context.Context, campaignID string) (map[model.FindingType]int, error)
}

// ArtifactStore defines the artifact operations needed by the API.
type ArtifactStore interface {
	GetByFindingID(ctx context.Context, findingID string) (*model.Artifact, error)
}

// HealthChecker provides database health checking.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// Server wraps a Gin engine and all dependencies needed by API handlers.
type Server struct {
	router      *gin.Engine
	httpServer  *http.Server
	recordings  RecordingStore
	campaigns   CampaignStore
	findings    FindingStore
	artifacts   ArtifactStore
	engine      *engine.Engine
	health      HealthChecker
	artifactDir string
	webFS       fs.FS
	logger      zerolog.Logger
}

// ServerConfig bundles all dependencies for the API server.
type ServerConfig struct {
	Addr        string
	Recordings  RecordingStore
	Campaigns   CampaignStore
	Findings    FindingStore
	Artifacts   ArtifactStore
	Engine      *engine.Engine
	Health      HealthChecker
	ArtifactDir string
	WebFS       fs.FS // Embedded SPA assets (nil = SPA disabled)
	Logger      zerolog.Logger
}

// NewServer creates a fully configured Gin-based API server.
func NewServer(cfg ServerConfig) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	s := &Server{
		router:      router,
		recordings:  cfg.Recordings,
		campaigns:   cfg.Campaigns,
		findings:    cfg.Findings,
		artifacts:   cfg.Artifacts,
		engine:      cfg.Engine,
		health:      cfg.Health,
		artifactDir: cfg.ArtifactDir,
		webFS:       cfg.WebFS,
		logger:      cfg.Logger,
	}

	// Middleware: X-Request-ID injection + request logging
	router.Use(s.requestIDMiddleware())
	router.Use(s.loggingMiddleware())

	// Health & metrics
	router.GET("/healthz", s.healthz)
	router.GET("/metrics", s.metricsHandler())

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Recordings
		v1.POST("/recordings/import", s.importRecordings)
		v1.GET("/recordings/tree", s.getRecordingsTree)
		v1.GET("/recordings/export", s.exportRecordings)
		v1.GET("/recordings", s.listRecordings)
		v1.GET("/recordings/:id", s.getRecording)
		v1.DELETE("/recordings/by-prefix", s.deleteRecordingsByPrefix)
		v1.DELETE("/recordings/:id", s.deleteRecording)

		// Campaigns
		v1.POST("/campaigns", s.createCampaign)
		v1.GET("/campaigns", s.listCampaigns)
		v1.GET("/campaigns/:id", s.getCampaign)
		v1.GET("/campaigns/:id/stats", s.getCampaignStats)
		v1.GET("/campaigns/:id/findings", s.getCampaignFindings)
		v1.GET("/campaigns/:id/config", s.getCampaignConfig)
		v1.GET("/campaigns/:id/stream", s.streamCampaignStats)
		v1.POST("/campaigns/:id/start", s.startCampaign)
		v1.POST("/campaigns/:id/stop", s.stopCampaign)
		v1.POST("/campaigns/:id/recordings", s.addRecordingsToCampaign)

		// Findings
		v1.GET("/findings", s.listFindings)
		v1.GET("/findings/:id", s.getFinding)
		v1.GET("/findings/:id/artifact", s.getFindingArtifact)
		v1.POST("/findings/:id/reproduce", s.reproduceFinding)
	}

	// Root redirect → SPA
	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/ui/")
	})

	// Embedded SPA
	router.GET("/ui/*filepath", s.spaHandler)

	s.httpServer = &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

// ListenAndServe starts the API server.
func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the API server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" || !ValidateRequestID(reqID) {
			reqID = httputil.NewRequestID()
		}
		c.Set("request_id", reqID)
		c.Header("X-Request-ID", reqID)
		c.Next()
	}
}

func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		reqID, _ := c.Get("request_id")
		rid, _ := reqID.(string)
		s.logger.Info().
			Str("request_id", rid).
			Int("status", c.Writer.Status()).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Msg("api request")
	}
}

func (s *Server) healthz(c *gin.Context) {
	status := "ok"
	dbStatus := "ok"

	if err := s.health.Ping(c.Request.Context()); err != nil {
		status = "degraded"
		dbStatus = classifyDBError(err)
		s.logger.Error().Err(err).Msg("health check: database ping failed")
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  status,
			"db":      dbStatus,
			"version": "0.1.0",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  status,
		"db":      dbStatus,
		"version": "0.1.0",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// classifyDBError returns a sanitized error category without exposing internal details.
func classifyDBError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	msg := err.Error()
	if strings.Contains(msg, "connection refused") {
		return "connection_refused"
	}
	if strings.Contains(msg, "connection reset") {
		return "connection_reset"
	}
	return "unavailable"
}

func (s *Server) metricsHandler() gin.HandlerFunc {
	h := promhttp.HandlerFor(metrics.Registry(), promhttp.HandlerOpts{})
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func errorResponse(c *gin.Context, status int, code, message string) {
	reqID, _ := c.Get("request_id")
	rid, _ := reqID.(string)
	c.JSON(status, gin.H{
		"error":      code,
		"message":    message,
		"request_id": rid,
	})
}

// internalError logs the real error and returns a generic message to the client.
func (s *Server) internalError(c *gin.Context, code string, err error) {
	reqID, _ := c.Get("request_id")
	rid, _ := reqID.(string)
	s.logger.Error().Err(err).Str("request_id", rid).Str("error_code", code).Msg("internal error")
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":      code,
		"message":    "internal server error",
		"request_id": rid,
	})
}

// parsePagination extracts limit and offset query parameters with defaults (50, 0).
// If parsing fails, defaults are used and a warning is logged.
// Limit is capped at 50 to prevent OOM attacks.
func (s *Server) parsePagination(c *gin.Context) (limit, offset int) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil {
		s.logger.Warn().Err(err).Str("param", "limit").Msg("invalid pagination parameter, using default")
		limit = 50
	}
	if limit > 50 {
		s.logger.Warn().Int("limit", limit).Msg("pagination limit exceeds maximum, capping at 50")
		limit = 50
	}
	offset, err = strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil {
		s.logger.Warn().Err(err).Str("param", "offset").Msg("invalid pagination parameter, using default")
		offset = 0
	}
	const maxOffset = 1_000_000
	if offset > maxOffset {
		s.logger.Warn().Int("offset", offset).Msg("pagination offset exceeds maximum, capping at 1000000")
		offset = maxOffset
	}
	return limit, offset
}

// parseSinceParam extracts the "since" query parameter as an RFC3339 time.
// Returns (nil, nil) if the parameter is not present.
// Returns (nil, error) if the parameter is present but has invalid format.
func parseSinceParam(c *gin.Context) (*time.Time, error) {
	sinceStr := c.Query("since")
	if sinceStr == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, sinceStr)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
