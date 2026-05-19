// Package llm provides LLM provider implementations for AI-assisted triage.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/sashabaranov/go-openai"

	"ffuuzz/internal/config"
	"ffuuzz/internal/model"
	"ffuuzz/internal/triage"
)

type openAIProvider struct {
	client    *openai.Client
	model     string
	maxTokens int
	timeout   time.Duration
	logger    zerolog.Logger
}

func newOpenAIProvider(cfg config.LLMConfig, logger zerolog.Logger) *openAIProvider {
	clientCfg := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		clientCfg.BaseURL = cfg.BaseURL
	}
	return &openAIProvider{
		client:    openai.NewClientWithConfig(clientCfg),
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
		timeout:   cfg.Timeout,
		logger:    logger,
	}
}

func (p *openAIProvider) AnalyzeFinding(ctx context.Context, req triage.LLMAnalysisRequest) (*model.LLMAnalysis, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	prompt := triage.BuildAnalyzeFindingPrompt(req)

	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: p.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: triage.AnalyzeFindingSystemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		MaxTokens:   p.maxTokens,
		Temperature: 0.1,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("openai chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	content := resp.Choices[0].Message.Content
	return parseAnalysisJSON(content)
}

func (p *openAIProvider) GenerateDescription(ctx context.Context, req triage.LLMDescriptionRequest) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	prompt := triage.BuildDescriptionPrompt(req)

	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: p.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		MaxTokens:   256,
		Temperature: 0.3,
	})
	if err != nil {
		return "", fmt.Errorf("openai description: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}
	return resp.Choices[0].Message.Content, nil
}

func (p *openAIProvider) GenerateReport(ctx context.Context, findings []triage.LLMReportInput) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	prompt := triage.BuildReportPrompt(findings)

	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: p.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		MaxTokens:   p.maxTokens,
		Temperature: 0.3,
	})
	if err != nil {
		return "", fmt.Errorf("openai report: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}
	return resp.Choices[0].Message.Content, nil
}

// parseAnalysisJSON parses LLM JSON output into an LLMAnalysis struct.
func parseAnalysisJSON(content string) (*model.LLMAnalysis, error) {
	var result llmResponseJSON
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse llm response: %w", err)
	}

	return &model.LLMAnalysis{
		Classification: result.Classification,
		Severity:       model.Severity(result.Severity),
		Confidence:     result.Confidence,
		Exploitability: result.Exploitability,
		Remediation:    result.Remediation,
		Description:    result.Description,
		AnalyzedAt:     time.Now().UTC(),
		ModelUsed:      "openai",
	}, nil
}
