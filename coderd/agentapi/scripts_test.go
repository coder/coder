package agentapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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
		scriptID     uuid.UUID
		timing       *agentproto.Timing
		expectInsert bool
		expectError  string
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
			expectInsert: true,
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
			expectInsert: true,
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
			expectInsert: true,
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
			expectInsert: true,
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
			expectInsert: true,
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:    agentproto.Timing_START,
				Start:    nil,
				End:      timestamppb.New(dbtime.Now().Add(time.Second)),
				Status:   agentproto.Timing_OK,
				ExitCode: 0,
			},
			expectInsert: false,
			expectError:  "script start time is required and cannot be zero",
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:    agentproto.Timing_START,
				Start:    timestamppb.New(dbtime.Now()),
				End:      nil,
				Status:   agentproto.Timing_OK,
				ExitCode: 0,
			},
			expectInsert: false,
			expectError:  "script end time is required and cannot be zero",
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:    agentproto.Timing_START,
				Start:    timestamppb.New(time.Time{}),
				End:      timestamppb.New(dbtime.Now()),
				Status:   agentproto.Timing_OK,
				ExitCode: 0,
			},
			expectInsert: false,
			expectError:  "script start time is required and cannot be zero",
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:    agentproto.Timing_START,
				Start:    timestamppb.New(dbtime.Now()),
				End:      timestamppb.New(time.Time{}),
				Status:   agentproto.Timing_OK,
				ExitCode: 0,
			},
			expectInsert: false,
			expectError:  "script end time is required and cannot be zero",
		},
		{
			scriptID: uuid.New(),
			timing: &agentproto.Timing{
				Stage:    agentproto.Timing_START,
				Start:    timestamppb.New(dbtime.Now()),
				End:      timestamppb.New(dbtime.Now().Add(-time.Second)),
				Status:   agentproto.Timing_OK,
				ExitCode: 0,
			},
			expectInsert: false,
			expectError:  "script start time cannot be after end time",
		},
	}

	for _, tt := range tests {
		// Setup the script ID
		tt.timing.ScriptId = tt.scriptID[:]

		mDB := dbmock.NewMockStore(gomock.NewController(t))
		if tt.expectInsert {
			mDB.EXPECT().InsertWorkspaceAgentScriptTimings(gomock.Any(), database.InsertWorkspaceAgentScriptTimingsParams{
				ScriptID:  tt.scriptID,
				Stage:     protoScriptTimingStageToDatabase(tt.timing.Stage),
				Status:    protoScriptTimingStatusToDatabase(tt.timing.Status),
				StartedAt: tt.timing.Start.AsTime(),
				EndedAt:   tt.timing.End.AsTime(),
				ExitCode:  tt.timing.ExitCode,
			})
		}

		api := &agentapi.ScriptsAPI{Database: mDB}
		_, err := api.ScriptCompleted(context.Background(), &agentproto.WorkspaceAgentScriptCompletedRequest{
			Timing: tt.timing,
		})
		if tt.expectError != "" {
			require.Contains(t, err.Error(), tt.expectError, "expected error did not match")
		} else {
			require.NoError(t, err, "expected no error but got one")
		}
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
