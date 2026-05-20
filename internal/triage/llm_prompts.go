// Package triage provides finding analysis, confirmation, minimization,
// severity scoring, and LLM-assisted triage.
package triage

import (
	"fmt"
	"strings"
	"time"
)

// MaxResponseSnippetLen is the max chars of response body sent to an LLM.
const MaxResponseSnippetLen = 4096

// AnalyzeFindingSystemPrompt is the system prompt for classifying a single finding.
// It requests structured JSON output.
const AnalyzeFindingSystemPrompt = `You are a security triage expert analyzing HTTP fuzzing findings.
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
- Stack traces, debug info, internal paths, or sensitive file contents in error responses → "Information Disclosure"
- Timeouts (ONLY for 408/504 with abnormally long delays), resource exhaustion from oversized payloads → "Denial of Service"
- SQL error messages, 5xx on SQL payloads, or blind-injection signs (SLEEP delays, boolean-based response differences) → "SQL Injection"
- Shell command output reflected in response, or blind time-based signs (sleep commands causing ≥5s delays) → "Command Injection"
- Unescaped HTML/JS payload reflected in response body or headers → "Cross-Site Scripting"
- File contents, directory listings, or path-based errors from ../ sequences → "Path Traversal"
- JNDI/LDAP/RMI/DNS callback signs, "lookup" errors, or ${jndi:...} payload responses → "JNDI Injection"
- Template expression ({{...}}, ${...}, #{...}, <%=...%>) evaluated and reflected as computed result → "Server-Side Template Injection"
- Internal network resources accessed (169.254.169.254, 127.0.0.1, file://, gopher://) reflected or causing unique errors → "Server-Side Request Forgery"
- XML parser errors referencing external entities, or file contents leaked via <!ENTITY/<!DOCTYPE declarations → "XML External Entity"
- __proto__/constructor/prototype pollution causing unexpected property access or type confusion → "Prototype Pollution"
- JWT/cookie manipulation bypassing auth checks (200 OK on previously-protected endpoints) → "Authentication Bypass"
- CRLF sequences or reflected input appearing in HTTP response headers → "Header Injection"
- Latency spikes without other vulnerability indicators → "Performance Anomaly"
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
// When req.PreviousAnalysis is non-nil, the prompt includes the previous analysis
// result so the LLM can re-evaluate with full context.
func BuildAnalyzeFindingPrompt(req LLMAnalysisRequest) string {
	snippet := req.ResponseSnippet
	if len(snippet) > MaxResponseSnippetLen {
		snippet = snippet[:MaxResponseSnippetLen] + "\n...[truncated]"
	}

	var previous string
	if req.PreviousAnalysis != nil {
		previous = fmt.Sprintf(`
Previous Analysis Result:
  Classification: %s
  Severity: %s
  Confidence: %.2f
  Exploitability: %s
  Remediation: %s
  Description: %s
  Analyzed At: %s
  Model Used: %s
`, req.PreviousAnalysis.Classification,
			req.PreviousAnalysis.Severity,
			req.PreviousAnalysis.Confidence,
			req.PreviousAnalysis.Exploitability,
			req.PreviousAnalysis.Remediation,
			req.PreviousAnalysis.Description,
			req.PreviousAnalysis.AnalyzedAt.Format(time.RFC3339),
			req.PreviousAnalysis.ModelUsed)
	}

	return fmt.Sprintf(`Analyze this fuzzing finding:
%s
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
		previous,
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
