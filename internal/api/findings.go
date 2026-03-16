package api

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"ffuuzz/internal/model"
)

func (s *Server) listFindings(c *gin.Context) {
	limit, offset := s.parsePagination(c)
	campaignID := c.Query("campaign_id")
	if campaignID != "" && !validateUUID(campaignID) {
		errorResponse(c, http.StatusBadRequest, "INVALID_CAMPAIGN_ID", "campaign_id must be a valid UUID")
		return
	}
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

	findings, err := s.findings.ListAll(c.Request.Context(), campaignID, typeFilter, statusFilter, since, limit, offset)
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

func (s *Server) getFinding(c *gin.Context) {
	id, ok := requireUUIDParam(c, "id")
	if !ok {
		return
	}

	finding, err := s.findings.GetByID(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "GET_FAILED", err)
		return
	}
	if finding == nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "finding not found")
		return
	}

	c.JSON(http.StatusOK, finding)
}

func (s *Server) getFindingArtifact(c *gin.Context) {
	id, ok := requireUUIDParam(c, "id")
	if !ok {
		return
	}

	artifact, err := s.artifacts.GetByFindingID(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "GET_FAILED", err)
		return
	}
	if artifact == nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "artifact not found for this finding")
		return
	}

	filePath := filepath.Join(s.artifactDir, artifact.FilePath)
	if !isPathContained(s.artifactDir, filePath) {
		errorResponse(c, http.StatusBadRequest, "INVALID_PATH", "artifact path is invalid")
		return
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		s.internalError(c, "READ_FAILED", err)
		return
	}

	c.Data(http.StatusOK, "application/json", data)
}

type reproduceRequest struct {
	Runs int `json:"runs"`
}

func (s *Server) reproduceFinding(c *gin.Context) {
	id, ok := requireUUIDParam(c, "id")
	if !ok {
		return
	}

	var req reproduceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Default to 3 runs if not specified
		req.Runs = 3
	}

	if req.Runs < 1 || req.Runs > 20 {
		errorResponse(c, http.StatusUnprocessableEntity, "INVALID_RUNS", "runs must be between 1 and 20")
		return
	}

	finding, err := s.findings.GetByID(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "GET_FAILED", err)
		return
	}
	if finding == nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "finding not found")
		return
	}

	// Check if already enqueued/running
	if finding.ReproduceStatus == string(model.ReproduceEnqueued) || finding.ReproduceStatus == string(model.ReproduceRunning) {
		errorResponse(c, http.StatusConflict, "ALREADY_QUEUED", "reproduction already "+finding.ReproduceStatus)
		return
	}

	if err := s.findings.UpdateReproduceStatus(c.Request.Context(), id, string(model.ReproduceEnqueued), req.Runs); err != nil {
		s.internalError(c, "UPDATE_FAILED", err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"finding_id":       id,
		"reproduce_status": string(model.ReproduceEnqueued),
		"runs":             req.Runs,
		"enqueued_at":      time.Now().UTC().Format(time.RFC3339),
	})
}
