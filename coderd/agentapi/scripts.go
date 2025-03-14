package agentapi
import (
	"fmt"
	"errors"
	"context"
	"github.com/google/uuid"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)
type ScriptsAPI struct {
	Database database.Store
}
func (s *ScriptsAPI) ScriptCompleted(ctx context.Context, req *agentproto.WorkspaceAgentScriptCompletedRequest) (*agentproto.WorkspaceAgentScriptCompletedResponse, error) {
	res := &agentproto.WorkspaceAgentScriptCompletedResponse{}
	scriptID, err := uuid.FromBytes(req.Timing.ScriptId)
	if err != nil {
		return nil, fmt.Errorf("script id from bytes: %w", err)
	}
	var stage database.WorkspaceAgentScriptTimingStage
	switch req.Timing.Stage {
	case agentproto.Timing_START:
		stage = database.WorkspaceAgentScriptTimingStageStart
	case agentproto.Timing_STOP:
		stage = database.WorkspaceAgentScriptTimingStageStop
	case agentproto.Timing_CRON:
		stage = database.WorkspaceAgentScriptTimingStageCron
	}
	var status database.WorkspaceAgentScriptTimingStatus
	switch req.Timing.Status {
	case agentproto.Timing_OK:
		status = database.WorkspaceAgentScriptTimingStatusOk
	case agentproto.Timing_EXIT_FAILURE:
		status = database.WorkspaceAgentScriptTimingStatusExitFailure
	case agentproto.Timing_TIMED_OUT:
		status = database.WorkspaceAgentScriptTimingStatusTimedOut
	case agentproto.Timing_PIPES_LEFT_OPEN:
		status = database.WorkspaceAgentScriptTimingStatusPipesLeftOpen
	}
	//nolint:gocritic // We need permissions to write to the DB here and we are in the context of the agent.
	ctx = dbauthz.AsProvisionerd(ctx)
	_, err = s.Database.InsertWorkspaceAgentScriptTimings(ctx, database.InsertWorkspaceAgentScriptTimingsParams{
		ScriptID:  scriptID,
		Stage:     stage,
		Status:    status,
		StartedAt: req.Timing.Start.AsTime(),
		EndedAt:   req.Timing.End.AsTime(),
		ExitCode:  req.Timing.ExitCode,
	})
	if err != nil {
		return nil, fmt.Errorf("insert workspace agent script timings into database: %w", err)
	}
	return res, nil
}
