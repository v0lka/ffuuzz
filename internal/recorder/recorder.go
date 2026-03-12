// Package recorder captures HTTP exchanges to JSONL files for later replay.
package recorder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/endpoint"
	"ffuuzz/internal/metrics"
	"ffuuzz/internal/model"
)

// TxRecord is the JSONL representation of a single recorded HTTP transaction.
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

func EncodeBodyToBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// TxRecordToExchange converts a TxRecord to a model.Exchange.
func TxRecordToExchange(tx TxRecord) model.Exchange {
	u, _ := url.Parse(tx.URL)
	path := ""
	query := ""
	if u != nil {
		path = u.Path
		query = u.RawQuery
	}

	durationMs := int64(0)
	if ms, ok := tx.Timings["total_ms"]; ok {
		durationMs = ms
	}

	return model.Exchange{
		RequestID:  tx.RequestID,
		StartedAt:  tx.Time,
		DurationMs: durationMs,
		Request: model.RequestData{
			Method:        tx.Method,
			Path:          path,
			Query:         query,
			Headers:       tx.ReqHeaders,
			BodyB64:       tx.ReqBody,
			BodyTruncated: tx.ReqTrunc,
		},
		Response: model.ResponseData{
			Status:        tx.RespStatus,
			Headers:       tx.RespHeaders,
			BodyB64:       tx.RespBody,
			BodyTruncated: tx.RespTrunc,
		},
	}
}

// ExchangeToTxRecord converts a model.Exchange back to a TxRecord.
func ExchangeToTxRecord(ex model.Exchange, baseURL string) TxRecord {
	fullURL := baseURL + ex.Request.Path
	if ex.Request.Query != "" {
		fullURL += "?" + ex.Request.Query
	}
	return TxRecord{
		RequestID:   ex.RequestID,
		Time:        ex.StartedAt,
		Method:      ex.Request.Method,
		URL:         fullURL,
		ReqHeaders:  ex.Request.Headers,
		ReqBody:     ex.Request.BodyB64,
		ReqTrunc:    ex.Request.BodyTruncated,
		RespStatus:  ex.Response.Status,
		RespHeaders: ex.Response.Headers,
		RespBody:    ex.Response.BodyB64,
		RespTrunc:   ex.Response.BodyTruncated,
		Timings:     map[string]int64{"total_ms": ex.DurationMs},
	}
}

// RecordingInserter defines the interface for inserting recording sessions.
type RecordingInserter interface {
	FindOrAppend(ctx context.Context, sess model.RecordingSession) (string, bool, error)
}

// DBRecorder implements Recorder by grouping transactions by endpoint
// (scheme+host+port+path) and appending exchanges to existing recordings.
// When a Resolver is provided, paths are normalised before storage and
// the resolver tracks cardinality for statistical collapse detection.
type DBRecorder struct {
	store    RecordingInserter
	resolver *endpoint.Resolver
	logger   zerolog.Logger
	mu       sync.Mutex
}

// NewDBRecorder creates a recorder that writes directly to the database.
// Transactions to the same endpoint are grouped into a single RecordingSession.
// If resolver is non-nil, paths are heuristically normalised and fed to the
// resolver for statistical collapse detection.
func NewDBRecorder(store RecordingInserter, resolver *endpoint.Resolver, logger zerolog.Logger) *DBRecorder {
	return &DBRecorder{
		store:    store,
		resolver: resolver,
		logger:   logger,
	}
}

func (d *DBRecorder) Record(tx *TxRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Parse URL to extract target info
	u, err := url.Parse(tx.URL)
	if err != nil {
		d.logger.Warn().Err(err).Str("url", tx.URL).Msg("failed to parse URL for recording")
		return err
	}

	// Extract scheme, host, port, path
	scheme := u.Scheme
	if scheme == "" {
		scheme = "http"
	}
	host := u.Hostname()
	if host == "" {
		host = "unknown"
	}
	port := 80
	if u.Port() != "" {
		port, _ = strconv.Atoi(u.Port())
	} else if scheme == "https" {
		port = 443
	}
	path := u.Path

	// Phase 1: heuristic normalisation (regex-based, immediate).
	path = endpoint.NormalizePath(path)

	origin := endpoint.Origin{Scheme: scheme, Host: host, Port: port}

	// Phase 2: statistical normalisation (trie-based, may trigger async merge).
	if d.resolver != nil {
		path = d.resolver.ObservePath(origin, path)
	}

	// Create session — ID is not set because FindOrAppend manages IDs
	sess := model.RecordingSession{
		SchemaVersion: 1,
		CreatedAt:     tx.Time,
		Target: model.TargetInfo{
			Scheme: scheme,
			Host:   host,
			Port:   port,
			Path:   path,
		},
		Entries:    []model.Exchange{TxRecordToExchange(*tx)},
		EntryCount: 1,
	}

	_, created, err := d.store.FindOrAppend(context.Background(), sess)
	if err != nil {
		d.logger.Error().Err(err).Str("request_id", tx.RequestID).Msg("failed to record exchange to database")
		return err
	}
	if created {
		metrics.CorpusSize.Inc()
	}

	return nil
}

func (d *DBRecorder) Close() error {
	return nil // No resources to close
}
