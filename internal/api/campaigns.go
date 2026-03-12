package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ffuuzz/internal/model"
)

type createCampaignRequest struct {
	Name         string               `json:"name" binding:"required"`
	RecordingIDs []string             `json:"recording_ids" binding:"required"`
	Config       model.CampaignConfig `json:"config" binding:"required"`
}

func (s *Server) createCampaign(c *gin.Context) {
	var req createCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	if len(req.RecordingIDs) == 0 {
		errorResponse(c, http.StatusBadRequest, "NO_RECORDINGS", "recording_ids must not be empty")
		return
	}

	// Validate recording IDs exist
	var firstSession *model.RecordingSession
	for _, rid := range req.RecordingIDs {
		sess, err := s.recordings.GetByID(c.Request.Context(), rid, false, 0)
		if err != nil {
			s.internalError(c, "CHECK_FAILED", err)
			return
		}
		if sess == nil {
			errorResponse(c, http.StatusNotFound, "RECORDING_NOT_FOUND", "recording not found: "+rid)
			return
		}
		if firstSession == nil {
			firstSession = sess
		}
	}

	// Auto-derive base_url from the first recording when not provided
	if req.Config.Target.BaseURL == "" && firstSession != nil {
		t := firstSession.Target
		req.Config.Target.BaseURL = fmt.Sprintf("%s://%s:%d", t.Scheme, t.Host, t.Port)
	}

	// Validate campaign config constraints
	if err := validateCampaignConfig(req.Config); err != nil {
		errorResponse(c, http.StatusUnprocessableEntity, "INVALID_CONFIG", err.Error())
		return
	}

	now := time.Now().UTC()
	campaign := model.Campaign{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Status:       model.CampaignCreated,
		CreatedAt:    now,
		UpdatedAt:    now,
		RecordingIDs: req.RecordingIDs,
		Config:       req.Config,
	}

	if err := s.campaigns.Create(c.Request.Context(), campaign); err != nil {
		s.internalError(c, "CREATE_FAILED", err)
		return
	}

	c.JSON(http.StatusCreated, campaign)
}

func (s *Server) startCampaign(c *gin.Context) {
	id := c.Param("id")

	campaign, err := s.campaigns.GetByID(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "GET_FAILED", err)
		return
	}
	if campaign == nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "campaign not found")
		return
	}

	// Check valid starting states
	switch campaign.Status {
	case model.CampaignRunning, model.CampaignStarting:
		errorResponse(c, http.StatusConflict, "ALREADY_RUNNING", "campaign is already "+string(campaign.Status))
		return
	case model.CampaignCreated, model.CampaignStopped, model.CampaignFinished, model.CampaignFailed:
		// valid, can start
	default:
		errorResponse(c, http.StatusUnprocessableEntity, "INVALID_STATE", "cannot start campaign in state: "+string(campaign.Status))
		return
	}

	if err := s.engine.StartCampaign(c.Request.Context(), campaign); err != nil {
		s.internalError(c, "START_FAILED", err)
		return
	}

	campaign.Status = model.CampaignStarting
	c.JSON(http.StatusAccepted, campaign)
}

func (s *Server) stopCampaign(c *gin.Context) {
	id := c.Param("id")

	campaign, err := s.campaigns.GetByID(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "GET_FAILED", err)
		return
	}
	if campaign == nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "campaign not found")
		return
	}

	if campaign.Status != model.CampaignRunning && campaign.Status != model.CampaignStarting {
		errorResponse(c, http.StatusConflict, "NOT_RUNNING", "campaign is not running (status: "+string(campaign.Status)+")")
		return
	}

	if err := s.engine.StopCampaign(c.Request.Context(), id); err != nil {
		s.internalError(c, "STOP_FAILED", err)
		return
	}

	campaign.Status = model.CampaignStopping
	c.JSON(http.StatusAccepted, campaign)
}

func (s *Server) listCampaigns(c *gin.Context) {
	limit, offset := parsePagination(c)
	status := c.Query("status")

	campaigns, err := s.campaigns.List(c.Request.Context(), status, limit, offset)
	if err != nil {
		s.internalError(c, "LIST_FAILED", err)
		return
	}

	if len(campaigns) == 0 {
		c.Status(http.StatusNoContent)
		return
	}

	c.JSON(http.StatusOK, campaigns)
}

func (s *Server) getCampaign(c *gin.Context) {
	id := c.Param("id")

	campaign, err := s.campaigns.GetByID(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "GET_FAILED", err)
		return
	}
	if campaign == nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "campaign not found")
		return
	}

	c.JSON(http.StatusOK, campaign)
}

func (s *Server) getCampaignStats(c *gin.Context) {
	id := c.Param("id")

	stats, err := s.buildCampaignStats(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "STATS_FAILED", err)
		return
	}
	if stats == nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "campaign not found")
		return
	}

	c.JSON(http.StatusOK, stats)
}

// buildCampaignStats computes aggregated statistics for a campaign.
// Returns nil, nil if the campaign does not exist.
func (s *Server) buildCampaignStats(ctx context.Context, id string) (*model.CampaignStats, error) {
	campaign, err := s.campaigns.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if campaign == nil {
		return nil, nil
	}

	// Count findings by type
	counts, err := s.findings.CountByType(ctx, id)
	if err != nil {
		return nil, err
	}

	stats := model.CampaignStats{
		CampaignID:     id,
		Status:         campaign.Status,
		TestsTotal:     campaign.TestsDone,
		LastActivityAt: campaign.UpdatedAt,
	}

	if campaign.StartedAt != nil && campaign.TestsDone > 0 {
		elapsed := time.Since(*campaign.StartedAt).Seconds()
		if elapsed > 0 {
			stats.TestsPerSec = float64(campaign.TestsDone) / elapsed
		}
	}

	stats.Timeouts = counts[model.FindingTimeout]
	stats.ServerErrors = counts[model.FindingServerError]
	stats.LatencyRegressions = counts[model.FindingLatencyRegression]
	stats.RegexMatches = counts[model.FindingRegexMatch]

	stats.Seeds.SessionsTotal = len(campaign.RecordingIDs)

	// Compute seeds.sessions_used and seeds.exchanges_sent from recording data
	sessionsUsed := 0
	exchangesSent := 0
	for _, rid := range campaign.RecordingIDs {
		sess, err := s.recordings.GetByID(ctx, rid, false, 0)
		if err == nil && sess != nil {
			sessionsUsed++
			exchangesSent += sess.EntryCount
		}
	}
	stats.Seeds.SessionsUsed = sessionsUsed
	stats.Seeds.ExchangesSent = exchangesSent

	return &stats, nil
}

func (s *Server) getCampaignFindings(c *gin.Context) {
	id := c.Param("id")

	// Verify campaign exists
	campaign, err := s.campaigns.GetByID(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "GET_FAILED", err)
		return
	}
	if campaign == nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "campaign not found")
		return
	}

	limit, offset := parsePagination(c)
	typeFilter := c.Query("type")
	statusFilter := c.Query("status")
	since := parseSinceParam(c)

	findings, err := s.findings.ListAll(c.Request.Context(), id, typeFilter, statusFilter, since, limit, offset)
	if err != nil {
		s.internalError(c, "LIST_FAILED", err)
		return
	}

	if len(findings) == 0 {
		c.Status(http.StatusNoContent)
		return
	}

	c.JSON(http.StatusOK, findings)
}

// validateCampaignConfig checks that campaign config parameters are logically valid.
func validateCampaignConfig(cfg model.CampaignConfig) error {
	if cfg.Target.BaseURL == "" {
		return fmt.Errorf("target.base_url must not be empty")
	}
	if cfg.Limits.Workers <= 0 {
		return fmt.Errorf("limits.workers must be > 0")
	}
	if cfg.Limits.RPS <= 0 {
		return fmt.Errorf("limits.rps must be > 0")
	}
	if cfg.Limits.ReqTimeoutMs <= 0 {
		return fmt.Errorf("limits.req_timeout_ms must be > 0")
	}
	if cfg.Limits.DurationSec <= 0 && cfg.Limits.MaxTests <= 0 {
		return fmt.Errorf("at least one of limits.duration_sec or limits.max_tests must be > 0")
	}
	if cfg.Mutations.Intensity < 0 || cfg.Mutations.Intensity > 1 {
		return fmt.Errorf("mutations.intensity must be between 0 and 1")
	}
	return nil
}

type addRecordingsRequest struct {
	Scheme     string `json:"scheme" binding:"required"`
	Host       string `json:"host" binding:"required"`
	Port       int    `json:"port" binding:"required"`
	PathPrefix string `json:"path_prefix"`
}

func (s *Server) addRecordingsToCampaign(c *gin.Context) {
	id := c.Param("id")

	var req addRecordingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	campaign, err := s.campaigns.GetByID(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "GET_FAILED", err)
		return
	}
	if campaign == nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "campaign not found")
		return
	}

	if campaign.Status == model.CampaignRunning || campaign.Status == model.CampaignStarting {
		errorResponse(c, http.StatusConflict, "CAMPAIGN_ACTIVE", "cannot add recordings to a running campaign")
		return
	}

	added, err := s.campaigns.AddRecordingsByFilter(c.Request.Context(), id, req.Scheme, req.Host, req.Port, req.PathPrefix)
	if err != nil {
		s.internalError(c, "ADD_RECORDINGS_FAILED", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"added": added})
}
