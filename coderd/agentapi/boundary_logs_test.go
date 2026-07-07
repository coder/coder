package agentapi_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

// boundaryFixture holds all database prerequisites for boundary log tests.
type boundaryFixture struct {
	DB            database.Store
	AgentID       uuid.UUID
	WorkspaceID   uuid.UUID
	OwnerID       uuid.UUID
	TemplateID    uuid.UUID
	TemplateVerID uuid.UUID
}

// newBoundaryFixture creates the full workspace-agent prerequisite chain needed
// by InsertBoundarySession's FK constraint on workspace_agent_id.
func newBoundaryFixture(t *testing.T) *boundaryFixture {
	t.Helper()
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	tmpl := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tmplVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{Valid: true, UUID: tmpl.ID},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     tmpl.ID,
		OwnerID:        user.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		Type: database.ProvisionerJobTypeWorkspaceBuild,
	})
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		JobID:             job.ID,
		WorkspaceID:       workspace.ID,
		TemplateVersionID: tmplVersion.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: build.JobID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: resource.ID,
	})
	return &boundaryFixture{
		DB:            db,
		AgentID:       agent.ID,
		WorkspaceID:   workspace.ID,
		OwnerID:       user.ID,
		TemplateID:    tmpl.ID,
		TemplateVerID: tmplVersion.ID,
	}
}

// api returns a new BoundaryLogsAPI backed by this fixture's database.
func (f *boundaryFixture) api(t *testing.T) *agentapi.BoundaryLogsAPI {
	return &agentapi.BoundaryLogsAPI{
		Log:               testutil.Logger(t),
		Database:          f.DB,
		AgentID:           f.AgentID,
		WorkspaceID:       f.WorkspaceID,
		OwnerID:           f.OwnerID,
		TemplateID:        f.TemplateID,
		TemplateVersionID: f.TemplateVerID,
	}
}

// preCreateSession inserts a boundary session directly, bypassing ensureSession,
// to simulate a session created by a prior request or a different coderd replica.
func (f *boundaryFixture) preCreateSession(t *testing.T, sessionID uuid.UUID, process string) {
	t.Helper()
	_, err := f.DB.InsertBoundarySession(context.Background(), database.InsertBoundarySessionParams{
		ID:                  sessionID,
		WorkspaceAgentID:    f.AgentID,
		ConfinedProcessName: process,
		StartedAt:           dbtime.Now(),
		UpdatedAt:           dbtime.Now(),
		OwnerID:             uuid.NullUUID{UUID: f.OwnerID, Valid: true},
	})
	require.NoError(t, err, "pre-create boundary session")
}

// addAgent creates another workspace agent in the same workspace chain,
// allowing tests to simulate multiple agents sharing one database.
func (f *boundaryFixture) addAgent(t *testing.T) uuid.UUID {
	t.Helper()
	job := dbgen.ProvisionerJob(t, f.DB, nil, database.ProvisionerJob{
		Type: database.ProvisionerJobTypeWorkspaceBuild,
	})
	build := dbgen.WorkspaceBuild(t, f.DB, database.WorkspaceBuild{
		JobID:             job.ID,
		WorkspaceID:       f.WorkspaceID,
		BuildNumber:       2,
		TemplateVersionID: f.TemplateVerID,
	})
	resource := dbgen.WorkspaceResource(t, f.DB, database.WorkspaceResource{
		JobID: build.JobID,
	})
	agent := dbgen.WorkspaceAgent(t, f.DB, database.WorkspaceAgent{
		ResourceID: resource.ID,
	})
	return agent.ID
}

func TestReportBoundaryLogs(t *testing.T) {
	t.Parallel()

	t.Run("PersistsSessionAndLogs", func(t *testing.T) {
		t.Parallel()

		// Given: a fresh database and two HTTP log entries (one allowed, one denied).
		f := newBoundaryFixture(t)
		api := f.api(t)
		sessionID := uuid.New()
		now := dbtime.Now()

		// When: boundary logs are reported.
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

		// Then: one boundary_sessions row and two boundary_logs rows are written.
		require.NoError(t, err)
		require.NotNil(t, resp)

		sess, err := f.DB.GetBoundarySessionByID(context.Background(), sessionID)
		require.NoError(t, err)
		require.Equal(t, sessionID, sess.ID)
		require.Equal(t, f.AgentID, sess.WorkspaceAgentID)
		require.Equal(t, "claude-code", sess.ConfinedProcessName)

		logs, err := f.DB.ListBoundaryLogsBySessionID(context.Background(), database.ListBoundaryLogsBySessionIDParams{
			SessionID: sessionID,
		})
		require.NoError(t, err)
		require.Len(t, logs, 2)

		require.Equal(t, int32(0), logs[0].SequenceNumber)
		require.Equal(t, "http", logs[0].Proto)
		require.Equal(t, "GET", logs[0].Method)
		require.Equal(t, "https://example.com", logs[0].Detail)
		require.Equal(t, "domain=example.com", logs[0].MatchedRule.String)

		require.Equal(t, int32(1), logs[1].SequenceNumber)
		require.Equal(t, "http", logs[1].Proto)
		require.Equal(t, "POST", logs[1].Method)
		require.Equal(t, "https://evil.com/exfil", logs[1].Detail)
		require.Equal(t, "", logs[1].MatchedRule.String)
	})

	t.Run("SessionAlreadyExistsSameInstance", func(t *testing.T) {
		t.Parallel()

		// Given: a session created during an earlier batch from the same
		// BoundaryLogsAPI instance (e.g. the normal second-and-beyond batch path).
		f := newBoundaryFixture(t)
		api := f.api(t)
		sessionID := uuid.New()
		f.preCreateSession(t, sessionID, "claude-code")

		// When: a subsequent batch arrives for the same session.
		resp, err := api.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			SessionId:           sessionID.String(),
			ConfinedProcessName: "claude-code",
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed:        true,
					Time:           timestamppb.New(dbtime.Now()),
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

		// Then: no duplicate session row is created and the new log is persisted.
		require.NoError(t, err)
		require.NotNil(t, resp)

		_, err = f.DB.GetBoundarySessionByID(context.Background(), sessionID)
		require.NoError(t, err)

		logs, err := f.DB.ListBoundaryLogsBySessionID(context.Background(), database.ListBoundaryLogsBySessionIDParams{
			SessionID: sessionID,
		})
		require.NoError(t, err)
		require.Len(t, logs, 1)
		require.Equal(t, int32(5), logs[0].SequenceNumber)
	})

	t.Run("SessionAlreadyExistsDifferentInstance", func(t *testing.T) {
		t.Parallel()

		// Given: a session created by a first BoundaryLogsAPI instance (first
		// coderd replica). A second instance backed by the same database receives
		// logs for the same session ID.
		f := newBoundaryFixture(t)
		api1 := f.api(t)
		api2 := f.api(t) // independent struct, simulates a different coderd replica
		sessionID := uuid.New()
		now := dbtime.Now()

		// api1 processes the first batch and creates the session.
		_, err := api1.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
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
							Url:    "https://openai.com",
						},
					},
				},
			},
		})
		require.NoError(t, err)

		// When: api2 processes a subsequent batch for the same session.
		resp, err := api2.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			SessionId:           sessionID.String(),
			ConfinedProcessName: "codex",
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed:        false,
					Time:           timestamppb.New(now),
					SequenceNumber: 1,
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method: "POST",
							Url:    "https://pastebin.com",
						},
					},
				},
			},
		})

		// Then: the existing session is reused and both log batches are persisted.
		require.NoError(t, err)
		require.NotNil(t, resp)

		_, err = f.DB.GetBoundarySessionByID(context.Background(), sessionID)
		require.NoError(t, err, "session must still exist")

		logs, err := f.DB.ListBoundaryLogsBySessionID(context.Background(), database.ListBoundaryLogsBySessionIDParams{
			SessionID: sessionID,
		})
		require.NoError(t, err)
		require.Len(t, logs, 2, "logs from both instances must be persisted")
	})

	t.Run("MissingSessionIDFallsBackToLogOnly", func(t *testing.T) {
		t.Parallel()

		// Given: a real database and a request with no session_id (old boundary client).
		f := newBoundaryFixture(t)
		api := f.api(t)

		// When: boundary logs are reported without a session_id.
		resp, err := api.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed: true,
					Time:    timestamppb.New(dbtime.Now()),
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method: "GET",
							Url:    "https://example.com",
						},
					},
				},
			},
		})

		// Then: the request succeeds (log-only mode) and no rows are persisted.
		require.NoError(t, err)
		require.NotNil(t, resp)

		logs, err := f.DB.ListBoundaryLogsBySessionID(context.Background(), database.ListBoundaryLogsBySessionIDParams{
			SessionID: uuid.Nil,
		})
		require.NoError(t, err)
		require.Empty(t, logs, "no boundary_logs rows should be persisted without a session_id")
	})

	t.Run("InvalidSessionIDFallsBackToLogOnly", func(t *testing.T) {
		t.Parallel()

		// Given: a real database and a request with a session_id that is not a valid UUID.
		f := newBoundaryFixture(t)
		api := f.api(t)

		// When: boundary logs are reported with an invalid session_id.
		resp, err := api.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			SessionId: "not-a-uuid",
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed: true,
					Time:    timestamppb.New(dbtime.Now()),
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method: "GET",
							Url:    "https://example.com",
						},
					},
				},
			},
		})

		// Then: the request succeeds (log-only mode) and no rows are persisted.
		require.NoError(t, err)
		require.NotNil(t, resp)

		logs, err := f.DB.ListBoundaryLogsBySessionID(context.Background(), database.ListBoundaryLogsBySessionIDParams{
			SessionID: uuid.Nil,
		})
		require.NoError(t, err)
		require.Empty(t, logs, "no boundary_logs rows should be persisted with an invalid session_id")
	})

	t.Run("SameSessionIDDifferentAgents", func(t *testing.T) {
		t.Parallel()

		// Given: two workspace agents in the same workspace, both reporting
		// logs with the same session ID. A UUID collision across agents is
		// negligible in practice; sessions are namespaced by agent_id at
		// query time. The first agent creates the session; the second
		// agent's ensureSession hits a unique constraint violation and
		// treats it as success.
		f := newBoundaryFixture(t)
		agent2ID := f.addAgent(t)

		api1 := f.api(t)
		api2 := &agentapi.BoundaryLogsAPI{
			Log:               testutil.Logger(t),
			Database:          f.DB,
			AgentID:           agent2ID,
			WorkspaceID:       f.WorkspaceID,
			OwnerID:           f.OwnerID,
			TemplateID:        f.TemplateID,
			TemplateVersionID: f.TemplateVerID,
		}

		sessionID := uuid.New()
		now := dbtime.Now()

		// When: agent1 reports the first batch, creating the session.
		_, err := api1.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			SessionId:           sessionID.String(),
			ConfinedProcessName: "claude-code",
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
		})
		require.NoError(t, err)

		// When: agent2 reports a batch with the same session ID.
		// ensureSession should hit the unique violation and treat it as success.
		resp, err := api2.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			SessionId:           sessionID.String(),
			ConfinedProcessName: "claude-code",
			Logs: []*agentproto.BoundaryLog{
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

		// Then: both agents' logs are persisted under the same session.
		require.NoError(t, err)
		require.NotNil(t, resp)

		sess, err := f.DB.GetBoundarySessionByID(context.Background(), sessionID)
		require.NoError(t, err)
		require.Equal(t, f.AgentID, sess.WorkspaceAgentID, "session belongs to the first agent that created it")

		logs, err := f.DB.ListBoundaryLogsBySessionID(context.Background(), database.ListBoundaryLogsBySessionIDParams{
			SessionID: sessionID,
		})
		require.NoError(t, err)
		require.Len(t, logs, 2, "logs from both agents must be persisted")
	})
}

// httpLogRequest builds a ReportBoundaryLogsRequest carrying a single allowed
// HTTP log for the given session.
func httpLogRequest(sessionID uuid.UUID, seq int32) *agentproto.ReportBoundaryLogsRequest {
	return &agentproto.ReportBoundaryLogsRequest{
		SessionId:           sessionID.String(),
		ConfinedProcessName: "claude-code",
		Logs: []*agentproto.BoundaryLog{
			{
				Allowed:        true,
				Time:           timestamppb.New(dbtime.Now()),
				SequenceNumber: seq,
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method:      "GET",
						Url:         "https://example.com",
						MatchedRule: "domain=example.com",
					},
				},
			},
		},
	}
}

// TestReportBoundaryLogsSessionGuard verifies that once a session has been
// ensured, later batches from the same connection skip the existence check and
// insert entirely, touching the database only for the logs themselves.
func TestReportBoundaryLogsSessionGuard(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	sessionID := uuid.New()
	api := &agentapi.BoundaryLogsAPI{
		Log:               testutil.Logger(t),
		Database:          db,
		AgentID:           uuid.New(),
		WorkspaceID:       uuid.New(),
		OwnerID:           uuid.New(),
		TemplateID:        uuid.New(),
		TemplateVersionID: uuid.New(),
	}

	// The session is inserted once, even though two batches arrive. Times(1)
	// fails the test if the guard does not suppress the second ensure attempt.
	// Logs insert on every batch.
	db.EXPECT().InsertBoundarySession(gomock.Any(), gomock.Any()).
		Return(database.BoundarySession{}, nil).Times(1)
	db.EXPECT().InsertBoundaryLogs(gomock.Any(), gomock.Any()).
		Return([]database.BoundaryLog{}, nil).Times(2)

	_, err := api.ReportBoundaryLogs(context.Background(), httpLogRequest(sessionID, 0))
	require.NoError(t, err)
	_, err = api.ReportBoundaryLogs(context.Background(), httpLogRequest(sessionID, 1))
	require.NoError(t, err)
}

// TestReportBoundaryLogsSessionRetriedOnError verifies that when ensureSession
// fails, the guard is not set, so the next batch retries the existence check.
func TestReportBoundaryLogsSessionRetriedOnError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	sessionID := uuid.New()
	api := &agentapi.BoundaryLogsAPI{
		// The first batch deliberately triggers a transient error, which
		// logs at ERROR level. Ignore errors so slogtest does not fail.
		Log:               slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		Database:          db,
		AgentID:           uuid.New(),
		WorkspaceID:       uuid.New(),
		OwnerID:           uuid.New(),
		TemplateID:        uuid.New(),
		TemplateVersionID: uuid.New(),
	}

	// First batch: insert fails transiently, so the session is not marked
	// ensured. Second batch: insert is retried and succeeds. Logs insert on both.
	gomock.InOrder(
		db.EXPECT().InsertBoundarySession(gomock.Any(), gomock.Any()).
			Return(database.BoundarySession{}, sql.ErrConnDone),
		db.EXPECT().InsertBoundarySession(gomock.Any(), gomock.Any()).
			Return(database.BoundarySession{}, nil),
	)
	db.EXPECT().InsertBoundaryLogs(gomock.Any(), gomock.Any()).
		Return([]database.BoundaryLog{}, nil).Times(2)

	_, err := api.ReportBoundaryLogs(context.Background(), httpLogRequest(sessionID, 0))
	require.NoError(t, err)
	_, err = api.ReportBoundaryLogs(context.Background(), httpLogRequest(sessionID, 1))
	require.NoError(t, err)
}
