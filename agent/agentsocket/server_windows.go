//go:build windows

package agentsocket

import (
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// Server provides access to the DRPCAgentSocketService via a Unix domain socket.
// Do not invoke Server{} directly. Use NewServer() instead.
type Server struct{}

// NewServer returns an error indicating that agentsocket is not supported on Windows.
func NewServer(_ slog.Logger, _ ...Option) (*Server, error) {
	return &Server{}, xerrors.New("agentsocket is not supported on Windows")
}

// Close stops the server and cleans up resources.
func (s *Server) Close() error {
	return nil
}
