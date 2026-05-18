package mutate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"ffuuzz/internal/model"
)

func TestNewDictionary(t *testing.T) {
	d := NewDictionary()
	if d == nil {
		t.Fatal("expected non-nil dictionary")
	}
	vals := d.ValuesForHeader("/api", "X-Test")
	if len(vals) != 0 {
		t.Errorf("empty dict should return no values, got %d", len(vals))
	}
}

func TestDictionary_GlobalValues(t *testing.T) {
	d := NewDictionary()
	d.AddGlobal("X-Custom", []string{"val1"})
	d.AddGlobal("X-Custom", []string{"val2"})

	vals := d.ValuesForHeader("/api/test", "X-Custom")
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d", len(vals))
	}
	if vals[0] != "val1" || vals[1] != "val2" {
		t.Errorf("unexpected values: %v", vals)
	}
}

func TestDictionary_PerEndpointValues(t *testing.T) {
	d := NewDictionary()
	d.AddGlobal("X-Custom", []string{"global"})
	d.AddForEndpoint("/api/users", "X-Custom", []string{"ep-specific"})

	// Global-only lookup
	vals := d.ValuesForHeader("/api/other", "X-Custom")
	if len(vals) != 1 || vals[0] != "global" {
		t.Errorf("expected [global], got %v", vals)
	}

	// Endpoint-specific lookup merges global + per-ep
	vals = d.ValuesForHeader("/api/users", "X-Custom")
	if len(vals) != 2 {
		t.Fatalf("expected 2 values (global + ep-specific), got %d", len(vals))
	}
	hasGlobal := false
	hasEP := false
	for _, v := range vals {
		if v == "global" {
			hasGlobal = true
		}
		if v == "ep-specific" {
			hasEP = true
		}
	}
	if !hasGlobal || !hasEP {
		t.Errorf("expected both global and ep-specific values, got %v", vals)
	}
}

func TestDictionary_LoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dict.json")

	content := dictFileFormat{
		Global: map[string][]string{
			"Authorization": {"Bearer test-token"},
		},
		Endpoints: map[string]map[string][]string{
			"/api/admin": {
				"X-Internal": {"true"},
			},
		},
	}
	data, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	d := NewDictionary()
	if err := d.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	vals := d.ValuesForHeader("/api/users", "Authorization")
	if len(vals) != 1 || vals[0] != "Bearer test-token" {
		t.Errorf("unexpected global values: %v", vals)
	}

	vals = d.ValuesForHeader("/api/admin", "X-Internal")
	if len(vals) != 1 || vals[0] != "true" {
		t.Errorf("unexpected endpoint values: %v", vals)
	}
}

func TestDictionary_LoadFromFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	d := NewDictionary()
	err := d.LoadFromFile(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDictionary_LoadFromFile_NotFound(t *testing.T) {
	d := NewDictionary()
	err := d.LoadFromFile("/nonexistent/dict.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestDictionary_ExtractFromTraffic(t *testing.T) {
	d := NewDictionary()

	sessions := []model.RecordingSession{
		{
			ID: "sess-1",
			Entries: []model.Exchange{
				{
					Request: model.RequestData{
						Method: "GET",
						Path:   "/api/users",
						Headers: map[string][]string{
							"X-Custom":     {"custom-value"},
							"Host":         {"example.com"},
							"User-Agent":   {"Mozilla/5.0 (custom-agent)"},
							"Content-Type": {"application/json"},
						},
					},
				},
				{
					Request: model.RequestData{
						Method: "POST",
						Path:   "/api/admin",
						Headers: map[string][]string{
							"X-Internal":    {"admin-only"},
							"Authorization": {"Bearer secret"},
							"Host":          {"admin.example.com"},
							"Connection":    {"keep-alive"},
						},
					},
				},
			},
		},
	}

	d.ExtractFromTraffic(sessions)

	// Check /api/users has X-Custom
	vals := d.ValuesForHeader("/api/users", "X-Custom")
	if len(vals) != 1 || vals[0] != "custom-value" {
		t.Errorf("expected [custom-value] for /api/users X-Custom, got %v", vals)
	}

	// Check /api/admin has X-Internal and Authorization
	vals = d.ValuesForHeader("/api/admin", "X-Internal")
	if len(vals) != 1 || vals[0] != "admin-only" {
		t.Errorf("expected [admin-only] for /api/admin X-Internal, got %v", vals)
	}
	vals = d.ValuesForHeader("/api/admin", "Authorization")
	if len(vals) != 1 || vals[0] != "Bearer secret" {
		t.Errorf("expected [Bearer secret] for /api/admin Authorization, got %v", vals)
	}

	// Host should be skipped
	vals = d.ValuesForHeader("/api/users", "Host")
	if len(vals) != 0 {
		t.Errorf("Host should be skipped, got %v", vals)
	}

	// Connection should be skipped
	vals = d.ValuesForHeader("/api/admin", "Connection")
	if len(vals) != 0 {
		t.Errorf("Connection should be skipped, got %v", vals)
	}

	// Common user-agent should be filtered
	vals = d.ValuesForHeader("/api/users", "User-Agent")
	if len(vals) != 0 {
		t.Errorf("common User-Agent should be filtered, got %v", vals)
	}
}

func TestDictionary_ExtractFromTraffic_Empty(t *testing.T) {
	d := NewDictionary()
	d.ExtractFromTraffic(nil)
	vals := d.ValuesForHeader("/any", "X-Any")
	if len(vals) != 0 {
		t.Errorf("empty traffic should produce no values, got %d", len(vals))
	}
}

func TestDictionary_Concurrency(t *testing.T) {
	d := NewDictionary()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d.AddGlobal("X-Concurrent", []string{string(rune('a' + i))})
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = d.ValuesForHeader("/api", "X-Concurrent")
		}()
	}

	wg.Wait()
	// If we got here without race detector firing, concurrency is safe
}

func TestNewDictionaryFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dict.json")

	content := dictFileFormat{
		Global: map[string][]string{"X-Test": {"val"}},
	}
	data, _ := json.Marshal(content)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	d, err := NewDictionaryFromFile(path)
	if err != nil {
		t.Fatalf("NewDictionaryFromFile failed: %v", err)
	}
	vals := d.ValuesForHeader("/any", "X-Test")
	if len(vals) != 1 || vals[0] != "val" {
		t.Errorf("unexpected values: %v", vals)
	}
}

func TestDictionary_NonExistentHeader(t *testing.T) {
	d := NewDictionary()
	vals := d.ValuesForHeader("/api", "NonExistent")
	if len(vals) != 0 {
		t.Errorf("expected empty for non-existent header, got %v", vals)
	}
}

func TestAllHeaders(t *testing.T) {
	d := NewDictionary()
	d.AddGlobal("x-global-1", []string{"v1"})
	d.AddGlobal("x-global-2", []string{"v2"})
	d.AddForEndpoint("/api/users", "x-ep-only", []string{"v3"})

	t.Run("global-only endpoint", func(t *testing.T) {
		headers := d.AllHeaders("/api/other")
		names := stringSet(headers)
		if !names["x-global-1"] || !names["x-global-2"] {
			t.Errorf("expected x-global-1 and x-global-2, got %v", headers)
		}
		if names["x-ep-only"] {
			t.Errorf("x-ep-only should not appear for /api/other, got %v", headers)
		}
	})

	t.Run("per-endpoint merge", func(t *testing.T) {
		headers := d.AllHeaders("/api/users")
		names := stringSet(headers)
		if !names["x-global-1"] || !names["x-global-2"] || !names["x-ep-only"] {
			t.Errorf("expected all three headers, got %v", headers)
		}
	})

	t.Run("empty dict", func(t *testing.T) {
		empty := NewDictionary()
		headers := empty.AllHeaders("/any")
		if len(headers) != 0 {
			t.Errorf("expected empty, got %v", headers)
		}
	})
}

func stringSet(s []string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, v := range s {
		m[v] = true
	}
	return m
}
