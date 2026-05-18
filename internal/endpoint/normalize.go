// Package endpoint infers URL endpoint patterns from observed traffic and
// manages recording grouping by those patterns.
package endpoint

import (
	"regexp"
	"strings"
	"unicode"
)

// Placeholder is the universal placeholder for parameterised path segments.
const Placeholder = "{_}"

// Origin identifies an HTTP origin by its scheme, host, and port.
type Origin struct {
	Scheme string
	Host   string
	Port   int
}

// Compiled regexes for heuristic segment classification.
var (
	numericRe = regexp.MustCompile(`^\d+$`)
	uuidRe    = regexp.MustCompile(
		`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
	)
	hexRe       = regexp.MustCompile(`^[0-9a-fA-F]{8,}$`)
	tokenCharRe = regexp.MustCompile(`^[A-Za-z0-9+/=_-]{16,}$`)
	// Matches filenames like app.a1b2c3d4.js  or  style.4f8e2a.css
	hashedFileRe = regexp.MustCompile(`^[^.]+\.([0-9a-fA-F]{6,})\.[^.]+$`)
)

// NormalizePath replaces path segments that look like parameters with the
// universal placeholder {_}. The function is idempotent: normalising an
// already-normalised path returns the same result.
func NormalizePath(path string) string {
	if path == "" || path == "/" {
		return path
	}

	// Preserve the leading slash.
	prefix := ""
	if strings.HasPrefix(path, "/") {
		prefix = "/"
		path = path[1:]
	}

	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if seg == "" || seg == Placeholder {
			continue
		}
		if isParameter(seg) {
			segments[i] = Placeholder
		}
	}

	return prefix + strings.Join(segments, "/")
}

// isParameter returns true if the segment looks like a variable parameter
// rather than a fixed resource name.
func isParameter(seg string) bool {
	// 1. Pure numeric ID
	if numericRe.MatchString(seg) {
		return true
	}

	// 2. UUID
	if uuidRe.MatchString(seg) {
		return true
	}

	// 3. Hex hash ≥ 8 chars (but not a short word like "deadbeef")
	if hexRe.MatchString(seg) {
		return true
	}

	// 4. Content-hashed filename (e.g. app.a1b2c3d4.js)
	if hashedFileRe.MatchString(seg) {
		return true
	}

	// 5. High-entropy token ≥ 16 chars with mixed character classes
	if tokenCharRe.MatchString(seg) && hasMixedClasses(seg) {
		return true
	}

	return false
}

// hasMixedClasses returns true if s contains characters from at least two of:
// uppercase letters, lowercase letters, digits.
func hasMixedClasses(s string) bool {
	var hasUpper, hasLower, hasDigit bool
	for _, r := range s {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	classes := 0
	if hasUpper {
		classes++
	}
	if hasLower {
		classes++
	}
	if hasDigit {
		classes++
	}
	return classes >= 2
}

// SplitPathSegments splits a URL path into non-empty segments, discarding
// empty strings between consecutive slashes.
func SplitPathSegments(path string) []string {
	var segments []string
	for _, s := range strings.Split(path, "/") {
		if s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}
