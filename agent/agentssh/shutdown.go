package agentssh

import (
	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// serverShutdownFields describes a connection closed by the SSH server.
func serverShutdownFields() []slog.Field {
	return []slog.Field{
		codersdk.DisconnectReasonServerShutdown.SlogField(),
		codersdk.DisconnectReasonServerShutdown.SlogExpectedField(),
		codersdk.DisconnectInitiatorAgent.SlogField(),
	}
}

// isClosing reports whether Close is in progress. Close keeps it set until
// every tracked connection, session, and channel has reported its close.
func (s *Server) isClosing() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closing != nil
}
