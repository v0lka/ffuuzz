package logging

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNew_DefaultWriter(t *testing.T) {
	logger := New(nil)
	// Should not panic
	logger.Info().Msg("test")
}

func TestNew_CustomWriter(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)
	logger.Info().Msg("hello")

	if buf.Len() == 0 {
		t.Fatal("expected output written to custom writer")
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if entry["message"] != "hello" {
		t.Errorf("message = %q, want %q", entry["message"], "hello")
	}
	if _, ok := entry["time"]; !ok {
		t.Error("expected timestamp field")
	}
}

func TestWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)
	child := WithRequestID(logger, "req-123")
	child.Info().Msg("test")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry["request_id"] != "req-123" {
		t.Errorf("request_id = %q, want %q", entry["request_id"], "req-123")
	}
}

func TestWithCampaignID(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)
	child := WithCampaignID(logger, "camp-456")
	child.Info().Msg("test")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry["campaign_id"] != "camp-456" {
		t.Errorf("campaign_id = %q, want %q", entry["campaign_id"], "camp-456")
	}
}

func TestWithRecordingID(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)
	child := WithRecordingID(logger, "rec-789")
	child.Info().Msg("test")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry["recording_id"] != "rec-789" {
		t.Errorf("recording_id = %q, want %q", entry["recording_id"], "rec-789")
	}
}
