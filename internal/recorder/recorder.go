package recorder

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"sync"
	"time"
)

type TxRecord struct {
	RequestID   string              `json:"request_id"`
	Time        time.Time           `json:"time"`
	Method      string              `json:"method"`
	URL         string              `json:"url"`
	ReqHeaders  map[string][]string `json:"req_headers,omitempty"`
	ReqBody     string              `json:"req_body,omitempty"`
	ReqTrunc    bool                `json:"req_truncated"`
	RespStatus  int                 `json:"resp_status"`
	RespHeaders map[string][]string `json:"resp_headers,omitempty"`
	RespBody    string              `json:"resp_body,omitempty"`
	RespTrunc   bool                `json:"resp_truncated"`
	Timings     map[string]int64    `json:"timings_ms,omitempty"`
}

type Recorder interface {
	Record(tx *TxRecord) error
	Close() error
}

type jsonl struct {
	f     *os.File
	mu    sync.Mutex
	fsync bool
}

func NewJSONL(path string) (Recorder, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &jsonl{f: f, fsync: false}, nil
}

func (j *jsonl) Record(tx *TxRecord) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	b, err := json.Marshal(tx)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	if _, err := j.f.Write(b); err != nil {
		return err
	}
	if j.fsync {
		if err := j.f.Sync(); err != nil {
			return err
		}
	}
	return nil
}

func (j *jsonl) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.f == nil {
		return nil
	}
	err := j.f.Close()
	j.f = nil
	return err
}

// кодировка с лимитом
func EncodeBodyToBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
