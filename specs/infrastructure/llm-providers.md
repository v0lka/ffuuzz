# LLM Providers

## Responsibility

Provides concrete LLM provider implementations (OpenAI and Anthropic) for AI-assisted triage. The factory function (`NewProvider`) supports graceful degradation: when LLM is disabled or improperly configured, it returns `nil, nil` so callers operate without LLM assistance.

## Key Files

| File | Role |
|------|------|
| `internal/llm/factory.go` | `NewProvider()` factory, `llmResponseJSON` shared types |
| `internal/llm/openai.go` | `openAIProvider`: OpenAI implementation using `go-openai` SDK with structured JSON output |
| `internal/llm/anthropic.go` | `anthropicProvider`: Anthropic implementation using `anthropic-sdk-go` with regex JSON extraction |

## Core Types

```go
// llmResponseJSON is the shared JSON response format from all LLM providers.
// Not exported — used internally by parseAnalysisJSON and parseAnthropicJSON.
type llmResponseJSON struct {
    Classification string  `json:"classification"`
    Severity       string  `json:"severity"`
    Confidence     float64 `json:"confidence"`
    Exploitability string  `json:"exploitability"`
    Remediation    string  `json:"remediation"`
    Description    string  `json:"description"`
}
```

Both provider structs are unexported. They implement `triage.LLMProvider`:

```go
type LLMProvider interface {
    AnalyzeFinding(ctx, LLMAnalysisRequest) (*model.LLMAnalysis, error)
    GenerateDescription(ctx, LLMDescriptionRequest) (string, error)
    GenerateReport(ctx, []LLMReportInput) (string, error)
}
```

## Public API

### `NewProvider(cfg config.LLMConfig, logger zerolog.Logger) (triage.LLMProvider, error)`

Factory that dispatches based on `cfg.Provider`:

| Condition | Return | Behavior |
|-----------|--------|----------|
| `cfg.Enabled == false` | `nil, nil` | Graceful degradation — LLM is disabled |
| `cfg.APIKey == ""` | `nil, error` | Fatal: LLM enabled but no key configured |
| `cfg.Provider == "openai"` | `*openAIProvider, nil` | OpenAI provider with `go-openai` SDK |
| `cfg.Provider == "anthropic"` | `*anthropicProvider, nil` | Anthropic provider with `anthropic-sdk-go` |
| Other | `nil, error` | Unknown provider name |

## Providers

### OpenAI Provider (`openAIProvider`)

Uses the `go-openai` SDK (`github.com/sashabaranov/go-openai`).

**Model default**: When `cfg.Model` is empty, uses the OpenAI client default (typically `gpt-4o`).

**Base URL override**: When `cfg.BaseURL` is set, it overrides the API endpoint (enabling API-compatible proxies like Ollama, OpenRouter, etc.).

**Response format**: Uses `response_format: json_object` for `AnalyzeFinding` — the API guarantees valid JSON output. Parsed by `parseAnalysisJSON()`.

**Temperature**:
- `AnalyzeFinding`: 0.1 (deterministic)
- `GenerateDescription`: 0.3
- `GenerateReport`: 0.3

**Max tokens**: `cfg.MaxTokens` for `AnalyzeFinding` and `GenerateReport`, 256 for `GenerateDescription`.

**Error handling**:
- Chat completion failure → wrapped error: `"openai chat completion: <err>"`
- Empty choices array → error: `"openai returned no choices"`
- JSON parse failure → error: `"parse llm response: <err>"`
- Timeout: context deadline from `cfg.Timeout`

**Model reported in output**: Always `"openai"` in `LLMAnalysis.ModelUsed`.

### Anthropic Provider (`anthropicProvider`)

Uses the `anthropic-sdk-go` SDK (`github.com/anthropics/anthropic-sdk-go`).

**Model default**: When `cfg.Model` is empty, defaults to `anthropic.ModelClaudeSonnet4_6` (currently `claude-sonnet-4-20250514`).

**Base URL override**: When `cfg.BaseURL` is set, uses `option.WithBaseURL()`.

**Response format**: Unlike OpenAI, Anthropic does not support native structured JSON output. The response is plain text. JSON is extracted from the response text using a greedy regex (`\{[\s\S]*\}`) in `parseAnthropicJSON()`. The first JSON object found in the response is used.

**System prompt**: Passed as a `System` text block parameter (separate from messages).

**Messages API**: Uses `Messages.New()` with `anthropic.MessageNewParams`.

**Max tokens**: `int64(cfg.MaxTokens)` for `AnalyzeFinding` and `GenerateReport`, 256 for `GenerateDescription`.

**Error handling**:
- Messages API failure → wrapped error: `"anthropic messages: <err>"`
- Empty content → error: `"anthropic returned no content"`
- No JSON found in response → error: `"no JSON object found in anthropic response"`
- JSON parse failure → error: `"parse anthropic response: <err>"`
- Timeout: context deadline from `cfg.Timeout`

**Severity normalization**: Anthropic severity is uppercased (e.g., `"high"` → `"HIGH"`) to handle case variations from the LLM.

**Model reported in output**: Always `"anthropic"` in `LLMAnalysis.ModelUsed`.

## JSON Response Parsing

Both providers produce the same `llmResponseJSON` struct, but parsing differs:

| Provider | Parsing method | JSON extraction |
|----------|---------------|-----------------|
| OpenAI | `parseAnalysisJSON` | Direct `json.Unmarshal` of response content |
| Anthropic | `parseAnthropicJSON` | Regex `\{[\s\S]*\}` then `json.Unmarshal` |

Both convert `llmResponseJSON` to `model.LLMAnalysis` with `AnalyzedAt` set to `time.Now().UTC()`.

## Prompt Construction

All prompts are built by functions in `internal/triage/llm_prompts.go`:

- `triage.AnalyzeFindingSystemPrompt` — constant system prompt
- `triage.BuildAnalyzeFindingPrompt(req)` — builds finding-specific prompt
- `triage.BuildDescriptionPrompt(req)` — builds description prompt
- `triage.BuildReportPrompt(findings)` — builds report from finding summaries

The providers do not construct prompts themselves — they call these `triage` package functions.

## Invariants

- `NewProvider` returns `nil, nil` when LLM is disabled. Callers in `cli/serve.go` handle this by leaving `llmTriager` as nil.
- Each provider creates its own HTTP client internally (via the SDK). No shared HTTP client or connection pooling.
- `AnalyzeFinding` always uses a timeout context (from `cfg.Timeout`). Long-running LLM calls cannot block the campaign indefinitely.
- The shared `llmResponseJSON` struct ensures both providers produce the same output format. Adding a new provider only requires implementing `triage.LLMProvider` and returning `*model.LLMAnalysis` via `llmResponseJSON`.
- `LLMAnalysis.ModelUsed` is set per-provider. This allows identifying which provider produced which analysis in the database.
- Anthropic's regex-based JSON extraction is greedy and skips any text before/after the JSON object. This tolerates conversational preambles but may fail if the JSON is malformed.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/config` | `LLMConfig` for provider initialization |
| `internal/model` | `LLMAnalysis`, `Severity` |
| `internal/triage` | `LLMProvider` interface, `LLMAnalysisRequest`, `LLMDescriptionRequest`, `LLMReportInput`, prompt construction |
| `github.com/sashabaranov/go-openai` | OpenAI chat completions API |
| `github.com/anthropics/anthropic-sdk-go` | Anthropic Messages API |

## Edge Cases

- **LLM disabled**: `NewProvider` returns `nil, nil`. No provider is created. The engine and API operate without LLM triage.
- **API key missing but LLM enabled**: `NewProvider` returns an error. Startup fails.
- **Unknown provider**: `NewProvider` returns an error. Startup fails.
- **LLM timeout**: The context deadline from `cfg.Timeout` cancels the request. The provider returns a timeout error.
- **Anthropic response without JSON**: `parseAnthropicJSON` returns error `"no JSON object found in anthropic response"`. The triage layer logs this and continues.
- **Malformed JSON from either provider**: The parse function returns an error. The triage layer logs a warning and does not persist the analysis.
- **Empty model name (OpenAI)**: Uses the OpenAI SDK default.
- **Empty model name (Anthropic)**: Defaults to `anthropic.ModelClaudeSonnet4_6`.
- **BaseURL override**: Both providers apply the custom base URL at client construction. If set, all API calls go to the override endpoint.

## Related

- [`config.md`](config.md) — `LLMConfig` defaults and env var configuration
- [`../domains/triage.md`](../domains/triage.md) — `LLMTriager` orchestration, `LLMProvider` interface definition
- [`../contracts/cli-infrastructure.md`](../contracts/cli-infrastructure.md) — `NewProvider` wiring at startup
