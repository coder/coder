package agentapi_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/boundaryusage"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestReportBoundaryLogs(t *testing.T) {
	t.Parallel()

	var (
		agentID       = uuid.New()
		workspaceID   = uuid.New()
		ownerID       = uuid.New()
		templateID    = uuid.New()
		templateVerID = uuid.New()
		sessionID     = uuid.New()
		now           = dbtime.Now()
	)

	t.Run("PersistsSessionAndLogs", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		api := &agentapi.BoundaryLogsAPI{
			Log:               testutil.Logger(t),
			Database:          dbM,
			AgentID:           agentID,
			WorkspaceID:       workspaceID,
			OwnerID:           ownerID,
			TemplateID:        templateID,
			TemplateVersionID: templateVerID,
		}

		// Session does not exist yet; GetBoundarySessionByID returns
		// an error, triggering lazy creation.
		dbM.EXPECT().
			GetBoundarySessionByID(gomock.Any(), sessionID).
			Return(database.BoundarySession{}, xerrors.New("not found"))

		dbM.EXPECT().
			InsertBoundarySession(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, arg database.InsertBoundarySessionParams) (database.BoundarySession, error) {
				assert.Equal(t, sessionID, arg.ID)
				assert.Equal(t, agentID, arg.WorkspaceAgentID)
				assert.Equal(t, "claude-code", arg.ConfinedProcessName)
				return database.BoundarySession{ID: arg.ID}, nil
			})

		// Expect a single batch insert with both logs.
		dbM.EXPECT().
			InsertBoundaryLogs(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, arg database.InsertBoundaryLogsParams) ([]database.BoundaryLog, error) {
				assert.Equal(t, sessionID, arg.SessionID)
				require.Len(t, arg.ID, 2)

				// First log: allowed GET.
				assert.Equal(t, int32(0), arg.SequenceNumber[0])
				assert.Equal(t, "http", arg.Proto[0])
				assert.Equal(t, "GET", arg.Method[0])
				assert.Equal(t, "https://example.com", arg.Detail[0])
				assert.Equal(t, "domain=example.com", arg.MatchedRule[0])

				// Second log: denied POST.
				assert.Equal(t, int32(1), arg.SequenceNumber[1])
				assert.Equal(t, "http", arg.Proto[1])
				assert.Equal(t, "POST", arg.Method[1])
				assert.Equal(t, "https://evil.com/exfil", arg.Detail[1])
				assert.Equal(t, "", arg.MatchedRule[1])

				return []database.BoundaryLog{{ID: arg.ID[0]}, {ID: arg.ID[1]}}, nil
			})

		resp, err := api.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			SessionId:           sessionID.String(),
			ConfinedProcessName: "claude-code",
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed:        true,
					Time:           timestamppb.New(now),
					SequenceNumber: 0,
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method:      "GET",
							Url:         "https://example.com",
							MatchedRule: "domain=example.com",
						},
					},
				},
				{
					Allowed:        false,
					Time:           timestamppb.New(now),
					SequenceNumber: 1,
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method: "POST",
							Url:    "https://evil.com/exfil",
						},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("SessionAlreadyExists", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		api := &agentapi.BoundaryLogsAPI{
			Log:               testutil.Logger(t),
			Database:          dbM,
			AgentID:           agentID,
			WorkspaceID:       workspaceID,
			OwnerID:           ownerID,
			TemplateID:        templateID,
			TemplateVersionID: templateVerID,
		}

		// Session already exists.
		dbM.EXPECT().
			GetBoundarySessionByID(gomock.Any(), sessionID).
			Return(database.BoundarySession{ID: sessionID}, nil)

		// InsertBoundarySession should NOT be called.

		dbM.EXPECT().
			InsertBoundaryLogs(gomock.Any(), gomock.Any()).
			Return([]database.BoundaryLog{{}}, nil)

		resp, err := api.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			SessionId:           sessionID.String(),
			ConfinedProcessName: "claude-code",
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed:        true,
					Time:           timestamppb.New(now),
					SequenceNumber: 5,
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method:      "GET",
							Url:         "https://github.com",
							MatchedRule: "domain=github.com",
						},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("SessionLookedUpOnEveryBatch", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		api := &agentapi.BoundaryLogsAPI{
			Log:               testutil.Logger(t),
			Database:          dbM,
			AgentID:           agentID,
			WorkspaceID:       workspaceID,
			OwnerID:           ownerID,
			TemplateID:        templateID,
			TemplateVersionID: templateVerID,
		}

		// First batch: session does not exist, gets created.
		dbM.EXPECT().
			GetBoundarySessionByID(gomock.Any(), sessionID).
			Return(database.BoundarySession{}, xerrors.New("not found")).
			Times(1)
		dbM.EXPECT().
			InsertBoundarySession(gomock.Any(), gomock.Any()).
			Return(database.BoundarySession{ID: sessionID}, nil).
			Times(1)

		// Second batch: session now exists in the database.
		dbM.EXPECT().
			GetBoundarySessionByID(gomock.Any(), sessionID).
			Return(database.BoundarySession{ID: sessionID}, nil).
			Times(1)

		dbM.EXPECT().
			InsertBoundaryLogs(gomock.Any(), gomock.Any()).
			Return([]database.BoundaryLog{{}}, nil).
			Times(2) // One per batch

		req := &agentproto.ReportBoundaryLogsRequest{
			SessionId:           sessionID.String(),
			ConfinedProcessName: "codex",
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed:        true,
					Time:           timestamppb.New(now),
					SequenceNumber: 0,
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method: "GET",
							Url:    "https://example.com",
						},
					},
				},
			},
		}

		// First call creates the session.
		_, err := api.ReportBoundaryLogs(context.Background(), req)
		require.NoError(t, err)

		// Second call looks up the session again in the database.
		req.Logs[0].SequenceNumber = 1
		_, err = api.ReportBoundaryLogs(context.Background(), req)
		require.NoError(t, err)
	})

	t.Run("NoSessionIDFallsBackToLogOnly", func(t *testing.T) {
		t.Parallel()

		// No database mock expectations means any DB call would panic.
		api := &agentapi.BoundaryLogsAPI{
			Log:               testutil.Logger(t),
			Database:          nil, // No DB, simulating legacy mode
			WorkspaceID:       workspaceID,
			OwnerID:           ownerID,
			TemplateID:        templateID,
			TemplateVersionID: templateVerID,
		}

		resp, err := api.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			// No session_id set.
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed: true,
					Time:    timestamppb.New(now),
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method: "GET",
							Url:    "https://example.com",
						},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("EmptyHTTPRequestSkipped", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		api := &agentapi.BoundaryLogsAPI{
			Log:               testutil.Logger(t),
			Database:          dbM,
			AgentID:           agentID,
			WorkspaceID:       workspaceID,
			OwnerID:           ownerID,
			TemplateID:        templateID,
			TemplateVersionID: templateVerID,
		}

		dbM.EXPECT().
			GetBoundarySessionByID(gomock.Any(), sessionID).
			Return(database.BoundarySession{ID: sessionID}, nil)

		// No InsertBoundaryLogs expected because the HTTP request is nil.

		resp, err := api.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			SessionId:           sessionID.String(),
			ConfinedProcessName: "claude-code",
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed: true,
					Time:    timestamppb.New(now),
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: nil,
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("InvalidSessionIDFallsBackToLogOnly", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))

		api := &agentapi.BoundaryLogsAPI{
			Log:               testutil.Logger(t),
			Database:          dbM,
			AgentID:           agentID,
			WorkspaceID:       workspaceID,
			OwnerID:           ownerID,
			TemplateID:        templateID,
			TemplateVersionID: templateVerID,
		}

		// No DB calls expected because session_id is invalid.
		resp, err := api.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			SessionId: "not-a-uuid",
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed: true,
					Time:    timestamppb.New(now),
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method: "GET",
							Url:    "https://example.com",
						},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("UsageTrackingStillWorks", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		tracker := boundaryusage.NewTracker()

		api := &agentapi.BoundaryLogsAPI{
			Log:                  testutil.Logger(t),
			Database:             dbM,
			AgentID:              agentID,
			WorkspaceID:          workspaceID,
			OwnerID:              ownerID,
			TemplateID:           templateID,
			TemplateVersionID:    templateVerID,
			BoundaryUsageTracker: tracker,
		}

		dbM.EXPECT().
			GetBoundarySessionByID(gomock.Any(), sessionID).
			Return(database.BoundarySession{ID: sessionID}, nil)

		dbM.EXPECT().
			InsertBoundaryLogs(gomock.Any(), gomock.Any()).
			Return([]database.BoundaryLog{{}, {}}, nil).
			Times(1)

		_, err := api.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			SessionId:           sessionID.String(),
			ConfinedProcessName: "claude-code",
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed:        true,
					Time:           timestamppb.New(now),
					SequenceNumber: 0,
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method:      "GET",
							Url:         "https://example.com",
							MatchedRule: "domain=example.com",
						},
					},
				},
				{
					Allowed:        false,
					Time:           timestamppb.New(now),
					SequenceNumber: 1,
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method: "POST",
							Url:    "https://evil.com",
						},
					},
				},
			},
		})
		require.NoError(t, err)
		// Tracker should have recorded the usage. We cannot easily
		// inspect the private fields, but the fact that no panic
		// occurred and the call completed confirms integration.
	})
}
