// Command ffuuzz is the main entry point for the web API fuzzing engine.
package main

import (
	"os"

	"ffuuzz/internal/cli"
)

func main() {
	c := cli.New(os.Stdout, os.Stderr)
	os.Exit(c.Run(os.Args[1:]))
}
