package agentapi

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/entity"
)

// AIAgentAPI serves requests about AI agents created inside a workspace.
type AIAgentAPI struct {
	AgentID     uuid.UUID
	WorkspaceID uuid.UUID
	OwnerID     uuid.UUID

	Database database.Store
	Log      slog.Logger
}

// CreateAIAgent registers an AI agent created inside the workspace and returns
// the identity minted for it.
//
// The request carries nothing. The workspace, its owner, and the
// workspace_agent that sent this all come from the connection: coderd resolved
// them while authenticating, so the request has no need to state them and no
// way to state them more reliably.
//
// The entry names the workspace_agent as the actor, since it is the party that
// made the request and the only one authenticated on this connection. Telling
// apart the party that asked from the party that relayed needs data these
// calls do not yet carry.
func (a *AIAgentAPI) CreateAIAgent(ctx context.Context, _ *agentproto.CreateAIAgentRequest) (*agentproto.CreateAIAgentResponse, error) {
	// Appending to the journal requires system permission, which this
	// connection's subject does not hold: it is scoped to one workspace. That
	// is deliberate. An entity cannot write entries, including about itself,
	// so the control plane writes them on its behalf and names it as actor.
	//nolint:gocritic // Writing the journal is the control plane's act, not the agent's.
	systemCtx := dbauthz.AsSystemRestricted(ctx)

	id, err := entity.CreateAIAgent(systemCtx, a.Database, entity.CreateAIAgentParams{
		OwnerID: a.OwnerID,
		Actor:   entity.Ref{Type: entity.TypeWorkspaceAgent, ID: a.AgentID},
	})
	if err != nil {
		return nil, xerrors.Errorf("create AI agent: %w", err)
	}

	a.Log.Debug(ctx, "created AI agent",
		slog.F("ai_agent_id", id),
		slog.F("workspace_id", a.WorkspaceID),
		slog.F("agent_id", a.AgentID))

	return &agentproto.CreateAIAgentResponse{Id: id[:]}, nil
}
