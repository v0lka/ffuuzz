package cli

import (
	"net/url"
)

// sanitizeDSN redacts credentials from a database connection URI for safe logging.
func sanitizeDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "<unparseable>"
	}
	return u.Redacted()
}
