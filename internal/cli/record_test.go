package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ffuuzz/internal/recorder"
	"ffuuzz/internal/report"
)

func TestCLI_runRecord_Success(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a test JSONL file with sample records
	records := []recorder.TxRecord{
		{
			RequestID:   "req-1",
			Time:        time.Now(),
			Method:      "GET",
			URL:         "http://example.com/api/users",
			ReqHeaders:  map[string][]string{"Content-Type": {"application/json"}},
			RespStatus:  200,
			RespHeaders: map[string][]string{"Content-Type": {"application/json"}},
		},
		{
			RequestID:   "req-2",
			Time:        time.Now(),
			Method:      "POST",
			URL:         "http://example.com/api/users",
			ReqHeaders:  map[string][]string{"Content-Type": {"application/json"}},
			ReqBody:     "eyJuYW1lIjoidGVzdCJ9", // base64 of {"name":"test"}
			RespStatus:  201,
			RespHeaders: map[string][]string{"Content-Type": {"application/json"}},
			RespBody:    "eyJpZCI6MSwibmFtZSI6InRlc3QifQ==",
		},
	}

	jsonlFile := filepath.Join(tmpDir, "test.jsonl")
	f, err := os.Create(jsonlFile)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	for _, rec := range records {
		data, _ := json.Marshal(rec)
		_, _ = f.Write(data)
		_, _ = f.WriteString("\n")
	}
	_ = f.Close()

	// Run the record command
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := New(stdout, stderr)

	exitCode := c.runRecord([]string{"-in", jsonlFile})

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// Verify output is valid JSON
	var summary report.Summary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Errorf("output is not valid JSON: %v\nOutput: %s", err, stdout.String())
	}

	// Verify summary contains expected data
	if summary.Total != 2 {
		t.Errorf("expected Total=2, got %d", summary.Total)
	}
}

func TestCLI_runRecord_MissingFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := New(stdout, stderr)

	exitCode := c.runRecord([]string{"-in", "/nonexistent/path/file.jsonl"})

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "open log file") {
		t.Errorf("expected 'open log file' error in stderr, got: %s", stderrStr)
	}
}

func TestCLI_runRecord_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlFile := filepath.Join(tmpDir, "invalid.jsonl")

	if err := os.WriteFile(jsonlFile, []byte("not valid json\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := New(stdout, stderr)

	exitCode := c.runRecord([]string{"-in", jsonlFile})

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "unmarshal") {
		t.Errorf("expected 'unmarshal' error in stderr, got: %s", stderrStr)
	}
}

func TestCLI_runRecord_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlFile := filepath.Join(tmpDir, "empty.jsonl")

	if err := os.WriteFile(jsonlFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := New(stdout, stderr)

	exitCode := c.runRecord([]string{"-in", jsonlFile})

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// Verify output is valid JSON with zero records
	var summary report.Summary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}

	if summary.Total != 0 {
		t.Errorf("expected Total=0 for empty file, got %d", summary.Total)
	}
}

func TestCLI_runRecord_DefaultInput(t *testing.T) {
	// Test with default input file (log.jsonl in current directory)
	// This should fail since the file doesn't exist
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := New(stdout, stderr)

	// Change to a temp directory to avoid finding an actual log.jsonl
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("failed to restore directory: %v", err)
		}
	}()

	exitCode := c.runRecord([]string{})

	if exitCode != 1 {
		t.Errorf("expected exit code 1 for missing default file, got %d", exitCode)
	}
}

func TestCLI_runRecord_WithWhitespaceLines(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlFile := filepath.Join(tmpDir, "whitespace.jsonl")

	// Create file with empty lines and whitespace
	content := `

{"Request":{"Method":"GET","URL":"http://example.com"},"Response":{"StatusCode":200}}

   
`
	if err := os.WriteFile(jsonlFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := New(stdout, stderr)

	exitCode := c.runRecord([]string{"-in", jsonlFile})

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	var summary report.Summary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}

	if summary.Total != 1 {
		t.Errorf("expected Total=1 (only one non-empty line), got %d", summary.Total)
	}
}
