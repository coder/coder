package agentapi

import (
	"context"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"
)

type ChildAgentAPI struct {
	AgentID uuid.UUID

	Database database.Store
}

func (a *ChildAgentAPI) CreateChildAgent(ctx context.Context, req *proto.CreateChildAgentRequest) (*proto.CreateChildAgentResponse, error) {
	agent, err := a.Database.GetWorkspaceAgentByID(ctx, a.AgentID)
	if err != nil {
		return nil, xerrors.Errorf("get agent: %w", err)
	}

	childAgentAuthToken := uuid.New()
	childAgent, err := a.Database.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
		ID:                       uuid.New(),
		CreatedAt:                dbtime.Now(),
		UpdatedAt:                dbtime.Now(),
		ResourceID:               agent.ResourceID,
		Name:                     req.Name,
		AuthToken:                childAgentAuthToken,
		AuthInstanceID:           agent.AuthInstanceID,
		Architecture:             agent.Architecture,
		EnvironmentVariables:     pqtype.NullRawMessage{},
		Directory:                req.Directory,
		OperatingSystem:          agent.OperatingSystem,
		ConnectionTimeoutSeconds: agent.ConnectionTimeoutSeconds,
		TroubleshootingURL:       agent.TroubleshootingURL,
		MOTDFile:                 agent.MOTDFile,
		DisplayApps:              []database.DisplayApp{},
		InstanceMetadata:         pqtype.NullRawMessage{},
		ResourceMetadata:         pqtype.NullRawMessage{},
		DisplayOrder:             agent.DisplayOrder,
	})
	if err != nil {
		return nil, xerrors.Errorf("insert agent: %w", err)
	}

	return &proto.CreateChildAgentResponse{
		Id: []byte(childAgent.ID.String()),
	}, nil
}
