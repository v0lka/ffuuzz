// Package mitm implements the MITM proxy that intercepts and records HTTPS traffic.
package mitm

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/httputil"
	"ffuuzz/internal/metrics"
	"ffuuzz/internal/recorder"
	"ffuuzz/internal/store"
)

// Config holds the MITM proxy configuration.
type Config struct {
	ListenAddr    string
	CertStore     *store.CertStore
	Recorder      recorder.Recorder
	MaxBodyBytes  int
	TLSSkipVerify bool
	Logger        zerolog.Logger
}

// Proxy is the MITM HTTP/HTTPS proxy that intercepts traffic for recording.
type Proxy struct {
	cfg       Config
	transport *http.Transport
	server    *http.Server
	logger    zerolog.Logger
}

// New creates a MITM Proxy with the given configuration.
func New(cfg Config) *Proxy {
	tr := &http.Transport{
		Proxy:               nil,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.TLSSkipVerify,
		},
	}
	return &Proxy{cfg: cfg, transport: tr, logger: cfg.Logger}
}

// ListenAndServe starts the proxy and returns the underlying *http.Server
// so the caller can call Shutdown for graceful stop.
func (p *Proxy) ListenAndServe() (*http.Server, error) {
	p.server = httputil.NewHTTPServer(httputil.ServerParams{
		Addr:              p.cfg.ListenAddr,
		Handler:           p,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	})
	return p.server, p.server.ListenAndServe()
}

// Shutdown gracefully shuts down the proxy server.
func (p *Proxy) Shutdown(ctx context.Context) error {
	if p.server != nil {
		return p.server.Shutdown(ctx)
	}
	return nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleCONNECT(w, r)
		return
	}
	p.handleHTTP(w, r)
}

func (p *Proxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := httputil.NewRequestID()

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""

	if outReq.URL.Scheme == "" {
		outReq.URL.Scheme = "http"
	}
	if outReq.URL.Host == "" {
		outReq.URL.Host = outReq.Host
	}

	httputil.RemoveHopByHop(outReq.Header)

	reqBuf := httputil.NewLimitedBuffer(p.cfg.MaxBodyBytes)
	if r.Body != nil {
		outReq.Body = httputil.NewTeeReadCloser(r.Body, reqBuf)
	}

	resp, err := p.transport.RoundTrip(outReq)
	if err != nil {
		p.logger.Error().Err(err).Str("request_id", reqID).Str("url", outReq.URL.String()).Msg("upstream error")
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	httputil.RemoveHopByHop(resp.Header)
	httputil.CopyHeaders(w.Header(), resp.Header)
	w.Header().Set("X-Request-ID", reqID)
	w.WriteHeader(resp.StatusCode)

	respBuf := httputil.NewLimitedBuffer(p.cfg.MaxBodyBytes)
	mw := io.MultiWriter(w, respBuf)

	if _, copyErr := io.Copy(mw, resp.Body); copyErr != nil {
		p.logger.Error().Err(copyErr).Str("request_id", reqID).Msg("error copying response body")
	}

	elapsed := time.Since(start)
	metrics.RequestDuration.Observe(elapsed.Seconds())

	tx := &recorder.TxRecord{
		RequestID:   reqID,
		Time:        start,
		Method:      r.Method,
		URL:         outReq.URL.String(),
		ReqHeaders:  outReq.Header.Clone(),
		RespStatus:  resp.StatusCode,
		RespHeaders: resp.Header.Clone(),
		ReqTrunc:    reqBuf.Truncated(),
		RespTrunc:   respBuf.Truncated(),
		Timings:     map[string]int64{"total_ms": elapsed.Milliseconds()},
	}

	if rb := reqBuf.Bytes(); len(rb) > 0 {
		tx.ReqBody = recorder.EncodeBodyToBase64(rb)
	}
	if rb := respBuf.Bytes(); len(rb) > 0 {
		tx.RespBody = recorder.EncodeBodyToBase64(rb)
	}

	if err := p.cfg.Recorder.Record(tx); err != nil {
		p.logger.Error().Err(err).Str("request_id", reqID).Msg("recorder error")
	}
}

type singleConnListener struct {
	conn   net.Conn
	once   sync.Once
	connCh chan net.Conn
	done   chan struct{}
}

func newSingleConnListener(c net.Conn) *singleConnListener {
	ch := make(chan net.Conn, 1)
	ch <- c
	return &singleConnListener{
		conn:   c,
		connCh: ch,
		done:   make(chan struct{}),
	}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.connCh:
		return c, nil
	case <-l.done:
		return nil, io.EOF
	}
}

func (l *singleConnListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

func (p *Proxy) handleCONNECT(w http.ResponseWriter, r *http.Request) {
	reqID := httputil.NewRequestID()
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		metrics.ConnectErrors.WithLabelValues("hijack_unsupported").Inc()
		p.logger.Error().Str("request_id", reqID).Str("host", r.Host).Msg("hijack not supported")
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hj.Hijack()
	if err != nil {
		metrics.ConnectErrors.WithLabelValues("hijack_failed").Inc()
		p.logger.Error().Err(err).Str("request_id", reqID).Str("host", r.Host).Msg("hijack failed")
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}

	go p.mitmHTTPS(clientConn, host, reqID)
}

func (p *Proxy) mitmHTTPS(clientConn net.Conn, host string, connectReqID string) {
	defer func() { _ = clientConn.Close() }()

	_, err := clientConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	if err != nil {
		metrics.ConnectErrors.WithLabelValues("write_200").Inc()
		p.logger.Error().Err(err).Str("request_id", connectReqID).Str("host", host).Msg("failed to write 200 to client")
		return
	}

	leaf, err := p.cfg.CertStore.GetCertFor(host)
	if err != nil {
		metrics.ConnectErrors.WithLabelValues("cert_generation").Inc()
		p.logger.Error().Err(err).Str("request_id", connectReqID).Str("host", host).Msg("GetCertFor failed")
		return
	}

	// Apply TLS handshake timeout
	if deadline := p.cfg.CertStore.HandshakeTimeout(); deadline > 0 {
		_ = clientConn.SetDeadline(time.Now().Add(deadline))
	}

	tlsCfg := p.cfg.CertStore.TLSConfigForClient(leaf)
	tlsConn := tls.Server(clientConn, tlsCfg)

	if err := tlsConn.Handshake(); err != nil {
		metrics.ConnectErrors.WithLabelValues("tls_handshake").Inc()
		p.logger.Warn().Err(err).Str("request_id", connectReqID).Str("host", host).Msg("TLS handshake failed")
		return
	}

	// Reset deadline after successful handshake
	_ = tlsConn.SetDeadline(time.Time{})

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Scheme == "" {
				r.URL.Scheme = "https"
			}
			if r.URL.Host == "" {
				r.URL.Host = host
			}
			p.handleHTTP(w, r)
		}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ln := newSingleConnListener(tlsConn)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		p.logger.Debug().Err(err).Str("request_id", connectReqID).Str("host", host).Msg("mitm serve ended")
	}
}
