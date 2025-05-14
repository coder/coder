package agentapi

import (
	"context"
	"strings"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/quartz"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/emptypb"
)

type DevContainerAgentAPI struct {
	AgentID uuid.UUID
	AgentFn func(context.Context) (database.WorkspaceAgent, error)

	Log      slog.Logger
	Clock    quartz.Clock
	Database database.Store
}

func (a *DevContainerAgentAPI) CreateDevContainerAgent(ctx context.Context, req *agentproto.CreateDevContainerAgentRequest) (*agentproto.CreateDevContainerAgentResponse, error) {
	ctx = dbauthz.AsDevContainerAgentAPI(ctx)

	parentAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get parent agent: %w", err)
	}

	// TODO(DanielleMaywood):
	// We need the following
	// - Architecture
	// - Operating System

	// TODO(DanielleMaywood):
	// Validate this agent name
	agentName := strings.ToLower(req.Name)

	devContainerAgent, err := a.Database.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
		ID:                       uuid.New(),
		ParentID:                 uuid.NullUUID{Valid: true, UUID: parentAgent.ID},
		CreatedAt:                a.Clock.Now(),
		UpdatedAt:                a.Clock.Now(),
		Name:                     agentName,
		ResourceID:               parentAgent.ResourceID,
		AuthToken:                uuid.New(),
		AuthInstanceID:           parentAgent.AuthInstanceID,
		Architecture:             "",
		EnvironmentVariables:     pqtype.NullRawMessage{},
		OperatingSystem:          "",
		Directory:                req.Directory,
		InstanceMetadata:         pqtype.NullRawMessage{},
		ResourceMetadata:         pqtype.NullRawMessage{},
		ConnectionTimeoutSeconds: parentAgent.ConnectionTimeoutSeconds,
		TroubleshootingURL:       parentAgent.TroubleshootingURL,
		MOTDFile:                 "",
		DisplayApps:              []database.DisplayApp{},
		DisplayOrder:             0,
	})
	if err != nil {
		return nil, xerrors.Errorf("insert dev container agent: %w", err)
	}

	return &agentproto.CreateDevContainerAgentResponse{
		Id:        devContainerAgent.ID[:],
		AuthToken: devContainerAgent.AuthToken[:],
	}, nil
}

func (a *DevContainerAgentAPI) DeleteDevContainerAgent(ctx context.Context, req *agentproto.DeleteDevContainerAgentRequest) (*emptypb.Empty, error) {
	ctx = dbauthz.AsDevContainerAgentAPI(ctx)

	devContainerAgentID, err := uuid.FromBytes(req.Id)
	if err != nil {
		return nil, err
	}

	if err := a.Database.DeleteWorkspaceAgentByID(ctx, devContainerAgentID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (a *DevContainerAgentAPI) ListDevContainerAgents(ctx context.Context, req *agentproto.ListDevContainerAgentsRequest) (*agentproto.ListDevContainerAgentsResponse, error) {
	ctx = dbauthz.AsDevContainerAgentAPI(ctx)

	workspaceAgents, err := a.Database.GetWorkspaceAgentsWithParentID(ctx, uuid.NullUUID{Valid: true, UUID: a.AgentID})
	if err != nil {
		return nil, err
	}

	agents := make([]*agentproto.ListDevContainerAgentsResponse_DevContainerAgent, len(workspaceAgents))

	for i, agent := range workspaceAgents {
		agents[i] = &agentproto.ListDevContainerAgentsResponse_DevContainerAgent{
			Name: agent.Name,
			Id:   agent.ID[:],
		}
	}

	return &agentproto.ListDevContainerAgentsResponse{Agents: agents}, nil
}
