// Package anomaly implements detectors that identify abnormal responses during fuzzing.
package anomaly

import (
	"context"
	"errors"
	"net"
	"os"
	"regexp"
	"strings"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
	"ffuuzz/internal/replayer"
)

// AnomalyHit represents a detected anomaly from a single exchange.
//
// Detector implementations populate Type, Method, Endpoint, Details, Exchange
// and ResultBody. The worker enriches each hit after detection with
// per-session context (HitExchangeIndex, OriginalRequest, OpsByExchange) so
// the original request endpoint is preserved and so the artifact payload can
// pinpoint which exchange triggered the finding.
type AnomalyHit struct {
	Type       model.FindingType
	Method     string
	Endpoint   string
	Details    model.FindingDetails
	Exchange   model.Exchange
	ResultBody []byte

	// Filled by the worker (not by detectors):
	OriginalRequest  model.RequestData // pre-mutation request for diff-based payload
	HitExchangeIndex int               // index of this hit's exchange in the session
	OpsByExchange    [][]string        // operators applied per exchange across the session
}

// BaselineEntry holds per-endpoint baseline data.
type BaselineEntry struct {
	Method     string
	Endpoint   string
	P50Ms      int64
	StatusCode int
}

// Detector checks a replay result against baseline for anomalies.
type Detector interface {
	Detect(ex model.Exchange, result replayer.ExchangeResult, baseline *BaselineEntry, cfg model.AnomalyConfig) []AnomalyHit
}

// TimeoutDetector flags exchanges that exceeded the request timeout.
type TimeoutDetector struct{}

func (d *TimeoutDetector) Detect(ex model.Exchange, result replayer.ExchangeResult, _ *BaselineEntry, cfg model.AnomalyConfig) []AnomalyHit {
	if result.Err == nil {
		return nil
	}

	if isTimeoutError(result.Err) {
		return []AnomalyHit{{
			Type:     model.FindingTimeout,
			Method:   ex.Request.Method,
			Endpoint: ex.Request.Path,
			Details: model.FindingDetails{
				ObservedMs: result.DurationMs,
			},
			Exchange:   ex,
			ResultBody: result.RespBody,
		}}
	}
	return nil
}

// ServerErrorDetector flags exchanges that returned HTTP 5xx status codes.
type ServerErrorDetector struct{}

func (d *ServerErrorDetector) Detect(ex model.Exchange, result replayer.ExchangeResult, baseline *BaselineEntry, cfg model.AnomalyConfig) []AnomalyHit {
	if !cfg.Detect5xx || result.Err != nil {
		return nil
	}
	if result.StatusCode < 500 {
		return nil
	}
	// Only flag if baseline was not a 5xx
	if baseline != nil && baseline.StatusCode >= 500 {
		return nil
	}

	return []AnomalyHit{{
		Type:     model.FindingServerError,
		Method:   ex.Request.Method,
		Endpoint: ex.Request.Path,
		Details: model.FindingDetails{
			HTTPStatus: result.StatusCode,
			ObservedMs: result.DurationMs,
		},
		Exchange:   ex,
		ResultBody: result.RespBody,
	}}
}

// LatencyDetector flags exchanges whose latency exceeds the baseline by a configured multiplier.
type LatencyDetector struct{}

func (d *LatencyDetector) Detect(ex model.Exchange, result replayer.ExchangeResult, baseline *BaselineEntry, cfg model.AnomalyConfig) []AnomalyHit {
	if result.Err != nil || baseline == nil {
		return nil
	}
	multiplier := cfg.LatencyMultiplier
	if multiplier <= 0 {
		return nil
	}
	threshold := int64(float64(baseline.P50Ms) * multiplier)
	if result.DurationMs <= threshold {
		return nil
	}

	return []AnomalyHit{{
		Type:     model.FindingLatencyRegression,
		Method:   ex.Request.Method,
		Endpoint: ex.Request.Path,
		Details: model.FindingDetails{
			BaselineMs: baseline.P50Ms,
			ObservedMs: result.DurationMs,
		},
		Exchange:   ex,
		ResultBody: result.RespBody,
	}}
}

// RegexDetector flags exchanges whose response body matches configured patterns.
type RegexDetector struct {
	compiled []*regexp.Regexp
}

// NewRegexDetector compiles regex patterns once at campaign start.
// Invalid patterns are logged and skipped.
func NewRegexDetector(patterns []string, logger zerolog.Logger) *RegexDetector {
	d := &RegexDetector{}
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			logger.Warn().Str("pattern", p).Err(err).Msg("invalid regex pattern, skipping")
			continue
		}
		d.compiled = append(d.compiled, re)
	}
	return d
}

func (d *RegexDetector) Detect(ex model.Exchange, result replayer.ExchangeResult, _ *BaselineEntry, _ model.AnomalyConfig) []AnomalyHit {
	if result.Err != nil || len(d.compiled) == 0 || len(result.RespBody) == 0 {
		return nil
	}

	for _, re := range d.compiled {
		loc := re.FindIndex(result.RespBody)
		if loc == nil {
			continue
		}
		return []AnomalyHit{{
			Type:     model.FindingRegexMatch,
			Method:   ex.Request.Method,
			Endpoint: ex.Request.Path,
			Details: model.FindingDetails{
				HTTPStatus:    result.StatusCode,
				ObservedMs:    result.DurationMs,
				RegexPattern:  re.String(),
				MatchOffset:   loc[0],
				MatchSnippet:  matchSnippet(result.RespBody, loc[0], loc[1]),
				BodyTotalSize: len(result.RespBody),
			},
			Exchange:   ex,
			ResultBody: result.RespBody,
		}}
	}
	return nil
}

// matchSnippetWindow controls how many bytes around a regex match are stored
// in FindingDetails.MatchSnippet so analysts can quickly see context without
// reading the full artifact body.
const matchSnippetWindow = 256

// matchSnippet returns up to matchSnippetWindow bytes around the [start, end)
// match location, truncating safely at the body boundaries.
func matchSnippet(body []byte, start, end int) string {
	if start < 0 || start > len(body) {
		return ""
	}
	if end < start {
		end = start
	}
	if end > len(body) {
		end = len(body)
	}
	from := start - matchSnippetWindow/2
	if from < 0 {
		from = 0
	}
	to := end + matchSnippetWindow/2
	if to > len(body) {
		to = len(body)
	}
	return string(body[from:to])
}

// MultiDetector runs all enabled detectors and collects results.
type MultiDetector struct {
	detectors []Detector
}

// NewMultiDetector creates a MultiDetector with all enabled anomaly detectors
// based on the supplied configuration.
func NewMultiDetector(cfg model.AnomalyConfig, logger zerolog.Logger) *MultiDetector {
	md := &MultiDetector{}
	// Always check for timeouts
	md.detectors = append(md.detectors, &TimeoutDetector{})
	if cfg.Detect5xx {
		md.detectors = append(md.detectors, &ServerErrorDetector{})
	}
	if cfg.LatencyMultiplier > 0 {
		md.detectors = append(md.detectors, &LatencyDetector{})
	}
	if len(cfg.RegexPatterns) > 0 {
		md.detectors = append(md.detectors, NewRegexDetector(cfg.RegexPatterns, logger))
	}
	return md
}

func (md *MultiDetector) Detect(ex model.Exchange, result replayer.ExchangeResult, baseline *BaselineEntry, cfg model.AnomalyConfig) []AnomalyHit {
	var hits []AnomalyHit
	for _, d := range md.detectors {
		hits = append(hits, d.Detect(ex, result, baseline, cfg)...)
	}
	return hits
}

// isTimeoutError checks whether an error represents a timeout condition,
// using standard library checks with a string-based fallback for edge cases.
func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return true
	}
	if os.IsTimeout(err) {
		return true
	}
	return strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline")
}
