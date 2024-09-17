package agentapi

import (
	"context"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
)

type ScriptsAPI struct {
	Database database.Store
}

func (s *ScriptsAPI) ScriptCompleted(ctx context.Context, req *agentproto.WorkspaceAgentScriptCompletedRequest) (*agentproto.WorkspaceAgentScriptCompletedResponse, error) {
	res := &agentproto.WorkspaceAgentScriptCompletedResponse{}

	_, err := s.Database.InsertWorkspaceAgentScriptTimings(ctx, database.InsertWorkspaceAgentScriptTimingsParams{
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
