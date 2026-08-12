package agentapi

import (
	"context"
	"os"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
)

// AIAgentAPI serves requests about AI agents created inside a workspace.
type AIAgentAPI struct {
	AgentID     uuid.UUID
	WorkspaceID uuid.UUID
	OwnerID     uuid.UUID

	Log slog.Logger
}

// CreateAIAgent registers an AI agent created inside the workspace.
//
// PROOF OF CONCEPT STUB. This does not register anything. It touches the path
// given in the request and returns an empty response.
//
// The workspace, its owner, and the workspace_agent that sent this all come
// from the connection rather than the request. coderd resolved them while
// authenticating, so the request has no need to state them and no way to
// state them more reliably.
//
// The marker is the last of the stub to survive. It works only because the
// acceptance test runs coderd on the same host as the workspace, which a real
// deployment does not. The increment that persists an AI agent identity
// replaces it with a row, and this file loses its dependency on os.
func (a *AIAgentAPI) CreateAIAgent(ctx context.Context, req *agentproto.CreateAIAgentRequest) (*agentproto.CreateAIAgentResponse, error) {
	markerPath := req.GetPocMarkerPath()
	if markerPath == "" {
		return nil, xerrors.New("poc_marker_path is required while this handler is a stub")
	}

	// The identifiers written here are the ones coderd derived, not ones the
	// caller supplied. A test comparing them against the workspace it built is
	// therefore checking attribution and not merely echo.
	marker := "CreateAIAgent\n" +
		"workspace_id=" + a.WorkspaceID.String() + "\n" +
		"agent_id=" + a.AgentID.String() + "\n"
	if err := os.WriteFile(markerPath, []byte(marker), 0o600); err != nil {
		return nil, xerrors.Errorf("write poc marker %q: %w", markerPath, err)
	}

	a.Log.Info(ctx, "poc stub: CreateAIAgent called",
		slog.F("workspace_id", a.WorkspaceID),
		slog.F("agent_id", a.AgentID),
		slog.F("marker_path", markerPath))

	return &agentproto.CreateAIAgentResponse{}, nil
}
