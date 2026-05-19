// Package llm provides LLM provider implementations for AI-assisted triage.
package llm

import (
	"errors"

	"github.com/rs/zerolog"

	"ffuuzz/internal/config"
	"ffuuzz/internal/triage"
)

// llmResponseJSON is the shared JSON response format from all LLM providers
// for vulnerability analysis.
type llmResponseJSON struct {
	Classification string  `json:"classification"`
	Severity       string  `json:"severity"`
	Confidence     float64 `json:"confidence"`
	Exploitability string  `json:"exploitability"`
	Remediation    string  `json:"remediation"`
	Description    string  `json:"description"`
}

// NewProvider creates an LLM provider based on configuration.
// Returns (nil, nil) when cfg.Enabled is false — meaning LLM is gracefully disabled.
// Returns an error when cfg.Enabled is true but required fields are missing.
func NewProvider(cfg config.LLMConfig, logger zerolog.Logger) (triage.LLMProvider, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	if cfg.APIKey == "" {
		return nil, errors.New("llm enabled but API key is not configured")
	}

	switch cfg.Provider {
	case "openai":
		return newOpenAIProvider(cfg, logger), nil
	case "anthropic":
		return newAnthropicProvider(cfg, logger), nil
	default:
		return nil, errors.New("unknown llm provider: " + cfg.Provider + " (must be 'openai' or 'anthropic')")
	}
}
