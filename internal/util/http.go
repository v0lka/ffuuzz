package util

import (
	"bytes"
	"io"
	"net/http"
)

var hopByHopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func RemoveHopByHop(h http.Header) {
	for _, k := range hopByHopHeaders {
		h.Del(k)
	}
}

func CopyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

type LimitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func NewLimitedBuffer(limit int) *LimitedBuffer {
	return &LimitedBuffer{limit: limit}
}

func (b *LimitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return b.buf.Write(p)
	}
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			_, _ = b.buf.Write(p)
		} else {
			_, _ = b.buf.Write(p[:remaining])
			b.truncated = true
		}
	} else {
		b.truncated = true
	}
	return len(p), nil
}

func (b *LimitedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

func (b *LimitedBuffer) Truncated() bool {
	return b.truncated
}

type TeeReadCloser struct {
	r io.ReadCloser
	w io.Writer
}

func NewTeeReadCloser(r io.ReadCloser, w io.Writer) io.ReadCloser {
	if r == nil || w == nil {
		return r
	}
	return &TeeReadCloser{r: r, w: w}
}

func (t *TeeReadCloser) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		_, _ = t.w.Write(p[:n])
	}
	return n, err
}

func (t *TeeReadCloser) Close() error {
	return t.r.Close()
}
