// Package triage confirms and minimises findings by replaying them against the target.
package triage

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/anomaly"
	"ffuuzz/internal/model"
	"ffuuzz/internal/replayer"
)

const maxNestedDepth = 5

var (
	numericIDPattern = regexp.MustCompile(`/\d+(/|$)`)
	uuidPattern      = regexp.MustCompile(`/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}(/|$)`)
)

// Triager handles deduplication, confirmation, and minimization of findings.
type Triager struct{}

func NewTriager() *Triager {
	return &Triager{}
}

// Signature computes a deduplication signature for an anomaly hit.
// Format: TYPE|METHOD|normalizedPath|hash(payload)
func (t *Triager) Signature(hit anomaly.AnomalyHit) string {
	normalizedPath := NormalizePath(hit.Endpoint)
	payloadHash := HashPayload(hit.Exchange.Request.BodyB64)
	return fmt.Sprintf("%s|%s|%s|%s", hit.Type, hit.Method, normalizedPath, payloadHash)
}

// fuzzSegmentThreshold is the minimum segment length considered a fuzz payload.
// Real REST path segments (resource names, short tokens) are well under this.
const fuzzSegmentThreshold = 64

// NormalizePath replaces numeric IDs, UUIDs, and long fuzz payloads with placeholders.
func NormalizePath(path string) string {
	path = uuidPattern.ReplaceAllString(path, "/{uuid}$1")
	path = numericIDPattern.ReplaceAllString(path, "/{id}$1")

	// Normalize long path segments that are fuzz payloads (e.g. AAAA...).
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if len(seg) >= fuzzSegmentThreshold {
			segments[i] = "{fuzz}"
		}
	}
	return strings.Join(segments, "/")
}

// HashPayload produces a short hash of the request body for dedup.
// For JSON bodies, it sorts keys to normalize structure.
// For non-JSON, it hashes the raw decoded content.
func HashPayload(bodyB64 string) string {
	if bodyB64 == "" {
		return "empty"
	}

	// Decode base64 first, then try JSON normalization
	decoded, decErr := base64.StdEncoding.DecodeString(bodyB64)
	var rawBytes []byte
	if decErr == nil {
		rawBytes = decoded
		var parsed interface{}
		if json.Unmarshal(decoded, &parsed) == nil {
			normalized := normalizeJSON(parsed)
			raw, err := json.Marshal(normalized)
			if err == nil {
				rawBytes = raw
			}
		}
	} else {
		// Fallback: hash the raw b64 string if it can't be decoded
		rawBytes = []byte(bodyB64)
	}

	h := sha256.Sum256(rawBytes)
	return hex.EncodeToString(h[:8]) // 16 hex chars
}

// normalizeJSON sorts object keys and strips values to produce a structural fingerprint.
func normalizeJSON(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		result := make(map[string]interface{})
		for _, k := range keys {
			result[k] = fmt.Sprintf("%T", val[k]) // replace value with type name
		}
		return result
	case []interface{}:
		if len(val) > 0 {
			return []interface{}{fmt.Sprintf("%T", val[0]), len(val)}
		}
		return val
	default:
		return fmt.Sprintf("%T", v)
	}
}

// stillTriggers replays a session and checks whether the anomaly detector fires.
func (t *Triager) stillTriggers(
	ctx context.Context,
	session model.RecordingSession,
	baseURL string,
	detector anomaly.Detector,
	anomalyCfg model.AnomalyConfig,
	baseline *anomaly.BaselineEntry,
	rep SessionReplayer,
	timeout time.Duration,
	logger zerolog.Logger,
) bool {
	wctx := replayer.NewWorkerContext(timeout, logger)
	results, err := rep.ReplaySession(ctx, session, baseURL, wctx, nil)
	if err != nil {
		return false
	}
	for _, result := range results {
		hits := detector.Detect(result.Exchange, result, baseline, anomalyCfg)
		if len(hits) > 0 {
			return true
		}
	}
	return false
}

// Confirm replays a finding N times to check if the anomaly is reproducible.
// Returns true if the anomaly reproduced in at least half the runs.
func (t *Triager) Confirm(
	ctx context.Context,
	session model.RecordingSession,
	baseURL string,
	detector anomaly.Detector,
	anomalyCfg model.AnomalyConfig,
	baseline *anomaly.BaselineEntry,
	rep SessionReplayer,
	runs int,
	timeout time.Duration,
	logger zerolog.Logger,
) (bool, error) {
	if runs <= 0 {
		runs = 3
	}

	reproduced := 0
	for i := 0; i < runs; i++ {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		if t.stillTriggers(ctx, session, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger) {
			reproduced++
		}
	}

	return reproduced >= (runs+1)/2, nil
}

// MinimizeSession attempts to reduce the session to the minimal set of exchanges
// that still triggers the anomaly. It tries removing exchanges one at a time.
func (t *Triager) MinimizeSession(
	ctx context.Context,
	session model.RecordingSession,
	baseURL string,
	detector anomaly.Detector,
	anomalyCfg model.AnomalyConfig,
	baseline *anomaly.BaselineEntry,
	rep SessionReplayer,
	timeout time.Duration,
	logger zerolog.Logger,
) (*model.RecordingSession, error) {
	entries := session.Entries
	if len(entries) <= 1 {
		return &session, nil
	}

	// Try removing each exchange (from the end, skip first)
	for i := len(entries) - 1; i >= 1; i-- {
		if ctx.Err() != nil {
			break
		}

		candidate := make([]model.Exchange, 0, len(entries)-1)
		candidate = append(candidate, entries[:i]...)
		candidate = append(candidate, entries[i+1:]...)

		testSession := session
		testSession.Entries = candidate

		if t.stillTriggers(ctx, testSession, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger) {
			entries = candidate
		}
	}

	result := session
	result.Entries = entries
	result.EntryCount = len(entries)
	return &result, nil
}

// HasJSONBody returns true if the exchange has a non-truncated JSON object body.
func HasJSONBody(ex model.Exchange) bool {
	if ex.Request.BodyB64 == "" || ex.Request.BodyTruncated {
		return false
	}
	ct := ""
	for k, vv := range ex.Request.Headers {
		if strings.EqualFold(k, "content-type") && len(vv) > 0 {
			ct = vv[0]
			break
		}
	}
	if ct != "" && !strings.Contains(strings.ToLower(ct), "json") {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
	if err != nil {
		return false
	}
	var obj map[string]interface{}
	return json.Unmarshal(raw, &obj) == nil
}

// cloneSessionWithBody returns a copy of session with the body at exchangeIdx replaced.
func cloneSessionWithBody(session model.RecordingSession, idx int, bodyB64 string) model.RecordingSession {
	cloned := session
	cloned.Entries = make([]model.Exchange, len(session.Entries))
	copy(cloned.Entries, session.Entries)
	cloned.Entries[idx].Request.BodyB64 = bodyB64
	return cloned
}

// MinimizeJSONBody tries to reduce a JSON request body by binary-search field removal
// while verifying the anomaly still fires after each reduction.
// Returns nil, nil if no reduction was possible.
func (t *Triager) MinimizeJSONBody(
	ctx context.Context,
	session model.RecordingSession,
	exchangeIdx int,
	baseURL string,
	detector anomaly.Detector,
	anomalyCfg model.AnomalyConfig,
	baseline *anomaly.BaselineEntry,
	rep SessionReplayer,
	timeout time.Duration,
	logger zerolog.Logger,
) (*model.RecordingSession, error) {
	if exchangeIdx < 0 || exchangeIdx >= len(session.Entries) {
		return nil, nil
	}

	ex := session.Entries[exchangeIdx]
	if ex.Request.BodyB64 == "" || ex.Request.BodyTruncated {
		return nil, nil
	}

	raw, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
	if err != nil {
		return nil, nil
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, nil
	}
	if len(obj) == 0 {
		return nil, nil
	}

	originalKeyCount := countKeysDeep(obj)

	verify := func(candidate map[string]interface{}) bool {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		data, err := json.Marshal(candidate)
		if err != nil {
			return false
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		testSession := cloneSessionWithBody(session, exchangeIdx, b64)
		return t.stillTriggers(ctx, testSession, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger)
	}

	reduced := t.deltaDebugKeys(ctx, obj, sortedKeys(obj), verify, 0)

	if countKeysDeep(reduced) >= originalKeyCount {
		return nil, nil
	}

	data, err := json.Marshal(reduced)
	if err != nil {
		return nil, nil
	}
	result := cloneSessionWithBody(session, exchangeIdx, base64.StdEncoding.EncodeToString(data))
	return &result, nil
}

// deltaDebugKeys performs binary-search field removal on a JSON object.
// It minimizes keys at the current level, then recurses into nested objects.
func (t *Triager) deltaDebugKeys(
	ctx context.Context,
	obj map[string]interface{},
	keys []string,
	verify func(map[string]interface{}) bool,
	depth int,
) map[string]interface{} {
	// Phase 1: remove unnecessary keys at this level
	reduced := t.removeKeys(ctx, obj, keys, verify)

	// Phase 2: recurse into nested objects if within depth limit
	if depth < maxNestedDepth {
		for _, k := range sortedKeys(reduced) {
			nested, ok := reduced[k].(map[string]interface{})
			if !ok || len(nested) == 0 {
				continue
			}
			nestedVerify := func(candidate map[string]interface{}) bool {
				trial := copyMap(reduced)
				trial[k] = candidate
				return verify(trial)
			}
			reduced[k] = t.deltaDebugKeys(ctx, nested, sortedKeys(nested), nestedVerify, depth+1)
		}
	}

	return reduced
}

// removeKeys performs the binary-search key removal without nested recursion.
func (t *Triager) removeKeys(
	ctx context.Context,
	obj map[string]interface{},
	keys []string,
	verify func(map[string]interface{}) bool,
) map[string]interface{} {
	if len(keys) == 0 {
		return obj
	}

	// Single key: try removing it
	if len(keys) == 1 {
		candidate := withoutKeys(obj, keys)
		if verify(candidate) {
			return candidate
		}
		return obj
	}

	mid := len(keys) / 2
	left := keys[:mid]
	right := keys[mid:]

	// Try keeping only right half (remove left)
	candidateRight := onlyKeys(obj, right)
	if verify(candidateRight) {
		return t.removeKeys(ctx, candidateRight, right, verify)
	}

	// Try keeping only left half (remove right)
	candidateLeft := onlyKeys(obj, left)
	if verify(candidateLeft) {
		return t.removeKeys(ctx, candidateLeft, left, verify)
	}

	// Neither half alone works — try fine-grained removal on each half
	select {
	case <-ctx.Done():
		return obj
	default:
	}

	reduced := t.removeKeys(ctx, obj, left, verify)
	reduced = t.removeKeys(ctx, reduced, right, verify)
	return reduced
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func withoutKeys(obj map[string]interface{}, remove []string) map[string]interface{} {
	removeSet := make(map[string]struct{}, len(remove))
	for _, k := range remove {
		removeSet[k] = struct{}{}
	}
	result := make(map[string]interface{}, len(obj)-len(remove))
	for k, v := range obj {
		if _, skip := removeSet[k]; !skip {
			result[k] = v
		}
	}
	return result
}

func onlyKeys(obj map[string]interface{}, keep []string) map[string]interface{} {
	keepSet := make(map[string]struct{}, len(keep))
	for _, k := range keep {
		keepSet[k] = struct{}{}
	}
	result := make(map[string]interface{}, len(keep))
	for _, k := range keep {
		if v, ok := obj[k]; ok {
			result[k] = v
		}
	}
	return result
}

func copyMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func countKeysDeep(m map[string]interface{}) int {
	count := len(m)
	for _, v := range m {
		if nested, ok := v.(map[string]interface{}); ok {
			count += countKeysDeep(nested)
		}
	}
	return count
}
