package mutate

import (
	"encoding/json"
	"os"
	"strings"
	"sync"

	"ffuuzz/internal/endpoint"
	"ffuuzz/internal/model"
)

// Dictionary holds fuzzing dictionary entries with global and per-endpoint values.
// It is safe for concurrent use from multiple worker goroutines.
type Dictionary struct {
	mu     sync.RWMutex
	global map[string][]string            // header name → values
	perEP  map[string]map[string][]string // endpoint pattern → header name → values
}

// dictFileFormat is the on-disk format for a dictionary JSON file.
type dictFileFormat struct {
	Global    map[string][]string            `json:"global"`
	Endpoints map[string]map[string][]string `json:"endpoints"`
}

// NewDictionary creates an empty Dictionary.
func NewDictionary() *Dictionary {
	return &Dictionary{
		global: make(map[string][]string),
		perEP:  make(map[string]map[string][]string),
	}
}

// NewDictionaryFromFile reads a dictionary from a JSON file.
func NewDictionaryFromFile(path string) (*Dictionary, error) {
	d := NewDictionary()
	if err := d.LoadFromFile(path); err != nil {
		return nil, err
	}
	return d, nil
}

// LoadFromFiles reads and merges dictionary entries from multiple JSON files.
// Files that cannot be read are silently skipped.
func LoadFromFiles(paths []string) *Dictionary {
	d := NewDictionary()
	for _, p := range paths {
		if err := d.LoadFromFile(p); err != nil {
			continue
		}
	}
	return d
}

// AddGlobal adds header-value mappings to the global dictionary.
// Header names are normalized to lowercase.
func (d *Dictionary) AddGlobal(header string, values []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	hl := strings.ToLower(header)
	d.global[hl] = append(d.global[hl], values...)
}

// AddForEndpoint adds header-value mappings scoped to an endpoint pattern.
// Header names are normalized to lowercase.
func (d *Dictionary) AddForEndpoint(endpoint, header string, values []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	hl := strings.ToLower(header)
	if d.perEP[endpoint] == nil {
		d.perEP[endpoint] = make(map[string][]string)
	}
	d.perEP[endpoint][hl] = append(d.perEP[endpoint][hl], values...)
}

// ValuesForHeader returns merged global + per-endpoint values for a header.
// Header names are matched case-insensitively.
func (d *Dictionary) ValuesForHeader(endpoint, header string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	hl := strings.ToLower(header)
	var result []string
	result = append(result, d.global[hl]...)
	if ep, ok := d.perEP[endpoint]; ok {
		result = append(result, ep[hl]...)
	}
	return result
}

// AllHeaders returns the set of header names (lowercased) known by the dictionary,
// merging global and per-endpoint entries.
func (d *Dictionary) AllHeaders(endpoint string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	seen := make(map[string]bool)
	for h := range d.global {
		seen[h] = true
	}
	if ep, ok := d.perEP[endpoint]; ok {
		for h := range ep {
			seen[h] = true
		}
	}
	headers := make([]string, 0, len(seen))
	for h := range seen {
		headers = append(headers, h)
	}
	return headers
}

// LoadFromFile reads a JSON dictionary file and merges it into the dictionary.
func (d *Dictionary) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var f dictFileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for h, vals := range f.Global {
		hl := strings.ToLower(h)
		d.global[hl] = append(d.global[hl], vals...)
	}
	for ep, headers := range f.Endpoints {
		if d.perEP[ep] == nil {
			d.perEP[ep] = make(map[string][]string)
		}
		for h, vals := range headers {
			hl := strings.ToLower(h)
			d.perEP[ep][hl] = append(d.perEP[ep][hl], vals...)
		}
	}
	return nil
}

// commonCTypes is a set of Content-Type values too common to be useful as dictionary entries.
var commonCTypes = map[string]bool{
	"text/html":                         true,
	"application/json":                  true,
	"text/plain":                        true,
	"application/xml":                   true,
	"text/xml":                          true,
	"application/x-www-form-urlencoded": true,
	"multipart/form-data":               true,
}

// commonUserAgents are User-Agent prefixes too common to extract.
var commonUserAgents = []string{
	"Mozilla/5.0",
	"curl/",
	"python-requests",
	"Go-http-client",
}

// isCommonValue returns true when a header value is too generic to be useful.
func isCommonValue(header, value string) bool {
	hl := strings.ToLower(header)
	if hl == "content-type" {
		return commonCTypes[strings.SplitN(value, ";", 2)[0]]
	}
	if hl == "user-agent" {
		for _, prefix := range commonUserAgents {
			if strings.HasPrefix(value, prefix) {
				return true
			}
		}
	}
	return false
}

// ExtractFromTraffic populates the dictionary from recorded traffic.
// It collects unique header values across all exchanges, skipping common values
// (standard content-types, user-agents) and internal HTTP headers.
func (d *Dictionary) ExtractFromTraffic(sessions []model.RecordingSession) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Internal HTTP headers to exclude from extraction
	skipHeaders := map[string]bool{
		"host":              true,
		"content-length":    true,
		"transfer-encoding": true,
		"connection":        true,
		"accept-encoding":   true,
	}

	for _, sess := range sessions {
		for _, ex := range sess.Entries {
			epp := endpoint.NormalizePath(ex.Request.Path)
			for h, vals := range ex.Request.Headers {
				hl := strings.ToLower(h)
				if skipHeaders[hl] {
					continue
				}
				for _, v := range vals {
					if v == "" || isCommonValue(hl, v) {
						continue
					}
					if d.perEP[epp] == nil {
						d.perEP[epp] = make(map[string][]string)
					}
					// Only add if not already present for this endpoint+header
					found := false
					for _, existing := range d.perEP[epp][hl] {
						if existing == v {
							found = true
							break
						}
					}
					if !found {
						d.perEP[epp][hl] = append(d.perEP[epp][hl], v)
					}
				}
			}
		}
	}
}
