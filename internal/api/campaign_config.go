package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// getCampaignConfig returns the configuration of a campaign.
func (s *Server) getCampaignConfig(c *gin.Context) {
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

	c.JSON(http.StatusOK, campaign.Config)
}
