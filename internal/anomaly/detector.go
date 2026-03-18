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
type AnomalyHit struct {
	Type       model.FindingType
	Method     string
	Endpoint   string
	Details    model.FindingDetails
	Exchange   model.Exchange
	ResultBody []byte
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
		if re.Match(result.RespBody) {
			return []AnomalyHit{{
				Type:     model.FindingRegexMatch,
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
	}
	return nil
}

// MultiDetector runs all enabled detectors and collects results.
type MultiDetector struct {
	detectors []Detector
}

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
