package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"ffuuzz/internal/mitm"
	"ffuuzz/internal/recorder"
	"ffuuzz/internal/report"
	"ffuuzz/internal/store"
)

// поддержка команд proxy(запуск), record
func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "proxy":
		runProxy(os.Args[2:])
	case "record":
		runRecord(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `usage: ffuuzz <command> [args]

commands:
  proxy    run MITM proxy
  record   check summ 

use "ffuuzz <command> -h" for command-specific flags
`)
}

func runProxy(args []string) {
	fs := flag.NewFlagSet("proxy", flag.ExitOnError)

	port := fs.Int("port", 8080, "port to listen on")
	out := fs.String("out", "log.jsonl", "jsonl output file")
	cadir := fs.String("cadir", "certs", "certs directory (for root CA and leaf certs)")
	maxBodyKB := fs.Int("maxbodykb", 64, "max body KB to record")

	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	rec, err := recorder.NewJSONL(*out)
	if err != nil {
		log.Fatalf("failed to initialize recorder: %v", err)
	}
	defer func() {
		if err := rec.Close(); err != nil {
			log.Printf("warning: error closing recorder: %v", err)
		}
	}()

	cs, err := store.NewCertStore(*cadir)
	if err != nil {
		log.Fatalf("failed to create certstore: %v", err)
	}

	cfg := mitm.Config{
		ListenAddr:   ":" + strconv.Itoa(*port),
		CertStore:    cs,
		Recorder:     rec,
		MaxBodyBytes: (*maxBodyKB) * 1024,
	}

	log.Printf("starting ffuuzz proxy on %s, writing to %s", cfg.ListenAddr, *out)
	proxy := mitm.New(cfg)
	if err := proxy.ListenAndServe(); err != nil {
		log.Fatalf("proxy exited: %v", err)
	}
}

func runRecord(args []string) {
	fs := flag.NewFlagSet("record", flag.ExitOnError)

	in := fs.String("in", "log.jsonl", "jsonl input file with TxRecord lines")

	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	f, err := os.Open(*in)
	if err != nil {
		log.Fatalf("open log file: %v", err)
	}
	defer f.Close()

	var records []recorder.TxRecord

	scanner := bufio.NewScanner(f)
	// увел. лимит на строку
	const maxLine = 10 * 1024 * 1024 // 10MB
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
			log.Fatalf("unmarshal tx record: %v (line: %s)", err, string(line))
		}
		records = append(records, tx)
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("scan log file: %v", err)
	}

	summary := report.BuildSummary(records)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary); err != nil {
		log.Fatalf("encode summary: %v", err)
	}
}
