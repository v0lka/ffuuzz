package endpoint

import "ffuuzz/internal/model"

// Key uniquely identifies a logical endpoint as the combination of an HTTP
// method and a path normalised via NormalizePath. It is the canonical key for
// per-endpoint state such as baselines, scheduling counters, and metrics.
//
// Two requests with the same method but paths that differ only in
// parameter-like segments (numeric IDs, UUIDs, content hashes, etc.) produce
// the same Key.
type Key struct {
	Method string
	Path   string
}

// NewKey returns a Key for the given method and path. The path is normalised
// via NormalizePath, so equivalent parametric paths collapse onto a single
// Key. The method is stored verbatim; callers that wish to be
// case-insensitive should upper-case it before constructing the Key.
func NewKey(method, path string) Key {
	return Key{Method: method, Path: NormalizePath(path)}
}

// KeyFromExchange returns the Key for an HTTP exchange. The path component is
// taken from the request and normalised via NormalizePath.
func KeyFromExchange(ex model.Exchange) Key {
	return NewKey(ex.Request.Method, ex.Request.Path)
}

// String returns a stable textual representation of the Key in the form
// "METHOD|PATH". It is used as a map key for stores that have not been
// migrated to use Key directly (notably baselines).
func (k Key) String() string {
	return k.Method + "|" + k.Path
}
