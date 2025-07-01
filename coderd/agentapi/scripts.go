package agentapi

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

type ScriptsAPI struct {
	Database database.Store
}

func (s *ScriptsAPI) ScriptCompleted(ctx context.Context, req *agentproto.WorkspaceAgentScriptCompletedRequest) (*agentproto.WorkspaceAgentScriptCompletedResponse, error) {
	res := &agentproto.WorkspaceAgentScriptCompletedResponse{}

	if req.GetTiming() == nil {
		return nil, xerrors.New("script timing is required")
	}

	scriptID, err := uuid.FromBytes(req.GetTiming().GetScriptId())
	if err != nil {
		return nil, xerrors.Errorf("script id from bytes: %w", err)
	}

	scriptStart := req.GetTiming().GetStart()
	if !scriptStart.IsValid() || scriptStart.AsTime().IsZero() {
		return nil, xerrors.New("script start time is required and cannot be zero")
	}

	scriptEnd := req.GetTiming().GetEnd()
	if !scriptEnd.IsValid() || scriptEnd.AsTime().IsZero() {
		return nil, xerrors.New("script end time is required and cannot be zero")
	}

	if scriptStart.AsTime().After(scriptEnd.AsTime()) {
		return nil, xerrors.New("script start time cannot be after end time")
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
		return nil, xerrors.Errorf("insert workspace agent script timings into database: %w", err)
	}

	return res, nil
}
