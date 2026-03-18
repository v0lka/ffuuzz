package endpoint

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/metrics"
)

// Merger is the interface that the DB layer implements for recording merge
// operations triggered by endpoint pattern collapses.
type Merger interface {
	// MergeRecordings updates target_path for all recordings whose target_path
	// starts with one of the given source prefixes, replacing the collapsed
	// segment with {_}. It skips recordings linked to active campaigns.
	// Returns the number of recordings updated.
	MergeRecordings(ctx context.Context, origin Origin, sourcePrefixes []string, targetPrefix string) (int, error)

	// ListDistinctPaths returns all distinct target_path values for the given
	// origin. Used during startup to rebuild tries.
	ListDistinctPaths(ctx context.Context, origin Origin) ([]string, error)

	// ListOrigins returns all distinct (scheme, host, port) tuples present in
	// the recordings table. Used during startup.
	ListOrigins(ctx context.Context) ([]Origin, error)
}

// mergeTimeout is the maximum time allowed for a single merge operation.
const mergeTimeout = 60 * time.Second

// Resolver manages per-origin segment tries and detects when observed paths
// should be collapsed into parameterised endpoint patterns. When a collapse is
// detected the Resolver rewrites affected DB recordings asynchronously.
type Resolver struct {
	mu     sync.Mutex
	tries  map[Origin]*trieNode
	merger Merger
	logger zerolog.Logger
}

// NewResolver creates a Resolver that will use the given Merger for DB
// operations. The tries are empty; call RebuildFromDB to populate them.
func NewResolver(merger Merger, logger zerolog.Logger) *Resolver {
	return &Resolver{
		tries:  make(map[Origin]*trieNode),
		merger: merger,
		logger: logger.With().Str("component", "endpoint.resolver").Logger(),
	}
}

// ObservePath records a (already heuristically-normalised) path for the given
// origin. If the observation triggers a statistical collapse, the Resolver
// fires an async merge and returns the (possibly rewritten) pattern path.
func (r *Resolver) ObservePath(origin Origin, normalizedPath string) string {
	segments := SplitPathSegments(normalizedPath)
	if len(segments) == 0 {
		return normalizedPath
	}

	r.mu.Lock()
	root, ok := r.tries[origin]
	if !ok {
		root = newTrieNode()
		r.tries[origin] = root
	}
	root.observe(segments)

	result, shouldCollapse := root.checkCollapse()
	if !shouldCollapse {
		r.mu.Unlock()
		return normalizedPath
	}

	// Execute the collapse on the trie while still holding the lock.
	collapsedKeys := root.collapse(result.ParentSegments)
	r.mu.Unlock()

	if len(collapsedKeys) == 0 {
		return normalizedPath
	}

	sourcePrefixes, targetPrefix := collapsedPatterns(result.ParentSegments, collapsedKeys)

	r.logger.Info().
		Strs("source_prefixes", sourcePrefixes).
		Str("target_prefix", targetPrefix).
		Str("host", origin.Host).
		Msg("endpoint collapse detected, scheduling merge")

	metrics.EndpointCollapses.Inc()

	// Fire-and-forget async merge. Errors are logged but do not block the
	// caller (which is in the recording hot-path).
	go r.executeMerge(origin, sourcePrefixes, targetPrefix)

	// Rewrite the current path: if the current path matches one of the
	// collapsed source prefixes, replace the collapsed segment.
	return rewritePath(segments, result.Position, normalizedPath)
}

// RebuildFromDB populates the per-origin tries from existing DB recordings.
// This should be called once at startup before the proxy starts accepting
// traffic. Any detected collapses are executed synchronously.
func (r *Resolver) RebuildFromDB(ctx context.Context) error {
	origins, err := r.merger.ListOrigins(ctx)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, origin := range origins {
		paths, err := r.merger.ListDistinctPaths(ctx, origin)
		if err != nil {
			r.logger.Error().Err(err).
				Str("host", origin.Host).
				Msg("failed to list paths for origin, skipping")
			continue
		}

		root := newTrieNode()
		for _, p := range paths {
			segments := SplitPathSegments(p)
			if len(segments) > 0 {
				root.observe(segments)
			}
		}

		// Drain all detectable collapses.
		for {
			result, ok := root.checkCollapse()
			if !ok {
				break
			}
			collapsedKeys := root.collapse(result.ParentSegments)
			if len(collapsedKeys) == 0 {
				break
			}
			sourcePrefixes, targetPrefix := collapsedPatterns(result.ParentSegments, collapsedKeys)

			r.logger.Info().
				Strs("source_prefixes", sourcePrefixes).
				Str("target_prefix", targetPrefix).
				Str("host", origin.Host).
				Msg("startup collapse detected, merging synchronously")

			mergeCtx, cancel := context.WithTimeout(ctx, mergeTimeout)
			n, mergeErr := r.merger.MergeRecordings(mergeCtx, origin, sourcePrefixes, targetPrefix)
			cancel()
			if mergeErr != nil {
				r.logger.Error().Err(mergeErr).
					Str("host", origin.Host).
					Str("target", targetPrefix).
					Msg("startup merge failed")
			} else if n > 0 {
				r.logger.Info().
					Int("merged", n).
					Str("target", targetPrefix).
					Msg("startup merge completed")
			}
		}

		r.tries[origin] = root
	}

	return nil
}

// executeMerge runs a single merge and then drains any cascading collapses.
func (r *Resolver) executeMerge(origin Origin, sourcePrefixes []string, targetPrefix string) {
	ctx, cancel := context.WithTimeout(context.Background(), mergeTimeout)
	defer cancel()

	n, err := r.merger.MergeRecordings(ctx, origin, sourcePrefixes, targetPrefix)
	if err != nil {
		r.logger.Error().Err(err).
			Strs("sources", sourcePrefixes).
			Str("target", targetPrefix).
			Msg("async merge failed")
		return
	}

	metrics.EndpointMerges.Add(float64(n))

	r.logger.Info().
		Int("merged", n).
		Str("target", targetPrefix).
		Msg("async merge completed")

	// Check for cascading collapses after the merge.
	r.drainCollapses(origin)
}

// drainCollapses checks and executes any further collapses in the origin trie.
func (r *Resolver) drainCollapses(origin Origin) {
	for {
		r.mu.Lock()
		root, ok := r.tries[origin]
		if !ok {
			r.mu.Unlock()
			return
		}
		result, shouldCollapse := root.checkCollapse()
		if !shouldCollapse {
			r.mu.Unlock()
			return
		}
		collapsedKeys := root.collapse(result.ParentSegments)
		r.mu.Unlock()

		if len(collapsedKeys) == 0 {
			return
		}

		sourcePrefixes, targetPrefix := collapsedPatterns(result.ParentSegments, collapsedKeys)

		r.logger.Info().
			Strs("source_prefixes", sourcePrefixes).
			Str("target_prefix", targetPrefix).
			Msg("cascading collapse detected")

		ctx, cancel := context.WithTimeout(context.Background(), mergeTimeout)
		n, err := r.merger.MergeRecordings(ctx, origin, sourcePrefixes, targetPrefix)
		cancel()

		if err != nil {
			r.logger.Error().Err(err).
				Str("target", targetPrefix).
				Msg("cascading merge failed")
			return
		}
		if n > 0 {
			r.logger.Info().Int("merged", n).Str("target", targetPrefix).Msg("cascading merge completed")
		}
	}
}

// rewritePath replaces the segment at the given position with {_} and
// reconstructs the path with a leading slash.
func rewritePath(segments []string, position int, originalPath string) string {
	if position >= len(segments) {
		return originalPath
	}

	// Build new segments with the collapsed position replaced.
	out := make([]string, len(segments))
	copy(out, segments)
	out[position] = Placeholder

	return "/" + joinSegments(out)
}

// joinSegments joins non-empty segments with "/".
func joinSegments(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	result := segments[0]
	for _, s := range segments[1:] {
		result += "/" + s
	}
	return result
}
