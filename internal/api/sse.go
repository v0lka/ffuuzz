package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ffuuzz/internal/model"
)

// streamCampaignStats implements GET /api/v1/campaigns/:id/stream as an SSE endpoint.
func (s *Server) streamCampaignStats(c *gin.Context) {
	id := c.Param("id")

	// Verify campaign exists.
	campaign, err := s.campaigns.GetByID(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "GET_FAILED", err)
		return
	}
	if campaign == nil {
		errorResponse(c, http.StatusNotFound, "NOT_FOUND", "campaign not found")
		return
	}

	// Set SSE headers.
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // disable nginx buffering
	c.Status(http.StatusOK)
	c.Writer.Flush()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	ctx := c.Request.Context()

	// Send an initial event immediately.
	if done := s.sendStatsEvent(c, id); done {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if done := s.sendStatsEvent(c, id); done {
				return
			}
		}
	}
}

// sendStatsEvent fetches current stats and writes a single SSE data frame.
// Returns true if the stream should end (terminal campaign status or write error).
func (s *Server) sendStatsEvent(c *gin.Context, campaignID string) bool {
	stats, err := s.buildCampaignStats(c.Request.Context(), campaignID)
	if err != nil {
		s.logger.Warn().Err(err).Str("campaign_id", campaignID).Msg("sse: failed to build stats")
		return false // non-fatal, try again on next tick
	}
	if stats == nil {
		return true // campaign deleted
	}

	data, err := json.Marshal(stats)
	if err != nil {
		s.logger.Warn().Err(err).Msg("sse: marshal error")
		return false
	}

	// Check for terminal status to send a done event.
	terminal := isTerminalStatus(stats.Status)

	if terminal {
		_, _ = fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", data)
		c.Writer.Flush()
		return true
	}

	_, writeErr := fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	c.Writer.Flush()
	return writeErr != nil
}

func isTerminalStatus(s model.CampaignStatus) bool {
	switch s {
	case model.CampaignFinished, model.CampaignFailed, model.CampaignStopped:
		return true
	default:
		return false
	}
}
