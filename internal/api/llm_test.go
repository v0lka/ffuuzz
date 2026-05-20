package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadFileContext_validFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := []byte(`{"key":"value"}`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	data, err := readFileContext(context.Background(), path)
	if err != nil {
		t.Fatalf("readFileContext: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("got %q, want %q", data, content)
	}
}

func TestReadFileContext_cancelledContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readFileContext(ctx, path)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got err=%v, want context.Canceled", err)
	}
}

func TestReadFileContext_expiredContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	<-ctx.Done()

	_, err := readFileContext(ctx, path)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got err=%v, want context.DeadlineExceeded", err)
	}
}

func TestReadFileContext_nonexistentFile(t *testing.T) {
	_, err := readFileContext(context.Background(), "/no/such/file.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got context error %v, want OS error", err)
	}
}
