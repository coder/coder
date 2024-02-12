package agentapi_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestBatchCreateLogs(t *testing.T) {
	t.Parallel()

	var (
		agent = database.WorkspaceAgent{
			ID: uuid.New(),
		}
		logSource = database.WorkspaceAgentLogSource{
			WorkspaceAgentID: agent.ID,
			CreatedAt:        dbtime.Now(),
			ID:               uuid.New(),
		}
	)

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		publishWorkspaceUpdateCalled := false
		publishWorkspaceAgentLogsUpdateCalled := false
		now := dbtime.Now()
		api := &agentapi.LogsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			PublishWorkspaceUpdateFn: func(ctx context.Context, wa *database.WorkspaceAgent) error {
				publishWorkspaceUpdateCalled = true
				return nil
			},
			PublishWorkspaceAgentLogsUpdateFn: func(ctx context.Context, workspaceAgentID uuid.UUID, msg agentsdk.LogsNotifyMessage) {
				publishWorkspaceAgentLogsUpdateCalled = true

				// Check the message content, should be for -1 since the lowest
				// log we inserted was 0.
				assert.Equal(t, agentsdk.LogsNotifyMessage{CreatedAfter: -1}, msg)
			},
			TimeNowFn: func() time.Time { return now },
		}

		req := &agentproto.BatchCreateLogsRequest{
			LogSourceId: logSource.ID[:],
			Logs: []*agentproto.Log{
				{
					CreatedAt: timestamppb.New(now),
					Level:     agentproto.Log_TRACE,
					Output:    "log line 1",
				},
				{
					CreatedAt: timestamppb.New(now.Add(time.Hour)),
					Level:     agentproto.Log_DEBUG,
					Output:    "log line 2",
				},
				{
					CreatedAt: timestamppb.New(now.Add(2 * time.Hour)),
					Level:     agentproto.Log_INFO,
					Output:    "log line 3",
				},
				{
					CreatedAt: timestamppb.New(now.Add(3 * time.Hour)),
					Level:     agentproto.Log_WARN,
					Output:    "log line 4",
				},
				{
					CreatedAt: timestamppb.New(now.Add(4 * time.Hour)),
					Level:     agentproto.Log_ERROR,
					Output:    "log line 5",
				},
				{
					CreatedAt: timestamppb.New(now.Add(5 * time.Hour)),
					Level:     -999, // defaults to INFO
					Output:    "log line 6",
				},
			},
		}

		// Craft expected DB request and response dynamically.
		insertWorkspaceAgentLogsParams := database.InsertWorkspaceAgentLogsParams{
			AgentID:      agent.ID,
			LogSourceID:  logSource.ID,
			CreatedAt:    now,
			Output:       make([]string, len(req.Logs)),
			Level:        make([]database.LogLevel, len(req.Logs)),
			OutputLength: 0,
		}
		insertWorkspaceAgentLogsReturn := make([]database.WorkspaceAgentLog, len(req.Logs))
		for i, logEntry := range req.Logs {
			insertWorkspaceAgentLogsParams.Output[i] = logEntry.Output
			level := database.LogLevelInfo
			if logEntry.Level >= 0 {
				level = database.LogLevel(strings.ToLower(logEntry.Level.String()))
			}
			insertWorkspaceAgentLogsParams.Level[i] = level
			insertWorkspaceAgentLogsParams.OutputLength += int32(len(logEntry.Output))

			insertWorkspaceAgentLogsReturn[i] = database.WorkspaceAgentLog{
				AgentID:     agent.ID,
				CreatedAt:   logEntry.CreatedAt.AsTime(),
				ID:          int64(i),
				Output:      logEntry.Output,
				Level:       insertWorkspaceAgentLogsParams.Level[i],
				LogSourceID: logSource.ID,
			}
		}

		dbM.EXPECT().InsertWorkspaceAgentLogs(gomock.Any(), insertWorkspaceAgentLogsParams).Return(insertWorkspaceAgentLogsReturn, nil)

		resp, err := api.BatchCreateLogs(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, &agentproto.BatchCreateLogsResponse{}, resp)
		require.True(t, publishWorkspaceUpdateCalled)
		require.True(t, publishWorkspaceAgentLogsUpdateCalled)
	})

	t.Run("NoWorkspacePublishIfNotFirstLogs", func(t *testing.T) {
		t.Parallel()

		agentWithLogs := agent
		agentWithLogs.LogsLength = 1

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		publishWorkspaceUpdateCalled := false
		publishWorkspaceAgentLogsUpdateCalled := false
		api := &agentapi.LogsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agentWithLogs, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			PublishWorkspaceUpdateFn: func(ctx context.Context, wa *database.WorkspaceAgent) error {
				publishWorkspaceUpdateCalled = true
				return nil
			},
			PublishWorkspaceAgentLogsUpdateFn: func(ctx context.Context, workspaceAgentID uuid.UUID, msg agentsdk.LogsNotifyMessage) {
				publishWorkspaceAgentLogsUpdateCalled = true
			},
		}

		// Don't really care about the DB call.
		dbM.EXPECT().InsertWorkspaceAgentLogs(gomock.Any(), gomock.Any()).Return([]database.WorkspaceAgentLog{
			{
				ID: 1,
			},
		}, nil)

		resp, err := api.BatchCreateLogs(context.Background(), &agentproto.BatchCreateLogsRequest{
			LogSourceId: logSource.ID[:],
			Logs: []*agentproto.Log{
				{
					CreatedAt: timestamppb.New(dbtime.Now()),
					Level:     agentproto.Log_INFO,
					Output:    "hello world",
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, &agentproto.BatchCreateLogsResponse{}, resp)
		require.False(t, publishWorkspaceUpdateCalled)
		require.True(t, publishWorkspaceAgentLogsUpdateCalled)
	})

	t.Run("AlreadyOverflowed", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		overflowedAgent := agent
		overflowedAgent.LogsOverflowed = true

		publishWorkspaceUpdateCalled := false
		publishWorkspaceAgentLogsUpdateCalled := false
		api := &agentapi.LogsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return overflowedAgent, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			PublishWorkspaceUpdateFn: func(ctx context.Context, wa *database.WorkspaceAgent) error {
				publishWorkspaceUpdateCalled = true
				return nil
			},
			PublishWorkspaceAgentLogsUpdateFn: func(ctx context.Context, workspaceAgentID uuid.UUID, msg agentsdk.LogsNotifyMessage) {
				publishWorkspaceAgentLogsUpdateCalled = true
			},
		}

		resp, err := api.BatchCreateLogs(context.Background(), &agentproto.BatchCreateLogsRequest{
			LogSourceId: logSource.ID[:],
			Logs:        []*agentproto.Log{},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.True(t, resp.LogLimitExceeded)
		require.False(t, publishWorkspaceUpdateCalled)
		require.False(t, publishWorkspaceAgentLogsUpdateCalled)
	})

	t.Run("InvalidLogSourceID", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		api := &agentapi.LogsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			// Test that they are ignored when nil.
			PublishWorkspaceUpdateFn:          nil,
			PublishWorkspaceAgentLogsUpdateFn: nil,
		}

		resp, err := api.BatchCreateLogs(context.Background(), &agentproto.BatchCreateLogsRequest{
			LogSourceId: []byte("invalid"),
			Logs: []*agentproto.Log{
				{}, // need at least 1 log
			},
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "parse log source ID")
		require.Nil(t, resp)
	})

	t.Run("UseExternalLogSourceID", func(t *testing.T) {
		t.Parallel()

		now := dbtime.Now()
		req := &agentproto.BatchCreateLogsRequest{
			LogSourceId: uuid.Nil[:], // defaults to "external"
			Logs: []*agentproto.Log{
				{
					CreatedAt: timestamppb.New(now),
					Level:     agentproto.Log_INFO,
					Output:    "hello world",
				},
			},
		}
		dbInsertParams := database.InsertWorkspaceAgentLogsParams{
			AgentID:      agent.ID,
			LogSourceID:  agentsdk.ExternalLogSourceID,
			CreatedAt:    now,
			Output:       []string{"hello world"},
			Level:        []database.LogLevel{database.LogLevelInfo},
			OutputLength: int32(len(req.Logs[0].Output)),
		}
		dbInsertRes := []database.WorkspaceAgentLog{
			{
				AgentID:     agent.ID,
				CreatedAt:   now,
				ID:          1,
				Output:      "hello world",
				Level:       database.LogLevelInfo,
				LogSourceID: agentsdk.ExternalLogSourceID,
			},
		}

		t.Run("Create", func(t *testing.T) {
			t.Parallel()

			dbM := dbmock.NewMockStore(gomock.NewController(t))

			publishWorkspaceUpdateCalled := false
			publishWorkspaceAgentLogsUpdateCalled := false
			api := &agentapi.LogsAPI{
				AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
					return agent, nil
				},
				Database: dbM,
				Log:      slogtest.Make(t, nil),
				PublishWorkspaceUpdateFn: func(ctx context.Context, wa *database.WorkspaceAgent) error {
					publishWorkspaceUpdateCalled = true
					return nil
				},
				PublishWorkspaceAgentLogsUpdateFn: func(ctx context.Context, workspaceAgentID uuid.UUID, msg agentsdk.LogsNotifyMessage) {
					publishWorkspaceAgentLogsUpdateCalled = true
				},
				TimeNowFn: func() time.Time { return now },
			}

			dbM.EXPECT().InsertWorkspaceAgentLogSources(gomock.Any(), database.InsertWorkspaceAgentLogSourcesParams{
				WorkspaceAgentID: agent.ID,
				CreatedAt:        now,
				ID:               []uuid.UUID{agentsdk.ExternalLogSourceID},
				DisplayName:      []string{"External"},
				Icon:             []string{"/emojis/1f310.png"},
			}).Return([]database.WorkspaceAgentLogSource{
				{
					// only the ID field is used
					ID: agentsdk.ExternalLogSourceID,
				},
			}, nil)
			dbM.EXPECT().InsertWorkspaceAgentLogs(gomock.Any(), dbInsertParams).Return(dbInsertRes, nil)

			resp, err := api.BatchCreateLogs(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, &agentproto.BatchCreateLogsResponse{}, resp)
			require.True(t, publishWorkspaceUpdateCalled)
			require.True(t, publishWorkspaceAgentLogsUpdateCalled)
		})

		t.Run("Exists", func(t *testing.T) {
			t.Parallel()

			dbM := dbmock.NewMockStore(gomock.NewController(t))

			publishWorkspaceUpdateCalled := false
			publishWorkspaceAgentLogsUpdateCalled := false
			api := &agentapi.LogsAPI{
				AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
					return agent, nil
				},
				Database: dbM,
				Log:      slogtest.Make(t, nil),
				PublishWorkspaceUpdateFn: func(ctx context.Context, wa *database.WorkspaceAgent) error {
					publishWorkspaceUpdateCalled = true
					return nil
				},
				PublishWorkspaceAgentLogsUpdateFn: func(ctx context.Context, workspaceAgentID uuid.UUID, msg agentsdk.LogsNotifyMessage) {
					publishWorkspaceAgentLogsUpdateCalled = true
				},
				TimeNowFn: func() time.Time { return now },
			}

			// Return a unique violation error to simulate the log source
			// already existing. This should be handled gracefully.
			logSourceInsertErr := &pq.Error{
				Code:       pq.ErrorCode("23505"), // unique_violation
				Constraint: string(database.UniqueWorkspaceAgentLogSourcesPkey),
			}
			dbM.EXPECT().InsertWorkspaceAgentLogSources(gomock.Any(), database.InsertWorkspaceAgentLogSourcesParams{
				WorkspaceAgentID: agent.ID,
				CreatedAt:        now,
				ID:               []uuid.UUID{agentsdk.ExternalLogSourceID},
				DisplayName:      []string{"External"},
				Icon:             []string{"/emojis/1f310.png"},
			}).Return([]database.WorkspaceAgentLogSource{}, logSourceInsertErr)

			dbM.EXPECT().InsertWorkspaceAgentLogs(gomock.Any(), dbInsertParams).Return(dbInsertRes, nil)

			resp, err := api.BatchCreateLogs(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, &agentproto.BatchCreateLogsResponse{}, resp)
			require.True(t, publishWorkspaceUpdateCalled)
			require.True(t, publishWorkspaceAgentLogsUpdateCalled)
		})
	})

	t.Run("Overflow", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		publishWorkspaceUpdateCalled := false
		publishWorkspaceAgentLogsUpdateCalled := false
		api := &agentapi.LogsAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: dbM,
			Log:      slogtest.Make(t, nil),
			PublishWorkspaceUpdateFn: func(ctx context.Context, wa *database.WorkspaceAgent) error {
				publishWorkspaceUpdateCalled = true
				return nil
			},
			PublishWorkspaceAgentLogsUpdateFn: func(ctx context.Context, workspaceAgentID uuid.UUID, msg agentsdk.LogsNotifyMessage) {
				publishWorkspaceAgentLogsUpdateCalled = true
			},
		}

		// Don't really care about the DB call params, just want to return an
		// error.
		dbErr := &pq.Error{
			Constraint: "max_logs_length",
			Table:      "workspace_agents",
		}
		dbM.EXPECT().InsertWorkspaceAgentLogs(gomock.Any(), gomock.Any()).Return(nil, dbErr)

		// Should also update the workspace agent.
		dbM.EXPECT().UpdateWorkspaceAgentLogOverflowByID(gomock.Any(), database.UpdateWorkspaceAgentLogOverflowByIDParams{
			ID:             agent.ID,
			LogsOverflowed: true,
		}).Return(nil)

		resp, err := api.BatchCreateLogs(context.Background(), &agentproto.BatchCreateLogsRequest{
			LogSourceId: logSource.ID[:],
			Logs: []*agentproto.Log{
				{
					CreatedAt: timestamppb.New(dbtime.Now()),
					Level:     agentproto.Log_INFO,
					Output:    "hello world",
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.True(t, resp.LogLimitExceeded)
		require.True(t, publishWorkspaceUpdateCalled)
		require.False(t, publishWorkspaceAgentLogsUpdateCalled)
	})
}
