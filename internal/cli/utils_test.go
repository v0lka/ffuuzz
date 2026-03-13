package cli

import (
	"testing"
)

func TestSanitizeDSN(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		expected string
	}{
		{
			name:     "postgres with password",
			dsn:      "postgres://user:secret@localhost:5432/dbname",
			expected: "postgres://user:xxxxx@localhost:5432/dbname",
		},
		{
			name:     "mysql with password",
			dsn:      "mysql://user:password@tcp(localhost:3306)/database",
			expected: "<unparseable>", // mysql DSN format is not a valid URL
		},
		{
			name:     "no password",
			dsn:      "postgres://user@localhost:5432/dbname",
			expected: "postgres://user@localhost:5432/dbname",
		},
		{
			name:     "invalid URL",
			dsn:      "://invalid-url",
			expected: "<unparseable>",
		},
		{
			name:     "empty string",
			dsn:      "",
			expected: "",
		},
		{
			name:     "URL with special characters in password",
			dsn:      "postgres://user:p@ss:w0rd!@localhost:5432/dbname",
			expected: "postgres://user:xxxxx@localhost:5432/dbname",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeDSN(tt.dsn)
			if result != tt.expected {
				t.Errorf("sanitizeDSN(%q) = %q, want %q", tt.dsn, result, tt.expected)
			}
		})
	}
}
