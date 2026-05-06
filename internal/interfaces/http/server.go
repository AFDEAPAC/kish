package http

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

// Server wraps net/http.Server with graceful shutdown support.
type Server struct {
	inner *http.Server
}

// NewServer constructs a Server bound to the given address with the provided handler.
func NewServer(host string, port int, handler http.Handler) *Server {
	return &Server{
		inner: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", host, port),
			Handler:      handler,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
}

// Start begins listening on the configured address and blocks until the server
// is stopped. It returns the first non-nil error from ListenAndServe (excluding
// http.ErrServerClosed which is the expected shutdown signal).
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.inner.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.inner.Addr, err)
	}
	log.Printf("[api] listening on http://%s", s.inner.Addr)
	if err := s.inner.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully drains in-flight requests up to the deadline in ctx.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.inner.Shutdown(ctx)
}
