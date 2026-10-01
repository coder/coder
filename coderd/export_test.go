package coderd

import (
	"net/http"

	"github.com/google/uuid"
)

// ChatStartWorkspace exposes chatStartWorkspace for external tests.
//
// chatStartWorkspace is intentionally unexported to keep symmetry with
// its sister chatCreateWorkspace. The alias lets external tests drive
// the RequireActiveVersion auto-update path end-to-end without
// stubbing the entire DB layer. The proper fix is to extract a pure
// request builder; tracked in CODAGT-292.
var ChatStartWorkspace = (*API).chatStartWorkspace

// ChatStopWorkspace exposes chatStopWorkspace for external tests.
var ChatStopWorkspace = (*API).chatStopWorkspace

// NormalizeWorkspaceFileReference exposes normalizeWorkspaceFileReference for tests.
var NormalizeWorkspaceFileReference = normalizeWorkspaceFileReference

func (s *ServerTailnet) AgentTicketCount(agentID uuid.UUID) int {
	s.coordCtrl.mu.Lock()
	defer s.coordCtrl.mu.Unlock()
	return len(s.coordCtrl.tickets[agentID])
}

func (s *ServerTailnet) AgentAPITransport(agentID uuid.UUID) http.RoundTripper {
	return s.transportsFor(agentID).api
}
