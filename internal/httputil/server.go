package httputil

import (
	"net/http"
	"time"
)

// ServerParams holds the configuration for constructing an HTTP server.
type ServerParams struct {
	Addr              string
	Handler           http.Handler
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// NewHTTPServer creates an *http.Server from the given parameters.
func NewHTTPServer(p ServerParams) *http.Server {
	return &http.Server{
		Addr:              p.Addr,
		Handler:           p.Handler,
		ReadTimeout:       p.ReadTimeout,
		ReadHeaderTimeout: p.ReadHeaderTimeout,
		WriteTimeout:      p.WriteTimeout,
		IdleTimeout:       p.IdleTimeout,
	}
}
