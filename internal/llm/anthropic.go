// Package llm provides LLM provider implementations for AI-assisted triage.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/rs/zerolog"

	"ffuuzz/internal/config"
	"ffuuzz/internal/model"
	"ffuuzz/internal/triage"
)

type anthropicProvider struct {
	client    anthropic.Client
	model     anthropic.Model
	maxTokens int64
	timeout   time.Duration
	logger    zerolog.Logger
}

func newAnthropicProvider(cfg config.LLMConfig, logger zerolog.Logger) *anthropicProvider {
	model := anthropic.Model(cfg.Model)
	if cfg.Model == "" {
		model = anthropic.ModelClaudeSonnet4_6
	}

	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	opts = append(opts, option.WithHTTPClient(&http.Client{
		Timeout: cfg.Timeout + 10*time.Second,
	}))

	return &anthropicProvider{
		client:    anthropic.NewClient(opts...),
		model:     model,
		maxTokens: int64(cfg.MaxTokens),
		timeout:   cfg.Timeout,
		logger:    logger,
	}
}

func (p *anthropicProvider) AnalyzeFinding(ctx context.Context, req triage.LLMAnalysisRequest) (*model.LLMAnalysis, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	prompt := triage.BuildAnalyzeFindingPrompt(req)
	msg, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		System: []anthropic.TextBlockParam{
			{Text: triage.AnalyzeFindingSystemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic messages: %w", err)
	}

	if len(msg.Content) == 0 {
		return nil, fmt.Errorf("anthropic returned no content")
	}

	// Extract text from text content blocks
	var content string
	for _, block := range msg.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return parseAnthropicJSON(content)
}

func (p *anthropicProvider) GenerateDescription(ctx context.Context, req triage.LLMDescriptionRequest) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	prompt := triage.BuildDescriptionPrompt(req)
	msg, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: 256,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("anthropic description: %w", err)
	}

	for _, block := range msg.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", nil
}

func (p *anthropicProvider) GenerateReport(ctx context.Context, findings []triage.LLMReportInput) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	prompt := triage.BuildReportPrompt(findings)
	msg, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("anthropic report: %w", err)
	}

	for _, block := range msg.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", nil
}

// jsonPattern extracts the first JSON object from Anthropic's text response.
var jsonPattern = regexp.MustCompile(`\{[\s\S]*\}`)

func parseAnthropicJSON(content string) (*model.LLMAnalysis, error) {
	match := jsonPattern.FindString(content)
	if match == "" {
		return nil, fmt.Errorf("no JSON object found in anthropic response")
	}

	match = strings.TrimSpace(match)

	var result llmResponseJSON
	if err := json.Unmarshal([]byte(match), &result); err != nil {
		return nil, fmt.Errorf("parse anthropic response: %w", err)
	}

	return &model.LLMAnalysis{
		Classification: result.Classification,
		Severity:       model.Severity(strings.ToUpper(result.Severity)),
		Confidence:     result.Confidence,
		Exploitability: result.Exploitability,
		Remediation:    result.Remediation,
		Description:    result.Description,
		AnalyzedAt:     time.Now().UTC(),
		ModelUsed:      "anthropic",
	}, nil
}
