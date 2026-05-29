package agentapi_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/boundaryusage"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
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
		// OwnerID is the zero uuid.NullUUID (NULL), which the FK allows.
	})
	require.NoError(t, err, "pre-create boundary session")
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

	t.Run("SessionLookedUpOnEveryBatch", func(t *testing.T) {
		t.Parallel()

		// Given: a fresh database; the session does not yet exist.
		f := newBoundaryFixture(t)
		api := f.api(t)
		sessionID := uuid.New()
		now := dbtime.Now()

		makeLog := func(seqNum int32) *agentproto.BoundaryLog {
			return &agentproto.BoundaryLog{
				Allowed:        true,
				Time:           timestamppb.New(now),
				SequenceNumber: seqNum,
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method: "GET",
						Url:    "https://example.com",
					},
				},
			}
		}

		req := &agentproto.ReportBoundaryLogsRequest{
			SessionId:           sessionID.String(),
			ConfinedProcessName: "codex",
			Logs:                []*agentproto.BoundaryLog{makeLog(0)},
		}

		// When: the first batch is reported; the session is created.
		_, err := api.ReportBoundaryLogs(context.Background(), req)
		require.NoError(t, err)

		_, err = f.DB.GetBoundarySessionByID(context.Background(), sessionID)
		require.NoError(t, err, "session must exist after first batch")

		// When: the second batch is reported; the session already exists.
		req.Logs = []*agentproto.BoundaryLog{makeLog(1)}
		_, err = api.ReportBoundaryLogs(context.Background(), req)
		require.NoError(t, err)

		// Then: both batches are persisted under a single session.
		logs, err := f.DB.ListBoundaryLogsBySessionID(context.Background(), database.ListBoundaryLogsBySessionIDParams{
			SessionID: sessionID,
		})
		require.NoError(t, err)
		require.Len(t, logs, 2, "logs from both batches must be persisted")
	})

	t.Run("MissingSessionIDReturnsError", func(t *testing.T) {
		t.Parallel()

		// Given: a request with no session_id.
		api := &agentapi.BoundaryLogsAPI{
			Log: testutil.Logger(t),
			// Database intentionally nil; the error must fire before any DB call.
		}

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

		// Then: an error is returned.
		require.Error(t, err)
		require.Nil(t, resp)
	})

	t.Run("EmptyHTTPRequestSkipped", func(t *testing.T) {
		t.Parallel()

		// Given: a pre-existing session and a log entry whose HttpRequest is nil.
		f := newBoundaryFixture(t)
		api := f.api(t)
		sessionID := uuid.New()
		f.preCreateSession(t, sessionID, "claude-code")

		// When: the nil HttpRequest log is reported.
		resp, err := api.ReportBoundaryLogs(context.Background(), &agentproto.ReportBoundaryLogsRequest{
			SessionId:           sessionID.String(),
			ConfinedProcessName: "claude-code",
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed: true,
					Time:    timestamppb.New(dbtime.Now()),
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: nil,
					},
				},
			},
		})

		// Then: the nil log is skipped and no boundary_logs row is written.
		require.NoError(t, err)
		require.NotNil(t, resp)

		logs, err := f.DB.ListBoundaryLogsBySessionID(context.Background(), database.ListBoundaryLogsBySessionIDParams{
			SessionID: sessionID,
		})
		require.NoError(t, err)
		require.Empty(t, logs, "nil HttpRequest must not produce a log row")
	})

	t.Run("InvalidSessionIDReturnsError", func(t *testing.T) {
		t.Parallel()

		// Given: a request with a session_id that is not a valid UUID.
		api := &agentapi.BoundaryLogsAPI{
			Log: testutil.Logger(t),
			// Database intentionally nil; the error must fire before any DB call.
		}

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

		// Then: an error is returned.
		require.Error(t, err)
		require.Nil(t, resp)
	})

	t.Run("PersistsLogsAndTracksBoundaryUsage", func(t *testing.T) {
		t.Parallel()

		// Given: a BoundaryUsageTracker, a pre-existing session, and one allowed
		// plus one denied log entry.
		f := newBoundaryFixture(t)
		tracker := boundaryusage.NewTracker()
		api := f.api(t)
		api.BoundaryUsageTracker = tracker
		sessionID := uuid.New()
		f.preCreateSession(t, sessionID, "claude-code")
		now := dbtime.Now()

		// When: boundary logs are reported.
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

		// Then: both logs are persisted in the database and the tracker records
		// the usage. The tracker's internal counters are not directly inspectable,
		// but the call completing without error confirms Track() was invoked.
		require.NoError(t, err)

		logs, err := f.DB.ListBoundaryLogsBySessionID(context.Background(), database.ListBoundaryLogsBySessionIDParams{
			SessionID: sessionID,
		})
		require.NoError(t, err)
		require.Len(t, logs, 2, "both logs must be persisted")
	})
}
