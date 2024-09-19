package agentapi

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
)

type ScriptsAPI struct {
	AgentID  uuid.UUID
	Database database.Store
}

func (s *ScriptsAPI) ScriptCompleted(ctx context.Context, req *agentproto.WorkspaceAgentScriptCompletedRequest) (*agentproto.WorkspaceAgentScriptCompletedResponse, error) {
	res := &agentproto.WorkspaceAgentScriptCompletedResponse{}

	_, err := s.Database.InsertWorkspaceAgentScriptTimings(ctx, database.InsertWorkspaceAgentScriptTimingsParams{
		AgentID:      s.AgentID,
		DisplayName:  req.Timing.DisplayName,
		StartedAt:    req.Timing.Start.AsTime(),
		EndedAt:      req.Timing.End.AsTime(),
		ExitCode:     req.Timing.ExitCode,
		RanOnStart:   req.Timing.RanOnStart,
		BlockedLogin: req.Timing.BlockedLogin,
	})
	if err != nil {
		return nil, xerrors.Errorf("insert workspace agent script timings into database: %w", err)
	}

	return res, nil
}
