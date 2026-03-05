package mitm

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"ffuuzz/internal/recorder"
	"ffuuzz/internal/store"
	"ffuuzz/internal/util"
)

type Config struct {
	ListenAddr   string
	CertStore    *store.CertStore
	Recorder     recorder.Recorder
	MaxBodyBytes int
}

type Proxy struct {
	cfg       Config
	transport *http.Transport
}

func New(cfg Config) *Proxy {
	tr := &http.Transport{
		Proxy:               nil,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
	}
	return &Proxy{cfg: cfg, transport: tr}
}

// запуск http-сервера и обработка запросов
func (p *Proxy) ListenAndServe() error {
	srv := util.NewHTTPServer(util.ServerParams{
		Addr:              p.cfg.ListenAddr,
		Handler:           p,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	})
	return srv.ListenAndServe()
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleCONNECT(w, r)
		return
	}
	p.handleHTTP(w, r)
}

// обработка http-запроса
func (p *Proxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	outReq := r.Clone(context.Background())
	outReq.RequestURI = ""

	if outReq.URL.Scheme == "" {
		outReq.URL.Scheme = "http"
	}
	if outReq.URL.Host == "" {
		outReq.URL.Host = outReq.Host
	}

	util.RemoveHopByHop(outReq.Header)

	reqBuf := util.NewLimitedBuffer(p.cfg.MaxBodyBytes)
	if r.Body != nil {
		outReq.Body = util.NewTeeReadCloser(r.Body, reqBuf)
	}

	resp, err := p.transport.RoundTrip(outReq)
	if err != nil {
		log.Printf("upstream error for %s: %v", outReq.URL.String(), err)
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	util.RemoveHopByHop(resp.Header)
	util.CopyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	respBuf := util.NewLimitedBuffer(p.cfg.MaxBodyBytes)
	mw := io.MultiWriter(w, respBuf)

	if _, copyErr := io.Copy(mw, resp.Body); copyErr != nil {
		log.Printf("error copying response body: %v", copyErr)
	}

	tx := &recorder.TxRecord{
		RequestID:   genReqID(),
		Time:        start,
		Method:      r.Method,
		URL:         outReq.URL.String(),
		ReqHeaders:  outReq.Header.Clone(),
		RespStatus:  resp.StatusCode,
		RespHeaders: resp.Header.Clone(),
		ReqTrunc:    reqBuf.Truncated(),
		RespTrunc:   respBuf.Truncated(),
		Timings:     map[string]int64{"total_ms": time.Since(start).Milliseconds()},
	}

	if rb := reqBuf.Bytes(); len(rb) > 0 {
		tx.ReqBody = recorder.EncodeBodyToBase64(rb)
	}
	if rb := respBuf.Bytes(); len(rb) > 0 {
		tx.RespBody = recorder.EncodeBodyToBase64(rb)
	}

	if err := p.cfg.Recorder.Record(tx); err != nil {
		log.Printf("recorder error: %v", err)
	}
}

type singleConnListener struct {
	conn net.Conn
	done chan struct{}
}

func newSingleConnListener(c net.Conn) *singleConnListener {
	return &singleConnListener{
		conn: c,
		done: make(chan struct{}),
	}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.conn == nil {
		<-l.done
		return nil, io.EOF
	}
	c := l.conn
	l.conn = nil
	return c, nil
}

func (l *singleConnListener) Close() error {
	close(l.done)
	return nil
}
func (l *singleConnListener) Addr() net.Addr {
	if l.conn != nil {
		return l.conn.LocalAddr()
	}
	return &net.TCPAddr{}
}

func (p *Proxy) handleCONNECT(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hj.Hijack()
	if err != nil {
		return
	}

	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}

	go p.mitmHTTPS(clientConn, host)
}

func (p *Proxy) mitmHTTPS(clientConn net.Conn, host string) {
	defer clientConn.Close()

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))

	leaf, err := p.cfg.CertStore.GetCertFor(host)
	if err != nil {
		log.Printf("GetCertFor(%s): %v", host, err)
		return
	}

	tlsConn := tls.Server(clientConn, &tls.Config{
		Certificates: []tls.Certificate{leaf},
	})

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
		log.Printf("mitm http serve error: %v", err)
	}
}

func genReqID() string {
	return time.Now().Format("20060102T150405.000000000")
}
