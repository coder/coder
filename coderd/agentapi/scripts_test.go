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
				Stage:       agentproto.Timing_START,
				DisplayName: "Start Script",
				Start:       timestamppb.New(dbtime.Now()),
				End:         timestamppb.New(dbtime.Now().Add(time.Second)),
				TimedOut:    false,
				ExitCode:    0,
			},
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:       agentproto.Timing_STOP,
				DisplayName: "Stop Script",
				Start:       timestamppb.New(dbtime.Now()),
				End:         timestamppb.New(dbtime.Now().Add(time.Second)),
				TimedOut:    false,
				ExitCode:    0,
			},
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:       agentproto.Timing_CRON,
				DisplayName: "Cron Script",
				Start:       timestamppb.New(dbtime.Now()),
				End:         timestamppb.New(dbtime.Now().Add(time.Second)),
				TimedOut:    false,
				ExitCode:    0,
			},
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:       agentproto.Timing_START,
				DisplayName: "Timed Out Script",
				Start:       timestamppb.New(dbtime.Now()),
				End:         timestamppb.New(dbtime.Now().Add(time.Second)),
				TimedOut:    true,
				ExitCode:    255,
			},
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:       agentproto.Timing_START,
				DisplayName: "Failed Script",
				Start:       timestamppb.New(dbtime.Now()),
				End:         timestamppb.New(dbtime.Now().Add(time.Second)),
				TimedOut:    true,
				ExitCode:    1,
			},
		},
	}

	for _, tt := range tests {
		// Setup the script ID
		tt.timing.ScriptId = tt.scriptID[:]

		mDB := dbmock.NewMockStore(gomock.NewController(t))
		mDB.EXPECT().InsertWorkspaceAgentScriptTimings(gomock.Any(), database.InsertWorkspaceAgentScriptTimingsParams{
			ScriptID:    tt.scriptID,
			Stage:       protoScriptTimingStageToDatabase(tt.timing.Stage),
			DisplayName: tt.timing.DisplayName,
			StartedAt:   tt.timing.Start.AsTime(),
			EndedAt:     tt.timing.End.AsTime(),
			TimedOut:    tt.timing.TimedOut,
			ExitCode:    tt.timing.ExitCode,
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
