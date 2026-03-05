package util

import (
	"net/http"
	"time"
)

type ServerParams struct {
	Addr              string
	Handler           http.Handler
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

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
