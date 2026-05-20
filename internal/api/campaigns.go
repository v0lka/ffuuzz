package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ffuuzz/internal/endpoint"
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

	if !validateStringLen(req.Name, 255) {
		errorResponse(c, http.StatusBadRequest, "NAME_TOO_LONG", "name must not exceed 255 characters")
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
	if err := validateCampaignConfig(&req.Config); err != nil {
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
	id, ok := requireUUIDParam(c, "id")
	if !ok {
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
	id, ok := requireUUIDParam(c, "id")
	if !ok {
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
	limit, offset := s.parsePagination(c)
	status := c.Query("status")
	if !validateEnumParam(status, validCampaignStatuses) {
		errorResponse(c, http.StatusBadRequest, "INVALID_STATUS", "status must be one of: CREATED, STARTING, RUNNING, STOPPING, STOPPED, FINISHED, FAILED")
		return
	}

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
	id, ok := requireUUIDParam(c, "id")
	if !ok {
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

	c.JSON(http.StatusOK, campaign)
}

func (s *Server) getCampaignStats(c *gin.Context) {
	id, ok := requireUUIDParam(c, "id")
	if !ok {
		return
	}

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
		var elapsed float64
		if campaign.FinishedAt != nil {
			elapsed = campaign.FinishedAt.Sub(*campaign.StartedAt).Seconds()
		} else {
			elapsed = time.Since(*campaign.StartedAt).Seconds()
		}
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
	sessions, err := s.recordings.GetByIDs(ctx, campaign.RecordingIDs)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to get recordings for stats")
	}
	stats.Seeds.SessionsUsed = len(sessions)
	for _, sess := range sessions {
		stats.Seeds.ExchangesSent += sess.EntryCount
	}

	return &stats, nil
}

func (s *Server) getCampaignFindings(c *gin.Context) {
	id, ok := requireUUIDParam(c, "id")
	if !ok {
		return
	}

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

	limit, offset := s.parsePagination(c)
	typeFilter := c.Query("type")
	if !validateEnumParam(typeFilter, validFindingTypes) {
		errorResponse(c, http.StatusBadRequest, "INVALID_TYPE", "type must be one of: TIMEOUT, SERVER_ERROR, LATENCY_REGRESSION, REGEX_MATCH")
		return
	}
	statusFilter := c.Query("status")
	if !validateEnumParam(statusFilter, validFindingStatuses) {
		errorResponse(c, http.StatusBadRequest, "INVALID_STATUS", "status must be one of: UNCONFIRMED, CONFIRMED")
		return
	}
	since, err := parseSinceParam(c)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "INVALID_SINCE", "invalid since parameter: expected RFC3339 format")
		return
	}

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

// validateCampaignConfig checks that campaign config parameters are logically
// valid. It also normalises EndpointWeights (uppercasing Method and applying
// endpoint.NormalizePath to Path), mutating cfg in place via the pointer.
func validateCampaignConfig(cfg *model.CampaignConfig) error {
	u, err := url.Parse(cfg.Target.BaseURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("target.base_url must be a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("target.base_url scheme must be http or https")
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
	if cfg.Limits.MinTestsPerEndpoint < 0 {
		return fmt.Errorf("limits.min_tests_per_endpoint must be >= 0")
	}
	if cfg.Limits.SequenceShare < 0 || cfg.Limits.SequenceShare > 1 {
		return fmt.Errorf("limits.sequence_share must be between 0 and 1")
	}
	for i := range cfg.Limits.EndpointWeights {
		ew := &cfg.Limits.EndpointWeights[i]
		if ew.Path == "" {
			return fmt.Errorf("limits.endpoint_weights[%d].path must not be empty", i)
		}
		if ew.Weight < 0 {
			return fmt.Errorf("limits.endpoint_weights[%d].weight must be >= 0", i)
		}
		if ew.Method != "" {
			method := strings.ToUpper(ew.Method)
			if !validHTTPMethod(method) {
				return fmt.Errorf("limits.endpoint_weights[%d].method %q is not a valid HTTP method", i, ew.Method)
			}
			ew.Method = method
		}
		ew.Path = endpoint.NormalizePath(ew.Path)
	}
	return nil
}

// validHTTPMethod reports whether m is a recognised HTTP method.
func validHTTPMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions,
		http.MethodConnect, http.MethodTrace:
		return true
	}
	return false
}

type addRecordingsRequest struct {
	Scheme     string `json:"scheme" binding:"required"`
	Host       string `json:"host" binding:"required"`
	Port       int    `json:"port" binding:"required"`
	PathPrefix string `json:"path_prefix"`
}

func (s *Server) addRecordingsToCampaign(c *gin.Context) {
	id, ok := requireUUIDParam(c, "id")
	if !ok {
		return
	}

	var req addRecordingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	if !validateScheme(req.Scheme) {
		errorResponse(c, http.StatusBadRequest, "INVALID_SCHEME", "scheme must be http or https")
		return
	}
	if !validatePort(req.Port) {
		errorResponse(c, http.StatusBadRequest, "INVALID_PORT", "port must be between 1 and 65535")
		return
	}
	if !validateStringLen(req.Host, 253) {
		errorResponse(c, http.StatusBadRequest, "INVALID_HOST", "host must not exceed 253 characters")
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

	added, err := s.campaigns.AddRecordingsByFilter(c.Request.Context(), id, req.Scheme, req.Host, req.Port, escapeLikePattern(req.PathPrefix))
	if err != nil {
		s.internalError(c, "ADD_RECORDINGS_FAILED", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"added": added})
}

type editCampaignRequest struct {
	Name         *string               `json:"name"`
	RecordingIDs *[]string             `json:"recording_ids"`
	Config       *model.CampaignConfig `json:"config"`
}

func (s *Server) editCampaign(c *gin.Context) {
	id, ok := requireUUIDParam(c, "id")
	if !ok {
		return
	}

	var req editCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	if req.Name == nil && req.RecordingIDs == nil && req.Config == nil {
		errorResponse(c, http.StatusBadRequest, "INVALID_BODY", "at least one field (name, recording_ids, or config) is required")
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

	switch campaign.Status {
	case model.CampaignRunning, model.CampaignStarting, model.CampaignStopping:
		errorResponse(c, http.StatusConflict, "INVALID_STATE", "cannot edit campaign in state: "+string(campaign.Status))
		return
	case model.CampaignCreated, model.CampaignStopped, model.CampaignFinished, model.CampaignFailed:
		// valid, can edit
	default:
		errorResponse(c, http.StatusUnprocessableEntity, "INVALID_STATE", "cannot edit campaign in state: "+string(campaign.Status))
		return
	}

	if req.Name != nil {
		if !validateStringLen(*req.Name, 255) {
			errorResponse(c, http.StatusBadRequest, "NAME_TOO_LONG", "name must not exceed 255 characters")
			return
		}
		campaign.Name = *req.Name
	}

	if req.RecordingIDs != nil {
		var firstSession *model.RecordingSession
		for _, rid := range *req.RecordingIDs {
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
		campaign.RecordingIDs = *req.RecordingIDs
	}

	if req.Config != nil {
		if err := validateCampaignConfig(req.Config); err != nil {
			errorResponse(c, http.StatusUnprocessableEntity, "INVALID_CONFIG", err.Error())
			return
		}
		campaign.Config = *req.Config
	}

	campaign.UpdatedAt = time.Now().UTC()

	if err := s.campaigns.Update(c.Request.Context(), *campaign); err != nil {
		s.internalError(c, "UPDATE_FAILED", err)
		return
	}

	c.JSON(http.StatusOK, campaign)
}

type quickCreateCampaignRequest struct {
	Name   string              `json:"name" binding:"required"`
	Filter addRecordingsRequest `json:"filter" binding:"required"`
}

func (s *Server) quickCreateCampaign(c *gin.Context) {
	var req quickCreateCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	if !validateStringLen(req.Name, 255) {
		errorResponse(c, http.StatusBadRequest, "NAME_TOO_LONG", "name must not exceed 255 characters")
		return
	}
	if !validateScheme(req.Filter.Scheme) {
		errorResponse(c, http.StatusBadRequest, "INVALID_SCHEME", "scheme must be http or https")
		return
	}
	if !validatePort(req.Filter.Port) {
		errorResponse(c, http.StatusBadRequest, "INVALID_PORT", "port must be between 1 and 65535")
		return
	}
	if !validateStringLen(req.Filter.Host, 253) {
		errorResponse(c, http.StatusBadRequest, "INVALID_HOST", "host must not exceed 253 characters")
		return
	}

	now := time.Now().UTC()
	campaign := model.Campaign{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Status:    model.CampaignCreated,
		CreatedAt: now,
		UpdatedAt: now,
		Config: model.CampaignConfig{
			Target: model.TargetURL{
				BaseURL: fmt.Sprintf("%s://%s:%d", req.Filter.Scheme, req.Filter.Host, req.Filter.Port),
			},
			Limits: model.CampaignLimits{
				Workers:      8,
				RPS:          50,
				MaxTests:     10000,
				ReqTimeoutMs: 3000,
			},
			Mutations: model.MutationConfig{
				PathQuery: true,
				Headers:   true,
				JSONBody:  true,
				Params:    true,
				Intensity: 0.6,
			},
			Anomaly: model.AnomalyConfig{
				Detect5xx:         true,
				LatencyMultiplier: 3.0,
			},
			Triage: model.TriageConfig{
				ConfirmRuns:        3,
				EnableMinimization: true,
			},
		},
	}

	added, err := s.campaigns.CreateWithFilter(
		c.Request.Context(),
		campaign,
		req.Filter.Scheme,
		req.Filter.Host,
		req.Filter.Port,
		escapeLikePattern(req.Filter.PathPrefix),
	)
	if err != nil {
		if err.Error() == "no recordings match the filter" {
			errorResponse(c, http.StatusBadRequest, "NO_RECORDINGS", "no recordings match the specified filter")
			return
		}
		s.internalError(c, "CREATE_FAILED", err)
		return
	}

	campaign.RecordingIDs = make([]string, 0, added)
	campaign.Progress = &model.CampaignProgress{}
	c.JSON(http.StatusCreated, campaign)
}
