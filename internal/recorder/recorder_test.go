package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRecorder_RecordConcurrent(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "ffuuzz_test_jsonl.log")

	rec, err := NewJSONL(tmp)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer rec.Close()

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			tx := &TxRecord{
				RequestID:  "req-" + time.Now().Format("150405.000000000"),
				Time:       time.Now(),
				Method:     "GET",
				URL:        "http://example.org/" + time.Now().Format("150405"),
				ReqHeaders: map[string][]string{"h": {"v"}},
				ReqBody:    "",
				ReqTrunc:   false,
				RespStatus: 200,
			}
			if err := rec.Record(tx); err != nil {
				t.Errorf("record err: %v", err)
			}
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != N {
		t.Fatalf("expected %d lines, got %d", N, lines)
	}

	var tx TxRecord
	firstLine := data
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			firstLine = data[:i]
			break
		}
	}
	if err := json.Unmarshal(firstLine, &tx); err != nil {
		t.Fatalf("unmarshal first line: %v", err)
	}
}
