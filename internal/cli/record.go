package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"ffuuzz/internal/recorder"
	"ffuuzz/internal/report"
)

func (c *CLI) runRecord(args []string) int {
	fs := flag.NewFlagSet("record", flag.ExitOnError)

	in := fs.String("in", "log.jsonl", "jsonl input file with TxRecord lines")

	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprintf(c.stderr, "parse flags: %v\n", err)
		return 1
	}

	f, err := os.Open(*in)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "open log file: %v\n", err)
		return 1
	}
	defer func() { _ = f.Close() }()

	var records []recorder.TxRecord

	scanner := bufio.NewScanner(f)
	const maxLine = 10 * 1024 * 1024
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxLine)

	for scanner.Scan() {
		line := scanner.Bytes()
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var tx recorder.TxRecord
		if err := json.Unmarshal(line, &tx); err != nil {
			_, _ = fmt.Fprintf(c.stderr, "unmarshal tx record: %v (line: %s)\n", err, string(line))
			return 1
		}
		records = append(records, tx)
	}

	if err := scanner.Err(); err != nil {
		_, _ = fmt.Fprintf(c.stderr, "scan log file: %v\n", err)
		return 1
	}

	summary := report.BuildSummary(records)

	enc := json.NewEncoder(c.stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary); err != nil {
		_, _ = fmt.Fprintf(c.stderr, "encode summary: %v\n", err)
		return 1
	}

	return 0
}
