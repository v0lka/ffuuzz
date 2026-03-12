// Package replayer sends mutated HTTP requests to the target and collects responses.
package replayer

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

// ExchangeResult holds the outcome of replaying a single exchange.
type ExchangeResult struct {
	Exchange    model.Exchange // the exchange as sent (after substitutions)
	StatusCode  int
	RespHeaders http.Header
	RespBody    []byte
	DurationMs  int64
	Err         error
}

// Replayer replays exchanges against a target, optionally using a WorkerContext
// for stateful operations (cookies, variables).
type Replayer struct {
	DefaultClient *http.Client
	logger        zerolog.Logger
}

// New creates a Replayer with a default HTTP client.
func New(client *http.Client, logger zerolog.Logger) *Replayer {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Replayer{DefaultClient: client, logger: logger}
}

// ReplayExchange sends a single exchange and returns the result.
func (r *Replayer) ReplayExchange(ctx context.Context, ex model.Exchange, baseURL string, wctx *WorkerContext) ExchangeResult {
	// Apply variable substitutions if context exists
	if wctx != nil {
		wctx.ApplySubstitutions(&ex)
	}

	// Build URL
	fullURL := baseURL + ex.Request.Path
	if ex.Request.Query != "" {
		fullURL += "?" + ex.Request.Query
	}

	// Build request body
	var bodyReader io.Reader
	if ex.Request.BodyB64 != "" {
		bodyBytes, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
		if err == nil && len(bodyBytes) > 0 {
			bodyReader = bytes.NewReader(bodyBytes)
		}
	}

	req, err := http.NewRequestWithContext(ctx, ex.Request.Method, fullURL, bodyReader)
	if err != nil {
		return ExchangeResult{Exchange: ex, Err: err}
	}

	// Set headers
	for k, vv := range ex.Request.Headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	// Disable compression to ensure readable response bodies
	// (Go's http.Client doesn't auto-decompress Brotli, only gzip)
	req.Header.Set("Accept-Encoding", "identity")

	// Choose client
	client := r.DefaultClient
	if wctx != nil && wctx.Client != nil {
		client = wctx.Client
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return ExchangeResult{
			Exchange:   ex,
			DurationMs: elapsed.Milliseconds(),
			Err:        err,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		r.logger.Debug().Err(err).Str("url", fullURL).Msg("read response body failed")
		body = nil
	}

	// Update cookies and extract variables
	if wctx != nil {
		reqURL, _ := url.Parse(fullURL)
		if reqURL != nil {
			wctx.UpdateCookies(resp, reqURL)
		}
	}

	return ExchangeResult{
		Exchange:    ex,
		StatusCode:  resp.StatusCode,
		RespHeaders: resp.Header.Clone(),
		RespBody:    body,
		DurationMs:  elapsed.Milliseconds(),
	}
}

// ReplaySession replays all exchanges in a session sequentially.
func (r *Replayer) ReplaySession(ctx context.Context, session model.RecordingSession, baseURL string, wctx *WorkerContext, extractionRules []ExtractionRule) ([]ExchangeResult, error) {
	results := make([]ExchangeResult, 0, len(session.Entries))

	for _, ex := range session.Entries {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result := r.ReplayExchange(ctx, ex, baseURL, wctx)
		results = append(results, result)

		// Extract variables from the response for subsequent requests
		if wctx != nil && result.Err == nil && len(extractionRules) > 0 {
			// Build a minimal http.Response for extraction using actual response headers
			resp := &http.Response{
				StatusCode: result.StatusCode,
				Header:     result.RespHeaders,
			}
			if resp.Header == nil {
				resp.Header = make(http.Header)
			}
			wctx.ExtractVariables(resp, result.RespBody, extractionRules)
		}

		// Stop on error if it's a hard failure (timeout/connection error)
		if result.Err != nil {
			break
		}
	}

	return results, nil
}
