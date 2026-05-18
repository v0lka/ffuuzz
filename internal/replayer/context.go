package replayer

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

// ExtractionRule defines how to extract a variable from a response.
type ExtractionRule struct {
	Name   string // variable name
	Source string // "body" or "header"
	Header string // header name (if Source == "header")
	Regex  string // regex with a capture group
}

// WorkerContext holds per-worker isolated state for stateful replay.
type WorkerContext struct {
	CookieJar *cookiejar.Jar
	Variables map[string]string
	Client    *http.Client
	logger    zerolog.Logger
}

// NewWorkerContext creates an isolated worker context with its own cookie jar and HTTP client.
func NewWorkerContext(timeout time.Duration, logger zerolog.Logger) *WorkerContext {
	jar, err := cookiejar.New(nil)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to create cookie jar, continuing without cookie storage")
		jar = nil
	}
	return &WorkerContext{
		CookieJar: jar,
		Variables: make(map[string]string),
		Client: &http.Client{
			Jar:     jar,
			Timeout: timeout,
		},
		logger: logger,
	}
}

// ApplySubstitutions replaces {{var}} placeholders in exchange path, query, headers, and body.
func (wc *WorkerContext) ApplySubstitutions(ex *model.Exchange) {
	ex.Request.Path = wc.substituteString(ex.Request.Path)
	ex.Request.Query = wc.substituteString(ex.Request.Query)
	ex.Request.BodyB64 = wc.substituteString(ex.Request.BodyB64)

	if ex.Request.Headers != nil {
		for k, vals := range ex.Request.Headers {
			for i, v := range vals {
				vals[i] = wc.substituteString(v)
			}
			ex.Request.Headers[k] = vals
		}
	}
}

func (wc *WorkerContext) substituteString(s string) string {
	for name, val := range wc.Variables {
		s = strings.ReplaceAll(s, "{{"+name+"}}", val)
	}
	return s
}

// ExtractVariables extracts variables from a response according to rules.
func (wc *WorkerContext) ExtractVariables(resp *http.Response, body []byte, rules []ExtractionRule) {
	for _, rule := range rules {
		var source string
		switch rule.Source {
		case "header":
			source = resp.Header.Get(rule.Header)
		case "body":
			source = string(body)
		default:
			continue
		}

		if rule.Regex == "" {
			continue
		}

		re, err := regexp.Compile(rule.Regex)
		if err != nil {
			wc.logger.Warn().Err(err).Str("rule", rule.Name).Str("regex", rule.Regex).Msg("invalid extraction rule regex")
			continue
		}

		matches := re.FindStringSubmatch(source)
		if len(matches) >= 2 {
			wc.Variables[rule.Name] = matches[1]
		}
	}
}

// UpdateCookies delegates cookie management to the jar for a given URL.
func (wc *WorkerContext) UpdateCookies(resp *http.Response, reqURL *url.URL) {
	if wc.CookieJar != nil && resp != nil {
		cookies := resp.Cookies()
		wc.CookieJar.SetCookies(reqURL, cookies)
	}
}
