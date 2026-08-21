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
// the identity minted for it along with the credential issued to it.
//
// The request carries nothing. The workspace, its owner, and the
// workspace_agent that sent this all come from the connection: coderd resolved
// them while authenticating, so the request has no need to state them and no
// way to state them more reliably.
//
// The entry names the owner as the actor. Creation is commanded, and what
// commands it is the order that brought the AI agent about; a workspace_agent
// relaying that order creates nothing. The owner is as reliably known as the
// relay is, both being resolved from the same authenticated connection.
func (a *AIAgentAPI) CreateAIAgent(ctx context.Context, _ *agentproto.CreateAIAgentRequest) (*agentproto.CreateAIAgentResponse, error) {
	// Appending to the journal requires system permission, which this
	// connection's subject does not hold: it is scoped to one workspace. That
	// is deliberate. An entity cannot write entries, including about itself,
	// so the control plane writes them on its behalf and names it as actor.
	//nolint:gocritic // Writing the journal is the control plane's act, not the agent's.
	systemCtx := dbauthz.AsSystemRestricted(ctx)

	created, err := entity.CreateAIAgent(systemCtx, a.Database, entity.CreateAIAgentParams{
		Owner: entity.Ref{Type: entity.TypeUser, ID: a.OwnerID},
	})
	if err != nil {
		return nil, xerrors.Errorf("create AI agent: %w", err)
	}

	// The credential is absent from the log line, as credentials are from logs.
	a.Log.Debug(ctx, "created AI agent",
		slog.F("ai_agent_id", created.ID),
		slog.F("workspace_id", a.WorkspaceID),
		slog.F("ws_agent_id", a.AgentID))

	return &agentproto.CreateAIAgentResponse{
		Id:         created.ID[:],
		Credential: []byte(created.Authenticator),
	}, nil
}
