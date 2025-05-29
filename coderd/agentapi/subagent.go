package agentapi

import (
	"context"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/provisioner"
	"github.com/coder/quartz"
)

type SubAgentAPI struct {
	OwnerID        uuid.UUID
	OrganizationID uuid.UUID
	AgentID        uuid.UUID
	AgentFn        func(context.Context) (database.WorkspaceAgent, error)

	Log      slog.Logger
	Clock    quartz.Clock
	Database database.Store
}

func (a *SubAgentAPI) CreateSubAgent(ctx context.Context, req *agentproto.CreateSubAgentRequest) (*agentproto.CreateSubAgentResponse, error) {
	//nolint:gocritic // This gives us only the permissions required to do the job.
	ctx = dbauthz.AsSubAgentAPI(ctx, a.OrganizationID, a.OwnerID)

	parentAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get parent agent: %w", err)
	}

	agentName := req.Name
	if agentName == "" {
		return nil, xerrors.Errorf("agent name cannot be empty")
	}
	if !provisioner.AgentNameRegex.MatchString(agentName) {
		return nil, xerrors.Errorf("agent name %q does not match regex %q", agentName, provisioner.AgentNameRegex.String())
	}

	createdAt := a.Clock.Now()

	subAgent, err := a.Database.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
		ID:                       uuid.New(),
		ParentID:                 uuid.NullUUID{Valid: true, UUID: parentAgent.ID},
		CreatedAt:                createdAt,
		UpdatedAt:                createdAt,
		Name:                     agentName,
		ResourceID:               parentAgent.ResourceID,
		AuthToken:                uuid.New(),
		AuthInstanceID:           parentAgent.AuthInstanceID,
		Architecture:             req.Architecture,
		EnvironmentVariables:     pqtype.NullRawMessage{},
		OperatingSystem:          req.OperatingSystem,
		Directory:                req.Directory,
		InstanceMetadata:         pqtype.NullRawMessage{},
		ResourceMetadata:         pqtype.NullRawMessage{},
		ConnectionTimeoutSeconds: parentAgent.ConnectionTimeoutSeconds,
		TroubleshootingURL:       parentAgent.TroubleshootingURL,
		MOTDFile:                 "",
		DisplayApps:              []database.DisplayApp{},
		DisplayOrder:             0,
		APIKeyScope:              parentAgent.APIKeyScope,
	})
	if err != nil {
		return nil, xerrors.Errorf("insert sub agent: %w", err)
	}

	return &agentproto.CreateSubAgentResponse{
		Agent: &agentproto.SubAgent{
			Name:      subAgent.Name,
			Id:        subAgent.ID[:],
			AuthToken: subAgent.AuthToken[:],
		},
	}, nil
}

func (a *SubAgentAPI) DeleteSubAgent(ctx context.Context, req *agentproto.DeleteSubAgentRequest) (*agentproto.DeleteSubAgentResponse, error) {
	//nolint:gocritic // This gives us only the permissions required to do the job.
	ctx = dbauthz.AsSubAgentAPI(ctx, a.OrganizationID, a.OwnerID)

	subAgentID, err := uuid.FromBytes(req.Id)
	if err != nil {
		return nil, err
	}

	if err := a.Database.DeleteWorkspaceSubAgentByID(ctx, subAgentID); err != nil {
		return nil, err
	}

	return &agentproto.DeleteSubAgentResponse{}, nil
}

func (a *SubAgentAPI) ListSubAgents(ctx context.Context, _ *agentproto.ListSubAgentsRequest) (*agentproto.ListSubAgentsResponse, error) {
	//nolint:gocritic // This gives us only the permissions required to do the job.
	ctx = dbauthz.AsSubAgentAPI(ctx, a.OrganizationID, a.OwnerID)

	workspaceAgents, err := a.Database.GetWorkspaceAgentsByParentID(ctx, a.AgentID)
	if err != nil {
		return nil, err
	}

	agents := make([]*agentproto.SubAgent, len(workspaceAgents))

	for i, agent := range workspaceAgents {
		agents[i] = &agentproto.SubAgent{
			Name:      agent.Name,
			Id:        agent.ID[:],
			AuthToken: agent.AuthToken[:],
		}
	}

	return &agentproto.ListSubAgentsResponse{Agents: agents}, nil
}
