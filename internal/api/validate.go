package api

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ffuuzz/internal/model"
)

// requestIDRegex validates the X-Request-ID header format.
// Allows alphanumeric characters and hyphens, with a max length of 64 chars.
// This prevents log injection attacks from malicious header values.
var requestIDRegex = regexp.MustCompile(`^[a-zA-Z0-9\-]{1,64}$`)

// Enum allow-lists derived from model constants.
var (
	validCampaignStatuses = []string{
		string(model.CampaignCreated),
		string(model.CampaignStarting),
		string(model.CampaignRunning),
		string(model.CampaignStopping),
		string(model.CampaignStopped),
		string(model.CampaignFinished),
		string(model.CampaignFailed),
	}
	validFindingTypes = []string{
		string(model.FindingTimeout),
		string(model.FindingServerError),
		string(model.FindingLatencyRegression),
		string(model.FindingRegexMatch),
	}
	validFindingStatuses = []string{
		string(model.FindingUnconfirmed),
		string(model.FindingConfirmed),
	}
)

// validateUUID returns true if s is a valid UUID.
func validateUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// requireUUIDParam extracts a path parameter and validates it as a UUID.
// Returns the value and true on success. On failure it writes a 400 response
// and returns ("", false) so the caller can return early.
func requireUUIDParam(c *gin.Context, paramName string) (string, bool) {
	id := c.Param(paramName)
	if !validateUUID(id) {
		errorResponse(c, http.StatusBadRequest, "INVALID_ID", paramName+" must be a valid UUID")
		return "", false
	}
	return id, true
}

// validateEnumParam returns true if value is empty (no filter) or is one of
// the allowed values.
func validateEnumParam(value string, allowed []string) bool {
	if value == "" {
		return true
	}
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

// validateScheme returns true if s is "http" or "https".
func validateScheme(s string) bool {
	return s == "http" || s == "https"
}

// validatePort returns true if port is in the valid TCP range 1-65535.
func validatePort(port int) bool {
	return port >= 1 && port <= 65535
}

// validateStringLen returns true if the string length does not exceed maxLen.
func validateStringLen(s string, maxLen int) bool {
	return len(s) <= maxLen
}

// escapeLikePattern escapes SQL LIKE special characters (%, _, \) so they are
// treated as literals. The caller must use ESCAPE '\' in the SQL query.
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// isPathContained returns true if target (after cleaning) is a child of base.
// Used to prevent path traversal when serving files.
func isPathContained(base, target string) bool {
	base = filepath.Clean(base)
	target = filepath.Clean(target)
	// Ensure base ends with separator for a strict prefix check.
	if !strings.HasSuffix(base, string(filepath.Separator)) {
		base += string(filepath.Separator)
	}
	return strings.HasPrefix(target, base)
}

// ValidateRequestID checks if the provided request ID is safe for logging.
// Returns true if the ID contains only alphanumeric characters and hyphens,
// and is within the acceptable length range (1-64 characters).
func ValidateRequestID(id string) bool {
	return requestIDRegex.MatchString(id)
}
