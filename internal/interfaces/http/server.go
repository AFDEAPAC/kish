package http

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

// Server wraps net/http.Server with graceful shutdown support and the
// fixed request-handling timeouts the API enforces on every connection.
type Server struct {
	inner *http.Server
}

// NewServer builds the API HTTP server bound to host:port.
//
// Timeouts are not configurable on purpose: they encode policy rather than
// preference. ReadTimeout caps how long a slowloris-style client can hold
// open a request; WriteTimeout bounds artifact downloads (currently 60s,
// the same horizon the S3 store uses for its outbound calls); IdleTimeout
// keeps idle keep-alive connections from accumulating. Adjusting these
// values affects upload size limits and storage backend timing
// assumptions, so changes must be coordinated with the artifact service
// and the storage backends.
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
