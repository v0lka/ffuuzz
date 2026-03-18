package anomaly

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
	"ffuuzz/internal/replayer"
)

func makeExchange(method, path string) model.Exchange {
	return model.Exchange{
		RequestID: "req-1",
		Request: model.RequestData{
			Method: method,
			Path:   path,
		},
	}
}

func makeResult(statusCode int, durationMs int64, err error, body []byte) replayer.ExchangeResult {
	return replayer.ExchangeResult{
		StatusCode: statusCode,
		DurationMs: durationMs,
		Err:        err,
		RespBody:   body,
	}
}

func TestTimeoutDetector_NoError(t *testing.T) {
	d := &TimeoutDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 50, nil, nil)
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits, got %d", len(hits))
	}
}

func TestTimeoutDetector_DeadlineExceeded(t *testing.T) {
	d := &TimeoutDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(0, 3000, context.DeadlineExceeded, nil)
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Type != model.FindingTimeout {
		t.Errorf("expected type %s, got %s", model.FindingTimeout, hits[0].Type)
	}
}

func TestTimeoutDetector_NetTimeout(t *testing.T) {
	d := &TimeoutDetector{}
	ex := makeExchange("POST", "/api/submit")
	netErr := &net.DNSError{IsTimeout: true}
	result := makeResult(0, 5000, netErr, nil)
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
}

func TestTimeoutDetector_TimeoutString(t *testing.T) {
	d := &TimeoutDetector{}
	ex := makeExchange("GET", "/slow")
	result := makeResult(0, 5000, errors.New("connection timeout exceeded"), nil)
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit for 'timeout' string error, got %d", len(hits))
	}
}

func TestTimeoutDetector_DeadlineString(t *testing.T) {
	d := &TimeoutDetector{}
	ex := makeExchange("GET", "/slow")
	result := makeResult(0, 5000, errors.New("deadline hit"), nil)
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit for 'deadline' string error, got %d", len(hits))
	}
}

func TestTimeoutDetector_NonTimeoutError(t *testing.T) {
	d := &TimeoutDetector{}
	ex := makeExchange("GET", "/fail")
	result := makeResult(0, 100, errors.New("connection refused"), nil)
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits for non-timeout error, got %d", len(hits))
	}
}

func TestServerErrorDetector_500(t *testing.T) {
	d := &ServerErrorDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(500, 50, nil, nil)
	cfg := model.AnomalyConfig{Detect5xx: true}
	hits := d.Detect(ex, result, nil, cfg)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Type != model.FindingServerError {
		t.Errorf("expected type %s, got %s", model.FindingServerError, hits[0].Type)
	}
	if hits[0].Details.HTTPStatus != 500 {
		t.Errorf("expected HTTPStatus 500, got %d", hits[0].Details.HTTPStatus)
	}
}

func TestServerErrorDetector_503(t *testing.T) {
	d := &ServerErrorDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(503, 50, nil, nil)
	cfg := model.AnomalyConfig{Detect5xx: true}
	hits := d.Detect(ex, result, nil, cfg)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit for 503, got %d", len(hits))
	}
}

func TestServerErrorDetector_Disabled(t *testing.T) {
	d := &ServerErrorDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(500, 50, nil, nil)
	cfg := model.AnomalyConfig{Detect5xx: false}
	hits := d.Detect(ex, result, nil, cfg)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits when disabled, got %d", len(hits))
	}
}

func TestServerErrorDetector_ErrorInResult(t *testing.T) {
	d := &ServerErrorDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(500, 50, errors.New("err"), nil)
	cfg := model.AnomalyConfig{Detect5xx: true}
	hits := d.Detect(ex, result, nil, cfg)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits when result has error, got %d", len(hits))
	}
}

func TestServerErrorDetector_Non5xx(t *testing.T) {
	d := &ServerErrorDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(400, 50, nil, nil)
	cfg := model.AnomalyConfig{Detect5xx: true}
	hits := d.Detect(ex, result, nil, cfg)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits for 400, got %d", len(hits))
	}
}

func TestServerErrorDetector_BaselineAlso5xx(t *testing.T) {
	d := &ServerErrorDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(500, 50, nil, nil)
	baseline := &BaselineEntry{StatusCode: 502}
	cfg := model.AnomalyConfig{Detect5xx: true}
	hits := d.Detect(ex, result, baseline, cfg)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits when baseline also 5xx, got %d", len(hits))
	}
}

func TestServerErrorDetector_BaselineNon5xx(t *testing.T) {
	d := &ServerErrorDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(500, 50, nil, nil)
	baseline := &BaselineEntry{StatusCode: 200}
	cfg := model.AnomalyConfig{Detect5xx: true}
	hits := d.Detect(ex, result, baseline, cfg)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit when baseline is 200, got %d", len(hits))
	}
}

func TestLatencyDetector_AboveThreshold(t *testing.T) {
	d := &LatencyDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 500, nil, nil)
	baseline := &BaselineEntry{P50Ms: 100}
	cfg := model.AnomalyConfig{LatencyMultiplier: 2.0}
	hits := d.Detect(ex, result, baseline, cfg)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Type != model.FindingLatencyRegression {
		t.Errorf("expected type %s, got %s", model.FindingLatencyRegression, hits[0].Type)
	}
	if hits[0].Details.BaselineMs != 100 {
		t.Errorf("expected BaselineMs 100, got %d", hits[0].Details.BaselineMs)
	}
}

func TestLatencyDetector_BelowThreshold(t *testing.T) {
	d := &LatencyDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 150, nil, nil)
	baseline := &BaselineEntry{P50Ms: 100}
	cfg := model.AnomalyConfig{LatencyMultiplier: 2.0}
	hits := d.Detect(ex, result, baseline, cfg)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits when below threshold, got %d", len(hits))
	}
}

func TestLatencyDetector_ExactThreshold(t *testing.T) {
	d := &LatencyDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 200, nil, nil)
	baseline := &BaselineEntry{P50Ms: 100}
	cfg := model.AnomalyConfig{LatencyMultiplier: 2.0}
	hits := d.Detect(ex, result, baseline, cfg)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits at exact threshold, got %d", len(hits))
	}
}

func TestLatencyDetector_NoBaseline(t *testing.T) {
	d := &LatencyDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 9999, nil, nil)
	cfg := model.AnomalyConfig{LatencyMultiplier: 2.0}
	hits := d.Detect(ex, result, nil, cfg)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits without baseline, got %d", len(hits))
	}
}

func TestLatencyDetector_ErrorResult(t *testing.T) {
	d := &LatencyDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(0, 9999, errors.New("err"), nil)
	baseline := &BaselineEntry{P50Ms: 100}
	cfg := model.AnomalyConfig{LatencyMultiplier: 2.0}
	hits := d.Detect(ex, result, baseline, cfg)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits on error result, got %d", len(hits))
	}
}

func TestLatencyDetector_ZeroMultiplier(t *testing.T) {
	d := &LatencyDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 9999, nil, nil)
	baseline := &BaselineEntry{P50Ms: 100}
	cfg := model.AnomalyConfig{LatencyMultiplier: 0}
	hits := d.Detect(ex, result, baseline, cfg)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits with 0 multiplier, got %d", len(hits))
	}
}

func TestLatencyDetector_NegativeMultiplier(t *testing.T) {
	d := &LatencyDetector{}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 9999, nil, nil)
	baseline := &BaselineEntry{P50Ms: 100}
	cfg := model.AnomalyConfig{LatencyMultiplier: -1.0}
	hits := d.Detect(ex, result, baseline, cfg)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits with negative multiplier, got %d", len(hits))
	}
}

func TestRegexDetector_Match(t *testing.T) {
	d := NewRegexDetector([]string{`error|fatal`}, zerolog.Nop())
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 50, nil, []byte("fatal exception occurred"))
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Type != model.FindingRegexMatch {
		t.Errorf("expected type %s, got %s", model.FindingRegexMatch, hits[0].Type)
	}
}

func TestRegexDetector_NoMatch(t *testing.T) {
	d := NewRegexDetector([]string{`error|fatal`}, zerolog.Nop())
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 50, nil, []byte("everything is fine"))
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits, got %d", len(hits))
	}
}

func TestRegexDetector_EmptyBody(t *testing.T) {
	d := NewRegexDetector([]string{`error`}, zerolog.Nop())
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 50, nil, nil)
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits for empty body, got %d", len(hits))
	}
}

func TestRegexDetector_ErrorResult(t *testing.T) {
	d := NewRegexDetector([]string{`error`}, zerolog.Nop())
	ex := makeExchange("GET", "/api/test")
	result := makeResult(0, 0, errors.New("err"), []byte("error in body"))
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits when result has error, got %d", len(hits))
	}
}

func TestRegexDetector_NoPatterns(t *testing.T) {
	d := NewRegexDetector(nil, zerolog.Nop())
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 50, nil, []byte("error"))
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits with no patterns, got %d", len(hits))
	}
}

func TestRegexDetector_InvalidPattern(t *testing.T) {
	d := NewRegexDetector([]string{`[invalid`}, zerolog.Nop())
	if len(d.compiled) != 0 {
		t.Fatalf("expected 0 compiled patterns for invalid regex, got %d", len(d.compiled))
	}
}

func TestRegexDetector_MultiplePatterns(t *testing.T) {
	d := NewRegexDetector([]string{`error`, `fatal`, `panic`}, zerolog.Nop())
	if len(d.compiled) != 3 {
		t.Fatalf("expected 3 compiled patterns, got %d", len(d.compiled))
	}
	ex := makeExchange("GET", "/api/test")
	result := makeResult(200, 50, nil, []byte("a panic happened"))
	hits := d.Detect(ex, result, nil, model.AnomalyConfig{})
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit (first match), got %d", len(hits))
	}
}

func TestMultiDetector_AllEnabled(t *testing.T) {
	cfg := model.AnomalyConfig{
		Detect5xx:         true,
		LatencyMultiplier: 2.0,
		RegexPatterns:     []string{`error`},
	}
	md := NewMultiDetector(cfg, zerolog.Nop())
	// Timeout detector + ServerError + Latency + Regex = 4
	if len(md.detectors) != 4 {
		t.Fatalf("expected 4 detectors, got %d", len(md.detectors))
	}
}

func TestMultiDetector_OnlyTimeout(t *testing.T) {
	cfg := model.AnomalyConfig{}
	md := NewMultiDetector(cfg, zerolog.Nop())
	if len(md.detectors) != 1 {
		t.Fatalf("expected 1 detector (timeout), got %d", len(md.detectors))
	}
}

func TestMultiDetector_CombinesHits(t *testing.T) {
	cfg := model.AnomalyConfig{Detect5xx: true}
	md := NewMultiDetector(cfg, zerolog.Nop())
	ex := makeExchange("GET", "/api/test")
	// Timeout error that also has status 500 won't produce server error hit
	// because result.Err != nil filters it. So test timeout only.
	result := makeResult(0, 5000, context.DeadlineExceeded, nil)
	hits := md.Detect(ex, result, nil, cfg)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit (timeout), got %d", len(hits))
	}
}

func TestMultiDetector_ServerErrorHit(t *testing.T) {
	cfg := model.AnomalyConfig{Detect5xx: true}
	md := NewMultiDetector(cfg, zerolog.Nop())
	ex := makeExchange("GET", "/api/test")
	result := makeResult(500, 50, nil, nil)
	hits := md.Detect(ex, result, nil, cfg)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"timeout string", errors.New("i/o timeout"), true},
		{"deadline string", errors.New("context deadline"), true},
		{"not timeout", errors.New("connection refused"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTimeoutError(tt.err)
			if got != tt.want {
				t.Errorf("isTimeoutError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
