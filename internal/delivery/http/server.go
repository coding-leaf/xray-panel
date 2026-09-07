package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"panel/internal/app"
)

var _ app.Service = (*Server)(nil)

// Server wraps http.Server to implement the app.Service interface,
// supporting graceful shutdown and active Keep-Alive connection draining.
type Server struct {
	httpServer      *http.Server
	shutdownTimeout time.Duration
}

// ServerOption configures the Server.
type ServerOption func(*Server)

// WithShutdownTimeout sets the maximum duration to wait for Keep-Alive
// and active requests to complete during shutdown.
func WithShutdownTimeout(timeout time.Duration) ServerOption {
	return func(s *Server) {
		if timeout > 0 {
			s.shutdownTimeout = timeout
		}
	}
}

// NewServer creates a new HTTP server service.
func NewServer(port string, handler http.Handler, opts ...ServerOption) *Server {
	addr := fmt.Sprintf(":%s", port)
	s := &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: handler,
		},
		shutdownTimeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NewServerWithHTTPServer wraps an existing *http.Server instance.
func NewServerWithHTTPServer(srv *http.Server, opts ...ServerOption) *Server {
	s := &Server{
		httpServer:      srv,
		shutdownTimeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Addr returns the configured address of the HTTP server.
func (s *Server) Addr() string {
	if s.httpServer != nil {
		return s.httpServer.Addr
	}
	return ""
}

// Start runs the HTTP server and blocks until ctx is canceled or a fatal
// server error occurs. Upon ctx.Done(), it triggers a graceful Shutdown
// to drain Keep-Alive and active connections.
func (s *Server) Start(ctx context.Context) error {
	if s.httpServer == nil {
		return errors.New("http server is not initialized")
	}

	errCh := make(chan error, 1)

	go func() {
		slog.Info("Panel HTTP server listening", slog.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server failed to listen or serve", slog.String("error", err.Error()))
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		// ListenAndServe exited prematurely before context was canceled
		if err == nil {
			return errors.New("http server stopped unexpectedly")
		}
		return err
	case <-ctx.Done():
		slog.Info("Shutting down HTTP server gracefully (draining keep-alive connections)...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()

		var shutdownErr error
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server graceful shutdown error, forcing connection close", slog.String("error", err.Error()))
			_ = s.httpServer.Close()
			shutdownErr = fmt.Errorf("http server shutdown: %w", err)
		} else {
			slog.Info("HTTP server stopped cleanly")
		}

		// Await ListenAndServe goroutine exit to prevent leaks
		<-errCh
		return shutdownErr
	}
}
