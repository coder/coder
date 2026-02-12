package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/scripts/cdev/catalog"
)

const (
	DefaultAPIPort = "19000"
)

// Server provides an HTTP API for controlling cdev services.
type Server struct {
	catalog *catalog.Catalog
	logger  slog.Logger
	srv     *http.Server
	addr    string
}

// NewServer creates a new API server.
func NewServer(c *catalog.Catalog, logger slog.Logger, addr string) *Server {
	s := &Server{
		catalog: c,
		logger:  logger,
		addr:    addr,
	}

	mux := http.NewServeMux()

	// Service endpoints.
	mux.HandleFunc("GET /api/events", s.handleSSE)
	mux.HandleFunc("GET /api/services", s.handleListServices)
	mux.HandleFunc("GET /api/services/{name}", s.handleGetService)
	mux.HandleFunc("POST /api/services/{name}/restart", s.handleRestartService)
	mux.HandleFunc("POST /api/services/{name}/start", s.handleStartService)
	mux.HandleFunc("POST /api/services/{name}/stop", s.handleStopService)

	// Health endpoint.
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	// Serve embedded static files (web UI).
	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("failed to create sub filesystem: " + err.Error())
	}
	mux.Handle("GET /", http.FileServer(http.FS(staticContent)))

	s.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

// Start begins listening for HTTP requests. This is non-blocking.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return xerrors.Errorf("listen on %s: %w", s.addr, err)
	}

	s.logger.Info(ctx, "API server listening", slog.F("addr", s.addr))

	go func() {
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error(ctx, "API server error", slog.Error(err))
		}
	}()

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// Addr returns the address the server is listening on.
func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"error": message})
}
