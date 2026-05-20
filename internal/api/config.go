// Package api implements the Control API server.
package api

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"ffuuzz/internal/config"
)

// --- API types (separate from config.Config to handle serialization differences) ---

type tlsConfigAPI struct {
	MinVersion            string `json:"min_version"`
	HandshakeTimeout      string `json:"handshake_timeout"`
	DisableSessionTickets bool   `json:"disable_session_tickets"`
}

type certCacheConfigAPI struct {
	MaxEntries int    `json:"max_entries"`
	MemoryOnly bool   `json:"memory_only"`
	CertDir    string `json:"cert_dir"`
}

type llmConfigAPI struct {
	Enabled   bool   `json:"enabled"`
	Provider  string `json:"provider"`
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	Timeout   string `json:"timeout"`
}

type configResponse struct {
	APIAddress      string           `json:"api_address"`
	ProxyAddress    string           `json:"proxy_address"`
	DatabaseURI     string           `json:"database_uri"`
	ArtifactDir     string           `json:"artifact_dir"`
	ReqTimeout      string           `json:"req_timeout"`
	ShutdownTimeout string           `json:"shutdown_timeout"`
	Workers         int              `json:"workers"`
	RPS             int              `json:"rps"`
	MaxBodyBytes    int              `json:"max_body_bytes"`
	TLSSkipVerify   bool             `json:"tls_skip_verify"`
	TLS             tlsConfigAPI     `json:"tls"`
	CertCache       certCacheConfigAPI `json:"cert_cache"`
	LLM             llmConfigAPI     `json:"llm"`
}

type configUpdateRequest struct {
	APIAddress      *string             `json:"api_address,omitempty"`
	ProxyAddress    *string             `json:"proxy_address,omitempty"`
	DatabaseURI     *string             `json:"database_uri,omitempty"`
	ArtifactDir     *string             `json:"artifact_dir,omitempty"`
	ReqTimeout      *string             `json:"req_timeout,omitempty"`
	ShutdownTimeout *string             `json:"shutdown_timeout,omitempty"`
	Workers         *int                `json:"workers,omitempty"`
	RPS             *int                `json:"rps,omitempty"`
	MaxBodyBytes    *int                `json:"max_body_bytes,omitempty"`
	TLSSkipVerify   *bool               `json:"tls_skip_verify,omitempty"`
	TLS             *tlsConfigAPI       `json:"tls,omitempty"`
	CertCache       *certCacheConfigAPI `json:"cert_cache,omitempty"`
	LLM             *llmConfigAPI       `json:"llm,omitempty"`
}

type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// maskedAPIKey is the sentinel value used to indicate a pre-existing key.
const maskedAPIKey = "••••••••"

// fieldNameToEnvKey maps JSON field paths to FFUUZZ_* environment variable names.
// Flat keys use the field name directly; nested keys use dot-separated paths.
var fieldNameToEnvKey = map[string]string{
	"api_address":                         "FFUUZZ_API_ADDRESS",
	"proxy_address":                       "FFUUZZ_PROXY_ADDRESS",
	"database_uri":                        "FFUUZZ_DATABASE_URI",
	"artifact_dir":                        "FFUUZZ_ARTIFACT_DIR",
	"req_timeout":                         "FFUUZZ_REQ_TIMEOUT",
	"shutdown_timeout":                    "FFUUZZ_SHUTDOWN_TIMEOUT",
	"workers":                             "FFUUZZ_WORKERS",
	"rps":                                 "FFUUZZ_RPS",
	"max_body_bytes":                      "FFUUZZ_MAX_BODY_BYTES",
	"tls_skip_verify":                     "FFUUZZ_TLS_SKIP_VERIFY",
	"tls.min_version":                     "FFUUZZ_TLS_MIN_VERSION",
	"tls.handshake_timeout":               "FFUUZZ_TLS_HANDSHAKE_TIMEOUT",
	"tls.disable_session_tickets":         "FFUUZZ_TLS_DISABLE_SESSION_TICKETS",
	"cert_cache.max_entries":              "FFUUZZ_CERT_CACHE_MAX_ENTRIES",
	"cert_cache.memory_only":              "FFUUZZ_CERT_MEMORY_ONLY",
	"cert_cache.cert_dir":                 "FFUUZZ_CERT_CACHE_DIR",
	"llm.enabled":                         "FFUUZZ_LLM_ENABLED",
	"llm.provider":                        "FFUUZZ_LLM_PROVIDER",
	"llm.api_key":                         "FFUUZZ_LLM_API_KEY",
	"llm.base_url":                        "FFUUZZ_LLM_BASE_URL",
	"llm.model":                           "FFUUZZ_LLM_MODEL",
	"llm.max_tokens":                      "FFUUZZ_LLM_MAX_TOKENS",
	"llm.timeout":                         "FFUUZZ_LLM_TIMEOUT",
}

// envVarLineRegex matches an .env line that assigns to a FFUUZZ_* variable.
// Captures: group 1 = optional '#', group 2 = key, group 3 = value.
var envVarLineRegex = regexp.MustCompile(`^(\s*#+\s*)?(\w+)\s*=\s*(.*)$`)

// defaultLookup maps field paths to their default string representations.
var defaultLookup = initDefaultLookup()

func initDefaultLookup() map[string]string {
	def := config.DefaultConfig()
	return map[string]string{
		"api_address":                         def.APIAddress,
		"proxy_address":                       def.ProxyAddress,
		"database_uri":                        def.DatabaseURI,
		"artifact_dir":                        def.ArtifactDir,
		"req_timeout":                         def.ReqTimeout.String(),
		"shutdown_timeout":                    def.ShutdownTimeout.String(),
		"workers":                             strconv.Itoa(def.Workers),
		"rps":                                 strconv.Itoa(def.RPS),
		"max_body_bytes":                      strconv.Itoa(def.MaxBodyBytes),
		"tls_skip_verify":                     strconv.FormatBool(def.TLSSkipVerify),
		"tls.min_version":                     "1.2",
		"tls.handshake_timeout":               def.TLS.HandshakeTimeout.String(),
		"tls.disable_session_tickets":         strconv.FormatBool(def.TLS.DisableSessionTickets),
		"cert_cache.max_entries":              strconv.Itoa(def.CertCache.MaxEntries),
		"cert_cache.memory_only":              strconv.FormatBool(def.CertCache.MemoryOnly),
		"cert_cache.cert_dir":                 def.CertCache.CertDir,
		"llm.enabled":                         strconv.FormatBool(def.LLM.Enabled),
		"llm.provider":                        "",
		"llm.api_key":                         "",
		"llm.base_url":                        "",
		"llm.model":                           "",
		"llm.max_tokens":                      strconv.Itoa(def.LLM.MaxTokens),
		"llm.timeout":                         def.LLM.Timeout.String(),
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/config
// ---------------------------------------------------------------------------

func (s *Server) getConfig(c *gin.Context) {
	// Read .env file. If missing, proceed with defaults.
	envMap := make(map[string]string)
	data, err := os.ReadFile(s.envPath)
	if err == nil {
		expanded := os.Expand(string(data), func(name string) string {
			if v, ok := os.LookupEnv(name); ok {
				return v
			}
			return "${" + name + "}"
		})
		envMap, _ = godotenv.Parse(strings.NewReader(expanded))
	} else if !os.IsNotExist(err) {
		s.internalError(c, "CONFIG_READ_FAILED", err)
		return
	}

	resp := buildConfigResponse(envMap)
	c.JSON(http.StatusOK, resp)
}

func buildConfigResponse(envMap map[string]string) configResponse {
	def := config.DefaultConfig()

	getStr := func(key, defaultVal string) string {
		if v, ok := envMap[key]; ok {
			return v
		}
		return defaultVal
	}
	getBool := func(key string, defaultVal bool) bool {
		if v, ok := envMap[key]; ok {
			if b, err := strconv.ParseBool(v); err == nil {
				return b
			}
		}
		return defaultVal
	}
	getInt := func(key string, defaultVal int) int {
		if v, ok := envMap[key]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
		return defaultVal
	}
	getDuration := func(key string, defaultVal time.Duration) string {
		if v, ok := envMap[key]; ok {
			if _, err := time.ParseDuration(v); err == nil {
				return v
			}
		}
		return defaultVal.String()
	}

	return configResponse{
		APIAddress:      getStr("FFUUZZ_API_ADDRESS", def.APIAddress),
		ProxyAddress:    getStr("FFUUZZ_PROXY_ADDRESS", def.ProxyAddress),
		DatabaseURI:     getStr("FFUUZZ_DATABASE_URI", def.DatabaseURI),
		ArtifactDir:     getStr("FFUUZZ_ARTIFACT_DIR", def.ArtifactDir),
		ReqTimeout:      getDuration("FFUUZZ_REQ_TIMEOUT", def.ReqTimeout),
		ShutdownTimeout: getDuration("FFUUZZ_SHUTDOWN_TIMEOUT", def.ShutdownTimeout),
		Workers:         getInt("FFUUZZ_WORKERS", def.Workers),
		RPS:             getInt("FFUUZZ_RPS", def.RPS),
		MaxBodyBytes:    getInt("FFUUZZ_MAX_BODY_BYTES", def.MaxBodyBytes),
		TLSSkipVerify:   getBool("FFUUZZ_TLS_SKIP_VERIFY", def.TLSSkipVerify),
		TLS: tlsConfigAPI{
			MinVersion:            tlsVersionToString(getStr("FFUUZZ_TLS_MIN_VERSION", "1.2")),
			HandshakeTimeout:      getDuration("FFUUZZ_TLS_HANDSHAKE_TIMEOUT", def.TLS.HandshakeTimeout),
			DisableSessionTickets: getBool("FFUUZZ_TLS_DISABLE_SESSION_TICKETS", def.TLS.DisableSessionTickets),
		},
		CertCache: certCacheConfigAPI{
			MaxEntries: getInt("FFUUZZ_CERT_CACHE_MAX_ENTRIES", def.CertCache.MaxEntries),
			MemoryOnly: getBool("FFUUZZ_CERT_MEMORY_ONLY", def.CertCache.MemoryOnly),
			CertDir:    getStr("FFUUZZ_CERT_CACHE_DIR", def.CertCache.CertDir),
		},
		LLM: llmConfigAPI{
			Enabled:   getBool("FFUUZZ_LLM_ENABLED", def.LLM.Enabled),
			Provider:  getStr("FFUUZZ_LLM_PROVIDER", ""),
			APIKey:    maskAPIKey(getStr("FFUUZZ_LLM_API_KEY", "")),
			BaseURL:   getStr("FFUUZZ_LLM_BASE_URL", ""),
			Model:     getStr("FFUUZZ_LLM_MODEL", ""),
			MaxTokens: getInt("FFUUZZ_LLM_MAX_TOKENS", def.LLM.MaxTokens),
			Timeout:   getDuration("FFUUZZ_LLM_TIMEOUT", def.LLM.Timeout),
		},
	}
}

// tlsVersionToString normalizes a TLS version string to "1.2" or "1.3".
// The .env file may contain "1.2" or "1.3"; this ensures consistency.
func tlsVersionToString(v string) string {
	switch v {
	case "1.3":
		return "1.3"
	default:
		return "1.2"
	}
}

// maskAPIKey returns the masked sentinel if the key is non-empty.
func maskAPIKey(key string) string {
	if key != "" {
		return maskedAPIKey
	}
	return ""
}

// ---------------------------------------------------------------------------
// PUT /api/v1/config
// ---------------------------------------------------------------------------

func (s *Server) updateConfig(c *gin.Context) {
	var req configUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "INVALID_JSON", "failed to parse request body")
		return
	}

	// Validate all provided fields.
	if errs := validateConfigUpdate(req); len(errs) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "VALIDATION_FAILED",
			"message":    "Some configuration values are invalid",
			"request_id": c.GetString("request_id"),
			"fields":     errs,
		})
		return
	}

	if err := updateEnvFile(s.envPath, req); err != nil {
		s.internalError(c, "CONFIG_WRITE_FAILED", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Configuration saved. Changes take effect on next restart.",
	})
}

// validateConfigUpdate checks all provided fields and returns any errors.
func validateConfigUpdate(req configUpdateRequest) []fieldError {
	var errs []fieldError

	// Top-level duration fields
	if req.ReqTimeout != nil {
		if _, err := time.ParseDuration(*req.ReqTimeout); err != nil {
			errs = append(errs, fieldError{Field: "req_timeout", Message: "must be a valid Go duration (e.g. 3s, 500ms, 1m)"})
		}
	}
	if req.ShutdownTimeout != nil {
		if _, err := time.ParseDuration(*req.ShutdownTimeout); err != nil {
			errs = append(errs, fieldError{Field: "shutdown_timeout", Message: "must be a valid Go duration (e.g. 30s, 1m)"})
		}
	}

	// Top-level positive int fields
	if req.Workers != nil && *req.Workers <= 0 {
		errs = append(errs, fieldError{Field: "workers", Message: "must be a positive integer"})
	}
	if req.RPS != nil && *req.RPS <= 0 {
		errs = append(errs, fieldError{Field: "rps", Message: "must be a positive integer"})
	}
	if req.MaxBodyBytes != nil && *req.MaxBodyBytes <= 0 {
		errs = append(errs, fieldError{Field: "max_body_bytes", Message: "must be a positive integer"})
	}

	// TLS nested fields
	if req.TLS != nil {
		if req.TLS.MinVersion != "" && req.TLS.MinVersion != "1.2" && req.TLS.MinVersion != "1.3" {
			errs = append(errs, fieldError{Field: "tls.min_version", Message: "must be \"1.2\" or \"1.3\""})
		}
		if req.TLS.HandshakeTimeout != "" {
			if _, err := time.ParseDuration(req.TLS.HandshakeTimeout); err != nil {
				errs = append(errs, fieldError{Field: "tls.handshake_timeout", Message: "must be a valid Go duration (e.g. 10s, 30s)"})
			}
		}
	}

	// Cert cache nested fields
	if req.CertCache != nil {
		if req.CertCache.MaxEntries < 0 {
			errs = append(errs, fieldError{Field: "cert_cache.max_entries", Message: "must be a positive integer"})
		}
	}

	// LLM nested fields
	if req.LLM != nil {
		if req.LLM.Provider != "" && req.LLM.Provider != "anthropic" && req.LLM.Provider != "openai" {
			errs = append(errs, fieldError{Field: "llm.provider", Message: "must be \"anthropic\" or \"openai\""})
		}
		if req.LLM.MaxTokens < 0 {
			errs = append(errs, fieldError{Field: "llm.max_tokens", Message: "must be a positive integer"})
		}
		if req.LLM.Timeout != "" {
			if _, err := time.ParseDuration(req.LLM.Timeout); err != nil {
				errs = append(errs, fieldError{Field: "llm.timeout", Message: "must be a valid Go duration (e.g. 30s, 1m)"})
			}
		}
	}

	return errs
}

// updateEnvFile reads the existing .env file, applies the requested updates
// line-by-line (preserving comments and structure), and writes it back.
func updateEnvFile(path string, req configUpdateRequest) error {
	// Read existing file (empty if missing).
	var lines []string
	data, err := os.ReadFile(path)
	if err == nil {
		lines = strings.Split(string(data), "\n")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	// Build a map of env keys to new string values (only for fields present in request).
	updates := collectUpdates(req)
	if len(updates) == 0 {
		return nil
	}

	// Track which keys were matched in existing lines.
	matched := make(map[string]bool)

	// Process existing lines.
	var output []string
	for _, line := range lines {
		updated := false
		for envKey, newVal := range updates {
			if matched[envKey] {
				continue // already handled this key
			}
			if newLine, ok := updateEnvLine(line, envKey, newVal); ok {
				if strings.TrimSpace(newLine) != "" {
					output = append(output, newLine)
				}
				matched[envKey] = true
				updated = true
				break
			}
		}
		if !updated {
			output = append(output, line)
		}
	}

	// Append keys that weren't found in the file.
	for envKey, newVal := range updates {
		if !matched[envKey] {
			defaultVal := defaultLookup[reverseFieldLookup(envKey)]
			if newVal == defaultVal {
				output = append(output, fmt.Sprintf("#%s=%s", envKey, defaultVal))
			} else {
				output = append(output, fmt.Sprintf("%s=%s", envKey, newVal))
			}
		}
	}

	result := joinLines(output, data)
	return os.WriteFile(path, []byte(result), 0o600)
}

// updateEnvLine attempts to replace the value in an .env line matching envKey.
// Returns the new line and true if the line matched; otherwise "" and false.
// If the new value equals the default, the line is commented out.
// If the new value differs from the default, the line is uncommented/updated.
func updateEnvLine(line, envKey, newVal string) (string, bool) {
	matches := envVarLineRegex.FindStringSubmatch(line)
	if matches == nil {
		return "", false
	}
	if matches[2] != envKey {
		return "", false
	}

	defaultVal := defaultLookup[reverseFieldLookup(envKey)]

	// If the new value matches the default, comment it out.
	if newVal == defaultVal {
		return fmt.Sprintf("#%s=%s", envKey, defaultVal), true
	}

	// Otherwise, ensure the line is active and updated.
	return fmt.Sprintf("%s=%s", envKey, newVal), true
}

// collectUpdates flattens the configUpdateRequest into a map of env var names to
// their string representation.
func collectUpdates(req configUpdateRequest) map[string]string {
	updates := make(map[string]string)

	addIf := func(field string, val string) {
		if envKey, ok := fieldNameToEnvKey[field]; ok && val != "" {
			// Never overwrite with the masked sentinel.
			if field == "llm.api_key" && val == maskedAPIKey {
				return
			}
			updates[envKey] = val
		}
	}
	addBool := func(field string, val *bool) {
		if val != nil {
			addIf(field, strconv.FormatBool(*val))
		}
	}
	addInt := func(field string, val *int) {
		if val != nil {
			addIf(field, strconv.Itoa(*val))
		}
	}

	addIf("api_address", derefStr(req.APIAddress))
	addIf("proxy_address", derefStr(req.ProxyAddress))
	addIf("database_uri", derefStr(req.DatabaseURI))
	addIf("artifact_dir", derefStr(req.ArtifactDir))
	addIf("req_timeout", derefStr(req.ReqTimeout))
	addIf("shutdown_timeout", derefStr(req.ShutdownTimeout))
	addBool("tls_skip_verify", req.TLSSkipVerify)
	addInt("workers", req.Workers)
	addInt("rps", req.RPS)
	addInt("max_body_bytes", req.MaxBodyBytes)

	if req.TLS != nil {
		addIf("tls.min_version", req.TLS.MinVersion)
		addIf("tls.handshake_timeout", req.TLS.HandshakeTimeout)
		addBool("tls.disable_session_tickets", &req.TLS.DisableSessionTickets)
	}
	if req.CertCache != nil {
		addInt("cert_cache.max_entries", &req.CertCache.MaxEntries)
		addBool("cert_cache.memory_only", &req.CertCache.MemoryOnly)
		addIf("cert_cache.cert_dir", req.CertCache.CertDir)
	}
	if req.LLM != nil {
		addBool("llm.enabled", &req.LLM.Enabled)
		addIf("llm.provider", req.LLM.Provider)
		addIf("llm.api_key", req.LLM.APIKey)
		addIf("llm.base_url", req.LLM.BaseURL)
		addIf("llm.model", req.LLM.Model)
		addInt("llm.max_tokens", &req.LLM.MaxTokens)
		addIf("llm.timeout", req.LLM.Timeout)
	}

	return updates
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// reverseFieldLookup maps an env key back to a field path for default lookup.
func reverseFieldLookup(envKey string) string {
	for field, key := range fieldNameToEnvKey {
		if key == envKey {
			return field
		}
	}
	return ""
}

// joinLines joins output lines preserving the trailing newline behavior of
// the original file.
func joinLines(lines []string, originalData []byte) string {
	result := strings.Join(lines, "\n")

	// Strip trailing newline from last line if original had no trailing content.
	if len(originalData) > 0 && originalData[len(originalData)-1] != '\n' {
		result = strings.TrimRight(result, "\n")
	}
	return result
}
