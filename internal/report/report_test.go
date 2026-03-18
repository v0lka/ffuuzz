package report

import (
	"testing"

	"ffuuzz/internal/recorder"
)

func TestBuildSummary_Empty(t *testing.T) {
	s := BuildSummary(nil)
	if s.Total != 0 {
		t.Errorf("Total = %d, want 0", s.Total)
	}
	if len(s.ByMethod) != 0 {
		t.Error("expected empty ByMethod")
	}
	if len(s.ByStatus) != 0 {
		t.Error("expected empty ByStatus")
	}
	if len(s.ByHost) != 0 {
		t.Error("expected empty ByHost")
	}
}

func TestBuildSummary_SingleRecord(t *testing.T) {
	records := []recorder.TxRecord{
		{Method: "GET", URL: "http://example.com/api", RespStatus: 200},
	}
	s := BuildSummary(records)
	if s.Total != 1 {
		t.Errorf("Total = %d, want 1", s.Total)
	}
	if s.ByMethod["GET"] != 1 {
		t.Errorf("ByMethod[GET] = %d, want 1", s.ByMethod["GET"])
	}
	if s.ByStatus[200] != 1 {
		t.Errorf("ByStatus[200] = %d, want 1", s.ByStatus[200])
	}
	if s.ByHost["example.com"] != 1 {
		t.Errorf("ByHost[example.com] = %d, want 1", s.ByHost["example.com"])
	}
}

func TestBuildSummary_MultipleRecords(t *testing.T) {
	records := []recorder.TxRecord{
		{Method: "GET", URL: "http://a.com/x", RespStatus: 200},
		{Method: "POST", URL: "http://a.com/y", RespStatus: 201},
		{Method: "GET", URL: "http://b.com/z", RespStatus: 500},
		{Method: "GET", URL: "http://a.com/x", RespStatus: 200},
	}
	s := BuildSummary(records)
	if s.Total != 4 {
		t.Errorf("Total = %d, want 4", s.Total)
	}
	if s.ByMethod["GET"] != 3 {
		t.Errorf("ByMethod[GET] = %d, want 3", s.ByMethod["GET"])
	}
	if s.ByMethod["POST"] != 1 {
		t.Errorf("ByMethod[POST] = %d, want 1", s.ByMethod["POST"])
	}
	if s.ByStatus[200] != 2 {
		t.Errorf("ByStatus[200] = %d, want 2", s.ByStatus[200])
	}
	if s.ByStatus[201] != 1 {
		t.Errorf("ByStatus[201] = %d, want 1", s.ByStatus[201])
	}
	if s.ByStatus[500] != 1 {
		t.Errorf("ByStatus[500] = %d, want 1", s.ByStatus[500])
	}
	if s.ByHost["a.com"] != 3 {
		t.Errorf("ByHost[a.com] = %d, want 3", s.ByHost["a.com"])
	}
	if s.ByHost["b.com"] != 1 {
		t.Errorf("ByHost[b.com] = %d, want 1", s.ByHost["b.com"])
	}
}

func TestBuildSummary_InvalidURL(t *testing.T) {
	records := []recorder.TxRecord{
		{Method: "GET", URL: "://bad-url", RespStatus: 200},
	}
	s := BuildSummary(records)
	if s.Total != 1 {
		t.Errorf("Total = %d, want 1", s.Total)
	}
	if len(s.ByHost) != 0 {
		t.Error("expected empty ByHost for invalid URL")
	}
}
