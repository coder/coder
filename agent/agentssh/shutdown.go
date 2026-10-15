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

// trackChannel makes Close wait for a channel whose handler runs outside the
// connection and session tracking. If the server is closing, the channel is
// not registered.
//
//nolint:revive // add is a control flag, matching trackConn and trackSession.
func (s *Server) trackChannel(add bool) (ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if add {
		if s.closing != nil {
			return false
		}
		s.wg.Add(1)
		return true
	}
	s.wg.Done()
	return true
}
