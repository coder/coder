package agentapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

func TestScriptCompleted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		scriptID uuid.UUID
		timing   *agentproto.Timing
	}{
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:    agentproto.Timing_START,
				Start:    timestamppb.New(dbtime.Now()),
				End:      timestamppb.New(dbtime.Now().Add(time.Second)),
				Status:   agentproto.Timing_OK,
				ExitCode: 0,
			},
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:    agentproto.Timing_STOP,
				Start:    timestamppb.New(dbtime.Now()),
				End:      timestamppb.New(dbtime.Now().Add(time.Second)),
				Status:   agentproto.Timing_OK,
				ExitCode: 0,
			},
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:    agentproto.Timing_CRON,
				Start:    timestamppb.New(dbtime.Now()),
				End:      timestamppb.New(dbtime.Now().Add(time.Second)),
				Status:   agentproto.Timing_OK,
				ExitCode: 0,
			},
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:    agentproto.Timing_START,
				Start:    timestamppb.New(dbtime.Now()),
				End:      timestamppb.New(dbtime.Now().Add(time.Second)),
				Status:   agentproto.Timing_TIMED_OUT,
				ExitCode: 255,
			},
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:    agentproto.Timing_START,
				Start:    timestamppb.New(dbtime.Now()),
				End:      timestamppb.New(dbtime.Now().Add(time.Second)),
				Status:   agentproto.Timing_EXIT_FAILURE,
				ExitCode: 1,
			},
		},
	}

	for _, tt := range tests {
		// Setup the script ID
		tt.timing.ScriptId = tt.scriptID[:]

		mDB := dbmock.NewMockStore(gomock.NewController(t))
		mDB.EXPECT().InsertWorkspaceAgentScriptTimings(gomock.Any(), database.InsertWorkspaceAgentScriptTimingsParams{
			ScriptID:  tt.scriptID,
			Stage:     protoScriptTimingStageToDatabase(tt.timing.Stage),
			Status:    protoScriptTimingStatusToDatabase(tt.timing.Status),
			StartedAt: tt.timing.Start.AsTime(),
			EndedAt:   tt.timing.End.AsTime(),
			ExitCode:  tt.timing.ExitCode,
		})

		api := &agentapi.ScriptsAPI{Database: mDB}
		api.ScriptCompleted(context.Background(), &agentproto.WorkspaceAgentScriptCompletedRequest{
			Timing: tt.timing,
		})
	}
}

func protoScriptTimingStageToDatabase(stage agentproto.Timing_Stage) database.WorkspaceAgentScriptTimingStage {
	var dbStage database.WorkspaceAgentScriptTimingStage
	switch stage {
	case agentproto.Timing_START:
		dbStage = database.WorkspaceAgentScriptTimingStageStart
	case agentproto.Timing_STOP:
		dbStage = database.WorkspaceAgentScriptTimingStageStop
	case agentproto.Timing_CRON:
		dbStage = database.WorkspaceAgentScriptTimingStageCron
	}
	return dbStage
}

func protoScriptTimingStatusToDatabase(stage agentproto.Timing_Status) database.WorkspaceAgentScriptTimingStatus {
	var dbStatus database.WorkspaceAgentScriptTimingStatus
	switch stage {
	case agentproto.Timing_OK:
		dbStatus = database.WorkspaceAgentScriptTimingStatusOk
	case agentproto.Timing_EXIT_FAILURE:
		dbStatus = database.WorkspaceAgentScriptTimingStatusExitFailure
	case agentproto.Timing_TIMED_OUT:
		dbStatus = database.WorkspaceAgentScriptTimingStatusTimedOut
	case agentproto.Timing_PIPES_LEFT_OPEN:
		dbStatus = database.WorkspaceAgentScriptTimingStatusPipesLeftOpen
	}
	return dbStatus
}
