package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateUUID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid v4", "550e8400-e29b-41d4-a716-446655440000", true},
		{"valid zeros", "00000000-0000-0000-0000-000000000000", true},
		{"short string", "c1", false},
		{"empty", "", false},
		{"sql injection", "'; DROP TABLE--", false},
		{"path traversal", "../../../etc/passwd", false},
		{"null byte", "550e8400-e29b-41d4-a716-44665544\x000", false},
		{"too long", "550e8400-e29b-41d4-a716-446655440000-extra", false},
		{"no dashes", "550e8400e29b41d4a716446655440000", true}, // uuid.Parse accepts this
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateUUID(tt.input); got != tt.want {
				t.Errorf("validateUUID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRequireUUIDParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("valid UUID", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.GET("/test/:id", func(c *gin.Context) {
			id, ok := requireUUIDParam(c, "id")
			if !ok {
				return
			}
			c.String(http.StatusOK, id)
		})
		c.Request = httptest.NewRequest("GET", "/test/550e8400-e29b-41d4-a716-446655440000", nil)
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("invalid UUID", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, r := gin.CreateTestContext(w)
		r.GET("/test/:id", func(c *gin.Context) {
			_, ok := requireUUIDParam(c, "id")
			if !ok {
				return
			}
			c.String(http.StatusOK, "should not reach")
		})
		c.Request = httptest.NewRequest("GET", "/test/not-a-uuid", nil)
		r.ServeHTTP(w, c.Request)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}

func TestValidateEnumParam(t *testing.T) {
	allowed := []string{"ALPHA", "BETA", "GAMMA"}
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"empty passes", "", true},
		{"valid first", "ALPHA", true},
		{"valid last", "GAMMA", true},
		{"invalid", "DELTA", false},
		{"lowercase", "alpha", false},
		{"sql injection", "'; DROP TABLE", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateEnumParam(tt.value, allowed); got != tt.want {
				t.Errorf("validateEnumParam(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestValidateScheme(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"http", "http", true},
		{"https", "https", true},
		{"ftp", "ftp", false},
		{"file", "file", false},
		{"empty", "", false},
		{"HTTP uppercase", "HTTP", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateScheme(tt.input); got != tt.want {
				t.Errorf("validateScheme(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name string
		port int
		want bool
	}{
		{"zero", 0, false},
		{"one", 1, true},
		{"http", 80, true},
		{"https", 443, true},
		{"max", 65535, true},
		{"over max", 65536, false},
		{"negative", -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validatePort(tt.port); got != tt.want {
				t.Errorf("validatePort(%d) = %v, want %v", tt.port, got, tt.want)
			}
		})
	}
}

func TestValidateStringLen(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   bool
	}{
		{"within", "hello", 10, true},
		{"at limit", "hello", 5, true},
		{"over limit", "hello!", 5, false},
		{"empty", "", 0, true},
		{"empty with limit", "", 10, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateStringLen(tt.input, tt.maxLen); got != tt.want {
				t.Errorf("validateStringLen(%q, %d) = %v, want %v", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no specials", "/api/v1", "/api/v1"},
		{"percent", "100%", `100\%`},
		{"underscore", "my_table", `my\_table`},
		{"backslash", `path\to`, `path\\to`},
		{"all specials", `a%b_c\d`, `a\%b\_c\\d`},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeLikePattern(tt.input); got != tt.want {
				t.Errorf("escapeLikePattern(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsPathContained(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		target string
		want   bool
	}{
		{"normal subpath", "/data/artifacts", "/data/artifacts/campaign1/file.json", true},
		{"traversal escape", "/data/artifacts", "/data/artifacts/../../../etc/passwd", false},
		{"absolute escape", "/data/artifacts", "/etc/passwd", false},
		{"exact base", "/data/artifacts", "/data/artifacts", false}, // base itself is not "under" base
		{"sibling dir", "/data/artifacts", "/data/artifacts2/file.json", false},
		{"clean needed", "/data/artifacts", "/data/artifacts/./campaign1/../campaign1/file.json", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPathContained(tt.base, tt.target); got != tt.want {
				t.Errorf("isPathContained(%q, %q) = %v, want %v", tt.base, tt.target, got, tt.want)
			}
		})
	}
}

func TestValidateRequestID_Valid(t *testing.T) {
	validIDs := []string{
		"abc123",
		"20260316-550e8400-e29b-41d4-a716-446655440000",
		"simple-id",
		"A-B-C-123",
		strings.Repeat("a", 64), // max length
	}
	for _, id := range validIDs {
		if !ValidateRequestID(id) {
			t.Errorf("expected %q to be valid", id)
		}
	}
}

func TestValidateRequestID_Invalid(t *testing.T) {
	invalidIDs := []string{
		"",                      // empty
		strings.Repeat("a", 65), // too long
		"id with spaces",        // spaces
		"id\nwith\nnewlines",    // newlines (log injection)
		"id\r\nwith\r\nCRLF",    // CRLF injection
		"id\twith\ttabs",        // tabs
		"id\x00with\x00null",    // null bytes
		"id\x1bwith\x1besc",     // escape sequences
		"id!@#$%^&*()",          // special characters
		"id/with/slashes",       // slashes
		"id:with:colons",        // colons
	}
	for _, id := range invalidIDs {
		if ValidateRequestID(id) {
			t.Errorf("expected %q to be invalid", id)
		}
	}
}
