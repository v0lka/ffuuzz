package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		stdout  *bytes.Buffer
		stderr  *bytes.Buffer
		wantNil bool
	}{
		{
			name:    "with provided writers",
			stdout:  &bytes.Buffer{},
			stderr:  &bytes.Buffer{},
			wantNil: false,
		},
		{
			name:    "with nil writers uses defaults",
			stdout:  nil,
			stderr:  nil,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.stdout, tt.stderr)
			if c == nil {
				t.Fatal("New() returned nil")
			}
			if c.stdout == nil {
				t.Error("stdout is nil")
			}
			if c.stderr == nil {
				t.Error("stderr is nil")
			}
		})
	}
}

func TestCLI_Run_UnknownCommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := New(stdout, stderr)

	exitCode := c.Run([]string{"unknown-cmd"})

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "unknown command") {
		t.Errorf("expected 'unknown command' in stderr, got: %s", stderrStr)
	}
}

func TestCLI_Run_NoCommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := New(stdout, stderr)

	exitCode := c.Run([]string{})

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "usage:") {
		t.Errorf("expected usage info in stderr, got: %s", stderrStr)
	}
}

func TestCLI_printUsage(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := New(stdout, stderr)

	c.printUsage()

	output := stderr.String()
	expectedCommands := []string{"serve", "proxy", "record"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(output, cmd) {
			t.Errorf("expected usage to contain command %q, got: %s", cmd, output)
		}
	}
}
