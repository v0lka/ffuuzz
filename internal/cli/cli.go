// Package cli provides command-line interface commands for ffuuzz.
package cli

import (
	"fmt"
	"io"
	"os"
)

// CLI represents the command-line interface with all available commands.
type CLI struct {
	stdout io.Writer
	stderr io.Writer
}

// New creates a new CLI instance.
func New(stdout, stderr io.Writer) *CLI {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	return &CLI{
		stdout: stdout,
		stderr: stderr,
	}
}

// Run executes the CLI with the given arguments.
func (c *CLI) Run(args []string) int {
	if len(args) < 1 {
		c.printUsage()
		return 1
	}

	cmd := args[0]

	switch cmd {
	case "serve":
		return c.runServe(args[1:])
	case "proxy":
		return c.runProxy(args[1:])
	case "record":
		return c.runRecord(args[1:])
	default:
		_, _ = fmt.Fprintf(c.stderr, "unknown command: %q\n\n", cmd)
		c.printUsage()
		return 1
	}
}

func (c *CLI) printUsage() {
	_, _ = fmt.Fprintf(c.stderr, `usage: ffuuzz <command> [args]

commands:
  serve    run full ffuuzz (proxy + API + engine)
  proxy    run MITM proxy only (dev mode)
  record   analyze recorded JSONL log

use "ffuuzz <command> -h" for command-specific flags
`)
}
