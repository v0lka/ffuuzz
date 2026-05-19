package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"ffuuzz/internal/model"
	"ffuuzz/internal/triage"
)

// analyzeFinding triggers LLM analysis for a single finding.
func (s *Server) analyzeFinding(c *gin.Context) {
	if s.llmTriager == nil {
		errorResponse(c, http.StatusServiceUnavailable, "LLM_DISABLED", "LLM-assisted triage is not configured")
		return
	}

	findingID := c.Param("id")
	if !validateUUID(findingID) {
		errorResponse(c, http.StatusBadRequest, "INVALID_ID", "finding id must be a valid UUID")
		return
	}

	finding, err := s.findings.GetByID(c.Request.Context(), findingID)
	if err != nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "finding not found")
		return
	}

	var artifactPayload *model.ArtifactPayload
	if finding.ArtifactID != "" {
		artifact, err := s.artifacts.GetByFindingID(c.Request.Context(), finding.ID)
		if err != nil {
			s.logger.Warn().Err(err).Str("finding_id", finding.ID).Msg("llm: load artifact failed")
		} else {
			filePath := filepath.Join(s.artifactDir, artifact.FilePath)
			data, err := os.ReadFile(filePath)
			if err != nil {
				s.logger.Warn().Err(err).Str("finding_id", finding.ID).Msg("llm: read artifact file failed")
			} else {
				var payload model.ArtifactPayload
				if err := json.Unmarshal(data, &payload); err != nil {
					s.logger.Warn().Err(err).Str("finding_id", finding.ID).Msg("llm: unmarshal artifact failed")
				} else {
					artifactPayload = &payload
				}
			}
		}
	}

	analysis, err := s.llmTriager.AnalyzeFinding(c.Request.Context(), finding, artifactPayload)
	if err != nil {
		s.internalError(c, "LLM_ANALYSIS_FAILED", err)
		return
	}
	if analysis == nil {
		errorResponse(c, http.StatusInternalServerError, "LLM_EMPTY", "llm returned no analysis")
		return
	}

	jsonData, err := triage.MarshalAnalysis(analysis)
	if err != nil {
		s.internalError(c, "MARSHAL_FAILED", err)
		return
	}
	if err := s.findings.UpdateLLMAnalysis(c.Request.Context(), finding.ID, jsonData); err != nil {
		s.internalError(c, "PERSIST_FAILED", err)
		return
	}

	c.JSON(http.StatusOK, analysis)
}

// analyzeCampaign starts async batch LLM analysis for unconfirmed findings in a campaign.
// The analysis runs in a goroutine with a 10-minute timeout and persists results
// to the database as they arrive.
func (s *Server) analyzeCampaign(c *gin.Context) {
	if s.llmTriager == nil {
		errorResponse(c, http.StatusServiceUnavailable, "LLM_DISABLED", "LLM-assisted triage is not configured")
		return
	}

	campaignID := c.Param("id")
	if !validateUUID(campaignID) {
		errorResponse(c, http.StatusBadRequest, "INVALID_ID", "campaign id must be a valid UUID")
		return
	}

	findings, err := s.findings.ListAll(c.Request.Context(), campaignID, "", string(model.FindingUnconfirmed), nil, 10000, 0)
	if err != nil {
		s.internalError(c, "LIST_FAILED", err)
		return
	}

	if len(findings) == 0 {
		c.JSON(http.StatusOK, gin.H{"analyzed": 0, "message": "no unconfirmed findings to analyze"})
		return
	}

	artifactGetter := func(findingID string) (*model.ArtifactPayload, error) {
		artifact, err := s.artifacts.GetByFindingID(context.Background(), findingID)
		if err != nil {
			return nil, err
		}
		filePath := filepath.Join(s.artifactDir, artifact.FilePath)
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		var payload model.ArtifactPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		return &payload, nil
	}

	s.logger.Info().Int("count", len(findings)).Str("campaign_id", campaignID).Msg("starting async llm batch analysis")

	// Run analysis in background; respond 202 immediately.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		var mu sync.Mutex
		analyzed := 0
		s.llmTriager.BatchAnalyze(ctx, findings, artifactGetter, func(findingID string, analysis *model.LLMAnalysis) {
			jsonData, err := triage.MarshalAnalysis(analysis)
			if err != nil {
				s.logger.Warn().Err(err).Str("finding_id", findingID).Msg("llm batch: marshal analysis failed")
				return
			}
			if err := s.findings.UpdateLLMAnalysis(context.Background(), findingID, jsonData); err != nil {
				s.logger.Warn().Err(err).Str("finding_id", findingID).Msg("llm batch: persist analysis failed")
			} else {
				mu.Lock()
				analyzed++
				mu.Unlock()
			}
		})
		s.logger.Info().Int("analyzed", analyzed).Int("total", len(findings)).Str("campaign_id", campaignID).Msg("async llm batch analysis complete")
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message": "analysis started",
		"total":   len(findings),
	})
}
