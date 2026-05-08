package httpx

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// ServerOptions configures Server.
type ServerOptions struct {
	Addr            string
	Handler         http.Handler
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// Server wraps http.Server with graceful shutdown helpers.
type Server struct {
	http     *http.Server
	listener net.Listener
	addr     string
	shutdown time.Duration
}

// NewServer constructs a Server with sensible defaults.
func NewServer(opts ServerOptions) *Server {
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 10 * time.Second
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = 30 * time.Second
	}
	if opts.IdleTimeout == 0 {
		opts.IdleTimeout = 90 * time.Second
	}
	if opts.ShutdownTimeout == 0 {
		opts.ShutdownTimeout = 30 * time.Second
	}

	return &Server{
		http: &http.Server{
			Addr:         opts.Addr,
			Handler:      opts.Handler,
			ReadTimeout:  opts.ReadTimeout,
			WriteTimeout: opts.WriteTimeout,
			IdleTimeout:  opts.IdleTimeout,
		},
		shutdown: opts.ShutdownTimeout,
	}
}

// Start binds the listener and serves HTTP. Returns nil on graceful shutdown.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.addr = ln.Addr().String()

	if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Addr returns the actual bound address (resolves :0 ports).
func (s *Server) Addr() string { return s.addr }

// Shutdown gracefully drains in-flight requests within the configured timeout.
func (s *Server) Shutdown(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, s.shutdown)
	defer cancel()
	return s.http.Shutdown(ctx)
}
