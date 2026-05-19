// Package triage provides finding analysis, confirmation, minimization,
// severity scoring, and LLM-assisted triage.
package triage

import (
	"fmt"
	"strings"
)

// MaxResponseSnippetLen is the max chars of response body sent to an LLM.
const MaxResponseSnippetLen = 2000

// AnalyzeFindingSystemPrompt is the system prompt for classifying a single finding.
// It requests structured JSON output.
const AnalyzeFindingSystemPrompt = `You are a security triage specialist analyzing HTTP fuzzing findings.
For each finding, classify the vulnerability, assess severity, and provide remediation.

You MUST respond with ONLY a valid JSON object with these fields:
{
  "classification": "<vulnerability class or 'No Vulnerability'>",
  "severity": "CRITICAL|HIGH|MEDIUM|LOW|INFO",
  "confidence": <0.0 to 1.0>,
  "exploitability": "<assessment of exploitability>",
  "remediation": "<recommended fix>",
  "description": "<concise human-readable summary of what happened>"
}

Classification guidance:
- Server errors (5xx) that reveal stack traces → "Information Disclosure"
- Timeouts → "Denial of Service"
- SQL error messages → "SQL Injection"
- Command output in response → "Command Injection"
- XSS reflection → "Cross-Site Scripting"
- Path traversal signs → "Path Traversal"
- JWT manipulation → "Authentication Bypass"
- Reflected input in headers → "Header Injection"
- Latency spikes without other signs → "Performance Anomaly"
- No security impact visible → "No Vulnerability"

Severity guidance:
- CRITICAL: confirmed injection, auth bypass, RCE
- HIGH: information disclosure with sensitive data, significant latency increase
- MEDIUM: potential injection indicators, moderate impact
- LOW: minor information disclosure, non-exploitable anomalies
- INFO: noise, false positives, expected behavior

Confidence:
- 0.9-1.0: clear vulnerability with unambiguous evidence
- 0.7-0.89: strong indicators but requires verification
- 0.5-0.69: possible vulnerability, ambiguous evidence
- 0.3-0.49: unlikely but worth investigating
- <0.3: probably false positive or noise`

// BuildAnalyzeFindingPrompt formats the finding analysis prompt with actual data.
func BuildAnalyzeFindingPrompt(req LLMAnalysisRequest) string {
	snippet := req.ResponseSnippet
	if len(snippet) > MaxResponseSnippetLen {
		snippet = snippet[:MaxResponseSnippetLen] + "\n...[truncated]"
	}

	return fmt.Sprintf(`Analyze this fuzzing finding:

Finding ID: %s
Type: %s
HTTP Method: %s
Endpoint: %s
Mutation Type: %s
Mutation Payload: %s
Baseline Status: %d
Anomalous Status: %d

Response Body (anomalous):
%s`,
		req.FindingID,
		req.FindingType,
		req.Method,
		req.Endpoint,
		req.MutationType,
		req.MutationPayload,
		req.BaselineStatus,
		req.AnomalousStatus,
		snippet,
	)
}

// BuildDescriptionPrompt generates a finding description prompt.
func BuildDescriptionPrompt(req LLMDescriptionRequest) string {
	summary := fmt.Sprintf(
		"Write a brief, single-paragraph description of this security finding: Classification=%s, Endpoint=%s, Severity=%s",
		req.Classification, req.Endpoint, req.Severity,
	)
	if req.MutationType != "" {
		summary += fmt.Sprintf(", Mutation=%s", req.MutationType)
	}
	return summary + "\nExplain what happened, why it matters, and how to reproduce it in plain language."
}

// BuildReportPrompt generates a report prompt from multiple findings.
func BuildReportPrompt(inputs []LLMReportInput) string {
	var b strings.Builder
	b.WriteString("Generate an executive summary of the following security findings from a fuzzing campaign.\n")
	b.WriteString("Include: overall assessment, severity breakdown, most critical findings, and recommended next steps.\n\n")
	b.WriteString("Findings:\n")
	for _, f := range inputs {
		fmt.Fprintf(&b, "- [%s] %s | %s | %s | %s\n",
			f.ID, f.Endpoint, f.Type, f.Severity, f.Classification)
	}
	return b.String()
}
