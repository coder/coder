package agentapi

import (
	"context"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/google/uuid"
)

type ScriptsAPI struct {
	AgentID  uuid.UUID
	Database database.Store
}

func (s *ScriptsAPI) ScriptCompleted(ctx context.Context, req *agentproto.WorkspaceAgentScriptCompletedRequest) (*agentproto.WorkspaceAgentScriptCompletedResponse, error) {
	res := &agentproto.WorkspaceAgentScriptCompletedResponse{}

	agent, err := s.Database.GetWorkspaceAgentByID(ctx, s.AgentID)
	if err != nil {
		return nil, err
	}

	resource, err := s.Database.GetWorkspaceResourceByID(ctx, agent.ResourceID)
	if err != nil {
		return nil, err
	}

	_, err = s.Database.InsertWorkspaceAgentScriptTimings(ctx, database.InsertWorkspaceAgentScriptTimingsParams{
		JobID:       resource.JobID,
		DisplayName: req.Timing.DisplayName,
		StartedAt:   req.Timing.Start.AsTime(),
		EndedAt:     req.Timing.End.AsTime(),
		ExitCode:    req.Timing.ExitCode,
	})
	if err != nil {
		return nil, err
	}

	return res, nil
}
