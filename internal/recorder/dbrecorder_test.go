package recorder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

type mockRecordingInserter struct {
	findOrAppendFn func(ctx context.Context, sess model.RecordingSession) (string, bool, error)
}

func (m *mockRecordingInserter) FindOrAppend(ctx context.Context, sess model.RecordingSession) (string, bool, error) {
	if m.findOrAppendFn != nil {
		return m.findOrAppendFn(ctx, sess)
	}
	return "sess-1", false, nil
}

func TestNewDBRecorder(t *testing.T) {
	store := &mockRecordingInserter{}
	rec := NewDBRecorder(store, nil, zerolog.Nop())
	if rec == nil {
		t.Fatal("expected non-nil DBRecorder")
	}
	if rec.store != store {
		t.Error("store not set")
	}
}

func TestDBRecorder_Record_Basic(t *testing.T) {
	var captured model.RecordingSession
	store := &mockRecordingInserter{
		findOrAppendFn: func(ctx context.Context, sess model.RecordingSession) (string, bool, error) {
			captured = sess
			return "sess-1", true, nil
		},
	}

	rec := NewDBRecorder(store, nil, zerolog.Nop())
	tx := &TxRecord{
		RequestID:  "req-1",
		Time:       time.Now(),
		Method:     "GET",
		URL:        "http://example.com/api/users?page=1",
		ReqHeaders: map[string][]string{"Accept": {"application/json"}},
		RespStatus: 200,
	}

	err := rec.Record(tx)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	if captured.Target.Scheme != "http" {
		t.Errorf("scheme = %q, want http", captured.Target.Scheme)
	}
	if captured.Target.Host != "example.com" {
		t.Errorf("host = %q, want example.com", captured.Target.Host)
	}
	if captured.Target.Port != 80 {
		t.Errorf("port = %d, want 80", captured.Target.Port)
	}
	if captured.EntryCount != 1 {
		t.Errorf("entry count = %d, want 1", captured.EntryCount)
	}
}

func TestDBRecorder_Record_HTTPS(t *testing.T) {
	var captured model.RecordingSession
	store := &mockRecordingInserter{
		findOrAppendFn: func(ctx context.Context, sess model.RecordingSession) (string, bool, error) {
			captured = sess
			return "sess-1", false, nil
		},
	}

	rec := NewDBRecorder(store, nil, zerolog.Nop())
	tx := &TxRecord{
		RequestID:  "req-2",
		Time:       time.Now(),
		Method:     "POST",
		URL:        "https://api.example.com/v1/data",
		RespStatus: 201,
	}

	err := rec.Record(tx)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	if captured.Target.Scheme != "https" {
		t.Errorf("scheme = %q, want https", captured.Target.Scheme)
	}
	if captured.Target.Port != 443 {
		t.Errorf("port = %d, want 443 for HTTPS", captured.Target.Port)
	}
}

func TestDBRecorder_Record_CustomPort(t *testing.T) {
	var captured model.RecordingSession
	store := &mockRecordingInserter{
		findOrAppendFn: func(ctx context.Context, sess model.RecordingSession) (string, bool, error) {
			captured = sess
			return "sess-1", false, nil
		},
	}

	rec := NewDBRecorder(store, nil, zerolog.Nop())
	tx := &TxRecord{
		RequestID:  "req-3",
		Time:       time.Now(),
		Method:     "GET",
		URL:        "http://localhost:9090/health",
		RespStatus: 200,
	}

	err := rec.Record(tx)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	if captured.Target.Port != 9090 {
		t.Errorf("port = %d, want 9090", captured.Target.Port)
	}
}

func TestDBRecorder_Record_InvalidURL(t *testing.T) {
	rec := NewDBRecorder(&mockRecordingInserter{}, nil, zerolog.Nop())
	tx := &TxRecord{
		RequestID: "req-err",
		URL:       "://bad-url",
	}
	err := rec.Record(tx)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestDBRecorder_Record_StoreError(t *testing.T) {
	store := &mockRecordingInserter{
		findOrAppendFn: func(ctx context.Context, sess model.RecordingSession) (string, bool, error) {
			return "", false, errors.New("db down")
		},
	}

	rec := NewDBRecorder(store, nil, zerolog.Nop())
	tx := &TxRecord{
		RequestID:  "req-err",
		Time:       time.Now(),
		Method:     "GET",
		URL:        "http://example.com/test",
		RespStatus: 200,
	}

	err := rec.Record(tx)
	if err == nil {
		t.Fatal("expected error from store")
	}
}

func TestDBRecorder_Record_NoScheme(t *testing.T) {
	var captured model.RecordingSession
	store := &mockRecordingInserter{
		findOrAppendFn: func(ctx context.Context, sess model.RecordingSession) (string, bool, error) {
			captured = sess
			return "sess-1", false, nil
		},
	}

	rec := NewDBRecorder(store, nil, zerolog.Nop())
	tx := &TxRecord{
		RequestID:  "req-no-scheme",
		Time:       time.Now(),
		Method:     "GET",
		URL:        "http:///path/only",
		RespStatus: 200,
	}

	err := rec.Record(tx)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	// empty host should default to "unknown"
	if captured.Target.Host != "unknown" {
		t.Errorf("host = %q, want unknown", captured.Target.Host)
	}
}

func TestDBRecorder_Close(t *testing.T) {
	rec := NewDBRecorder(&mockRecordingInserter{}, nil, zerolog.Nop())
	err := rec.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
}
