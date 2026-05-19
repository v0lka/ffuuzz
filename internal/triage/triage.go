// Package triage confirms and minimises findings by replaying them against the target.
package triage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
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

// NewTriager creates a new Triager.
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
// Returns whether the anomaly was confirmed, the number of successful reproductions, and any error.
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
) (bool, int, error) {
	if runs <= 0 {
		runs = 3
	}

	reproduced := 0
	for i := 0; i < runs; i++ {
		select {
		case <-ctx.Done():
			return false, reproduced, ctx.Err()
		default:
		}

		if t.stillTriggers(ctx, session, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger) {
			reproduced++
		}
	}

	return reproduced >= (runs+1)/2, reproduced, nil
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
	var obj map[string]any
	return json.Unmarshal(raw, &obj) == nil
}

// cloneSessionWithBody returns a copy of session with the body at exchangeIdx replaced.
// Note: shallow-copies the session struct, relying on an explicit deep-copy of Entries.
// If RecordingSession gains pointer/slice fields in the future, they must be deep-copied here.
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

	var obj map[string]any
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

// ScoreSeverity assigns a severity level (CRITICAL/HIGH/MEDIUM/LOW/INFO) based on
// endpoint sensitivity, finding type, mutation type, and reproducibility rate.
func (t *Triager) ScoreSeverity(
	findingType model.FindingType,
	endpoint, method, mutationType string,
	reproducibility float64,
	responseStatus int,
) model.Severity {
	endpointWeight := 0.4 // default
	switch {
	case strings.HasPrefix(endpoint, "/auth/"):
		endpointWeight = 1.0
	case strings.HasPrefix(endpoint, "/admin/"):
		endpointWeight = 0.8
	case strings.HasPrefix(endpoint, "/api/users"):
		endpointWeight = 0.7
	case strings.HasPrefix(endpoint, "/api/"):
		endpointWeight = 0.5
	case strings.HasPrefix(endpoint, "/health"):
		endpointWeight = 0.2
	}

	var typeWeight float64
	switch findingType {
	case model.FindingServerError:
		typeWeight = 0.8
	case model.FindingRegexMatch:
		typeWeight = 0.6
	case model.FindingTimeout:
		typeWeight = 0.5
	case model.FindingLatencyRegression:
		typeWeight = 0.3
	default:
		typeWeight = 0.4
	}

	mutationWeight := 0.4 // default
	mtl := strings.ToLower(mutationType)
	if strings.Contains(mtl, "header") {
		mutationWeight = 0.6
	} else if strings.Contains(mtl, "uri") || strings.Contains(mtl, "param") || strings.Contains(mtl, "query") {
		mutationWeight = 0.5
	} else if strings.Contains(mtl, "seq") {
		mutationWeight = 0.3
	}
	for _, kw := range []string{"sqli", "cmdi", "xxe", "injection", "os_command", "template", "ldap", "xpath", "ssrf"} {
		if strings.Contains(mtl, kw) {
			mutationWeight = 1.0
			break
		}
	}

	var reproMult float64
	switch {
	case reproducibility <= 0:
		reproMult = 1.0
	case reproducibility > 0.8:
		reproMult = 1.0
	case reproducibility >= 0.5:
		reproMult = 0.75
	default:
		reproMult = 0.5
	}

	score := (endpointWeight + typeWeight + mutationWeight) / 3 * reproMult

	switch {
	case score >= 0.8:
		return model.SeverityCritical
	case score >= 0.6:
		return model.SeverityHigh
	case score >= 0.4:
		return model.SeverityMedium
	case score >= 0.2:
		return model.SeverityLow
	default:
		return model.SeverityInfo
	}
}

// stackTracePatterns matches common error disclosure patterns in response bodies.
var stackTracePatterns = []*regexp.Regexp{
	regexp.MustCompile(`at com\.\w+\.`),
	regexp.MustCompile(`NullPointerException`),
	regexp.MustCompile(`Traceback \(most recent call last\)`),
	regexp.MustCompile(`PHP Fatal error`),
	regexp.MustCompile(`SQLSTATE`),
	regexp.MustCompile(`PostgreSQL query failed`),
	regexp.MustCompile(`Microsoft OLE DB`),
}

// CategorizeFinding maps a finding to an OWASP Top 10 2025 category based on
// mutation type, response body content, and finding type.
func (t *Triager) CategorizeFinding(
	findingType model.FindingType,
	mutationType string,
	responseBody []byte,
	httpStatus int,
) model.OWASPCategory {
	mtl := strings.ToLower(mutationType)

	// A05: Injection — sqli, cmdi, xxe, ssrf, template/ldap/xpath injection
	for _, kw := range []string{"sqli", "cmdi", "xxe", "ssrf", "template_injection", "ldap_injection", "xpath_injection"} {
		if strings.Contains(mtl, kw) {
			return model.OWASPCatA05Injection
		}
	}

	// A04: Cryptographic Failures — jwt mutation causing 500
	if strings.Contains(mtl, "jwt") && httpStatus >= 500 {
		return model.OWASPCatA04CryptographicFailures
	}

	// A07: Authentication Failures — jwt or cookie mutations (non-500)
	if strings.Contains(mtl, "jwt") || strings.Contains(mtl, "cookie") {
		return model.OWASPCatA07AuthenticationFailures
	}

	// A02: Security Misconfiguration — cors/origin header mutations
	if strings.Contains(mtl, "cors") || strings.Contains(mtl, "origin") {
		return model.OWASPCatA02SecurityMisconfiguration
	}

	// A10: Exceptional Conditions — stack traces / error disclosure in body
	bodyStr := string(responseBody)
	for _, re := range stackTracePatterns {
		if re.MatchString(bodyStr) {
			return model.OWASPCatA10ExceptionalConditions
		}
	}

	// A06: Insecure Design — SERVER_ERROR with no specific match
	if findingType == model.FindingServerError {
		return model.OWASPCatA06InsecureDesign
	}

	return model.OWASPCatUncategorized
}

// GroupFindings groups findings by (Type, MutationPrefix, EndpointPattern, HTTPStatusRange).
// Returns a map from group key to the slice of findings that belong to that group.
func (t *Triager) GroupFindings(findings []model.Finding) map[string][]model.Finding {
	groups := make(map[string][]model.Finding)
	for _, f := range findings {
		key := groupKey(f)
		groups[key] = append(groups[key], f)
	}
	return groups
}

// groupKey builds a grouping key for a finding.
func groupKey(f model.Finding) string {
	// Mutation prefix: first colon-delimited segment, or "unknown"
	mutPrefix := "unknown"
	if f.MutationType != "" {
		if idx := strings.IndexByte(f.MutationType, ':'); idx >= 0 {
			mutPrefix = f.MutationType[:idx]
		} else {
			mutPrefix = f.MutationType
		}
	}

	// Endpoint pattern: first path segment, or "root"
	endpointPattern := "root"
	if f.Endpoint != "" && f.Endpoint != "/" {
		segments := strings.Split(strings.TrimPrefix(f.Endpoint, "/"), "/")
		if len(segments) > 0 && segments[0] != "" {
			endpointPattern = segments[0]
		}
	}

	// HTTP status range
	statusRange := "0xx"
	status := f.Details.HTTPStatus
	switch {
	case status >= 500:
		statusRange = "5xx"
	case status >= 400:
		statusRange = "4xx"
	case status >= 300:
		statusRange = "3xx"
	case status >= 200:
		statusRange = "2xx"
	}

	return string(f.Type) + "|" + mutPrefix + "|" + endpointPattern + "|" + statusRange
}

// GetContentType extracts the Content-Type header value (lowercased, without parameters)
// from a request, or returns an empty string if not present.
func GetContentType(req model.RequestData) string {
	for k, vv := range req.Headers {
		if strings.EqualFold(k, "content-type") && len(vv) > 0 {
			ct := strings.ToLower(vv[0])
			if idx := strings.IndexByte(ct, ';'); idx >= 0 {
				ct = strings.TrimSpace(ct[:idx])
			}
			return ct
		}
	}
	return ""
}

// MinimizeQueryParams attempts to reduce query parameters by binary-search removal
// while verifying the anomaly still fires. Returns nil, nil if no reduction was possible.
func (t *Triager) MinimizeQueryParams(
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
	if ex.Request.Query == "" {
		return nil, nil
	}

	vals, err := url.ParseQuery(ex.Request.Query)
	if err != nil || len(vals) == 0 {
		return nil, nil
	}

	params := make(map[string]interface{}, len(vals))
	for k := range vals {
		params[k] = vals.Get(k)
	}

	verify := func(candidate map[string]interface{}) bool {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		newVals := make(url.Values, len(candidate))
		for k, v := range candidate {
			s, ok := v.(string)
			if !ok {
				s = fmt.Sprintf("%v", v)
			}
			newVals.Set(k, s)
		}
		cloned := cloneSessionWithBody(session, exchangeIdx, ex.Request.BodyB64)
		cloned.Entries[exchangeIdx].Request.Query = newVals.Encode()
		return t.stillTriggers(ctx, cloned, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger)
	}

	reduced := t.deltaDebugKeys(ctx, params, sortedKeys(params), verify, 0)

	if len(reduced) >= len(params) {
		return nil, nil
	}

	newVals := make(url.Values, len(reduced))
	for k, v := range reduced {
		s, ok := v.(string)
		if !ok {
			s = fmt.Sprintf("%v", v)
		}
		newVals.Set(k, s)
	}

	result := cloneSessionWithBody(session, exchangeIdx, ex.Request.BodyB64)
	result.Entries[exchangeIdx].Request.Query = newVals.Encode()
	return &result, nil
}

// MinimizeXMLBody attempts to reduce an XML request body by binary-search leaf-element removal
// while verifying the anomaly still fires. Returns nil, nil if no reduction was possible.
func (t *Triager) MinimizeXMLBody(
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

	ct := GetContentType(ex.Request)
	if ct != "" && !strings.Contains(ct, "xml") {
		return nil, nil
	}

	raw, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
	if err != nil || len(raw) == 0 {
		return nil, nil
	}

	xmlMap, err := parseXMLToMap(raw)
	if err != nil || len(xmlMap) == 0 {
		return nil, nil
	}

	verify := func(candidate map[string]interface{}) bool {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		data, err := mapToXMLBytes(candidate)
		if err != nil || data == nil {
			return false
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		testSession := cloneSessionWithBody(session, exchangeIdx, b64)
		return t.stillTriggers(ctx, testSession, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger)
	}

	reduced := t.deltaDebugKeys(ctx, xmlMap, sortedKeys(xmlMap), verify, 0)

	if len(reduced) >= len(xmlMap) {
		return nil, nil
	}

	data, err := mapToXMLBytes(reduced)
	if err != nil || data == nil {
		return nil, nil
	}
	result := cloneSessionWithBody(session, exchangeIdx, base64.StdEncoding.EncodeToString(data))
	return &result, nil
}

// parseXMLToMap uses XML tokenization to flatten an XML document into a path→value map.
func parseXMLToMap(data []byte) (map[string]interface{}, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	result := make(map[string]interface{})
	var path []string

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			path = append(path, t.Name.Local)
		case xml.EndElement:
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text != "" && len(path) > 0 {
				key := strings.Join(path, ".")
				result[key] = text
			}
		}
	}
	return result, nil
}

// mapToXMLBytes rebuilds XML from a flat path→value map.
func mapToXMLBytes(m map[string]interface{}) ([]byte, error) {
	if len(m) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)

	for _, k := range keys {
		v, ok := m[k].(string)
		if !ok {
			v = fmt.Sprintf("%v", m[k])
		}
		parts := strings.Split(k, ".")
		for _, part := range parts {
			buf.WriteString("<" + part + ">")
		}
		buf.WriteString(xmlEscapeText(v))
		for i := len(parts) - 1; i >= 0; i-- {
			buf.WriteString("</" + parts[i] + ">")
		}
	}

	return buf.Bytes(), nil
}

// xmlEscapeText escapes special XML characters in text content.
func xmlEscapeText(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}

// multipartPart represents a single part in a multipart form body.
type multipartPart struct {
	name string
	body []byte
}

// MinimizeMultipartBody attempts to reduce a multipart form-data body by removing parts
// one at a time while verifying the anomaly still fires. Returns nil, nil if no reduction was possible.
func (t *Triager) MinimizeMultipartBody(
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

	ct := GetContentType(ex.Request)
	if !strings.Contains(ct, "multipart/form-data") {
		return nil, nil
	}

	raw, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
	if err != nil || len(raw) == 0 {
		return nil, nil
	}

	_, params, err := mime.ParseMediaType(ex.Request.Headers["Content-Type"][0])
	if err != nil {
		return nil, nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, nil
	}

	var parts []multipartPart
	mr := multipart.NewReader(bytes.NewReader(raw), boundary)
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil
		}
		body, _ := io.ReadAll(p)
		parts = append(parts, multipartPart{
			name: p.FormName(),
			body: body,
		})
	}

	if len(parts) <= 1 {
		return nil, nil
	}

	changed := false
	for i := len(parts) - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		candidateParts := make([]multipartPart, 0, len(parts)-1)
		for j, p := range parts {
			if j != i {
				candidateParts = append(candidateParts, p)
			}
		}

		newBody := buildMultipartBody(candidateParts, boundary)
		b64 := base64.StdEncoding.EncodeToString(newBody)
		testSession := cloneSessionWithBody(session, exchangeIdx, b64)

		if t.stillTriggers(ctx, testSession, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger) {
			parts = candidateParts
			changed = true
		}
	}

	if !changed {
		return nil, nil
	}

	newBody := buildMultipartBody(parts, boundary)
	result := cloneSessionWithBody(session, exchangeIdx, base64.StdEncoding.EncodeToString(newBody))
	return &result, nil
}

// buildMultipartBody constructs a multipart/form-data body from parts.
func buildMultipartBody(parts []multipartPart, boundary string) []byte {
	var buf bytes.Buffer
	for _, p := range parts {
		buf.WriteString("--" + boundary + "\r\n")
		buf.WriteString("Content-Disposition: form-data; name=\"" + p.name + "\"\r\n")
		buf.WriteString("\r\n")
		buf.Write(p.body)
		buf.WriteString("\r\n")
	}
	buf.WriteString("--" + boundary + "--\r\n")
	return buf.Bytes()
}
