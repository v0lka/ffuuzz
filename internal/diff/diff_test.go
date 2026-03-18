package diff

import (
	"testing"

	"ffuuzz/internal/recorder"
)

func TestDiffTxRecords_NoDiffs(t *testing.T) {
	a := recorder.TxRecord{RequestID: "a", URL: "http://example.com/api", RespStatus: 200}
	b := recorder.TxRecord{RequestID: "b", URL: "http://example.com/api", RespStatus: 200}
	d := DiffTxRecords(a, b)
	if d.RequestIDA != "a" || d.RequestIDB != "b" {
		t.Errorf("IDs wrong: %q, %q", d.RequestIDA, d.RequestIDB)
	}
	if len(d.Diffs) != 0 {
		t.Fatalf("expected 0 diffs, got %d", len(d.Diffs))
	}
}

func TestDiffTxRecords_URLDiff(t *testing.T) {
	a := recorder.TxRecord{RequestID: "a", URL: "http://example.com/old", RespStatus: 200}
	b := recorder.TxRecord{RequestID: "b", URL: "http://example.com/new", RespStatus: 200}
	d := DiffTxRecords(a, b)
	if len(d.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(d.Diffs))
	}
	if d.Diffs[0].Field != "url" {
		t.Errorf("diff field = %q, want url", d.Diffs[0].Field)
	}
	if d.Diffs[0].Old != "http://example.com/old" {
		t.Errorf("old = %q", d.Diffs[0].Old)
	}
	if d.Diffs[0].New != "http://example.com/new" {
		t.Errorf("new = %q", d.Diffs[0].New)
	}
}

func TestDiffTxRecords_StatusDiff(t *testing.T) {
	a := recorder.TxRecord{RequestID: "a", URL: "http://example.com/api", RespStatus: 200}
	b := recorder.TxRecord{RequestID: "b", URL: "http://example.com/api", RespStatus: 500}
	d := DiffTxRecords(a, b)
	if len(d.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(d.Diffs))
	}
	if d.Diffs[0].Field != "resp_status" {
		t.Errorf("diff field = %q, want resp_status", d.Diffs[0].Field)
	}
}

func TestDiffTxRecords_BothDiffs(t *testing.T) {
	a := recorder.TxRecord{RequestID: "a", URL: "http://a.com", RespStatus: 200}
	b := recorder.TxRecord{RequestID: "b", URL: "http://b.com", RespStatus: 404}
	d := DiffTxRecords(a, b)
	if len(d.Diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %d", len(d.Diffs))
	}
}
