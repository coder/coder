package coderd_test

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestBoundarySessionByID(t *testing.T) {
	t.Parallel()

	// seedBoundarySession creates the workspace -> build -> resource ->
	// agent chain, then inserts a boundary session linked to that agent.
	// It returns the session and the workspace owner ID.
	seedBoundarySession := func(t *testing.T, db database.Store, ownerID, orgID, templateID uuid.UUID) database.BoundarySession {
		t.Helper()

		ws := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        ownerID,
			OrganizationID: orgID,
			TemplateID:     templateID,
		})
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: orgID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
		})
		dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			WorkspaceID:       ws.ID,
			JobID:             job.ID,
			TemplateVersionID: templateID,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})

		//nolint:gocritic // Seeding data requires system context.
		session := dbgen.BoundarySession(t, db, database.BoundarySession{
			WorkspaceAgentID: agent.ID,
			ConfinedProcess:  "claude-code",
		})
		return session
	}

	t.Run("Owner", func(t *testing.T) {
		t.Parallel()

		ownerClient, _, api := coderdtest.NewWithAPI(t, nil)
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)
		db := api.Database

		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      owner.UserID,
		})

		session := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID, tpl.ID)

		// Owner should be able to read the session.
		//nolint:gocritic // Testing owner role.
		got, err := ownerClient.BoundarySessionByID(ctx, session.ID)
		require.NoError(t, err)
		require.Equal(t, session.ID, got.ID)
		require.Equal(t, owner.UserID, got.OwnerID)
		require.Equal(t, session.ConfinedProcess, got.ConfinedProcess)
		require.NotEqual(t, uuid.Nil, got.WorkspaceID)
	})

	t.Run("Auditor", func(t *testing.T) {
		t.Parallel()

		ownerClient, _, api := coderdtest.NewWithAPI(t, nil)
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)
		db := api.Database

		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      owner.UserID,
		})

		session := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID, tpl.ID)

		// Create an auditor user; auditors have read access to
		// boundary_log resources.
		auditorClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleAuditor())

		got, err := auditorClient.BoundarySessionByID(ctx, session.ID)
		require.NoError(t, err)
		require.Equal(t, session.ID, got.ID)
		require.Equal(t, "claude-code", got.ConfinedProcess)
	})

	t.Run("MemberDenied", func(t *testing.T) {
		t.Parallel()

		ownerClient, _, api := coderdtest.NewWithAPI(t, nil)
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)
		db := api.Database

		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      owner.UserID,
		})

		session := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID, tpl.ID)

		// Create a plain member; members have no read access to
		// boundary_log resources.
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		_, err := memberClient.BoundarySessionByID(ctx, session.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Testing owner role.
		_, err := ownerClient.BoundarySessionByID(ctx, uuid.New())
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

// TestBoundarySessionByID_DBAuth verifies that the dbauthz layer
// correctly gates access to GetBoundarySessionByID using
// ResourceBoundaryLog with ActionRead.
func TestBoundarySessionByID_DBAuth(t *testing.T) {
	t.Parallel()

	ownerClient, _, api := coderdtest.NewWithAPI(t, nil)
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	db := api.Database
	ctx := testutil.Context(t, testutil.WaitLong)

	tpl := dbgen.Template(t, db, database.Template{
		OrganizationID: owner.OrganizationID,
		CreatedBy:      owner.UserID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        owner.UserID,
		OrganizationID: owner.OrganizationID,
		TemplateID:     tpl.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: owner.OrganizationID,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       ws.ID,
		JobID:             job.ID,
		TemplateVersionID: tpl.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: job.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: resource.ID,
	})

	//nolint:gocritic // Seeding data requires system context.
	session := dbgen.BoundarySession(t, db, database.BoundarySession{
		WorkspaceAgentID: agent.ID,
		ConfinedProcess:  "codex",
	})

	// AsSystemRestricted should succeed.
	got, err := db.GetBoundarySessionByID(dbauthz.AsSystemRestricted(ctx), session.ID)
	require.NoError(t, err)
	require.Equal(t, session.ID, got.ID)
}

func TestBoundarySessionLogs(t *testing.T) {
	t.Parallel()

	// setupBoundarySession creates the prerequisite database objects
	// (provisioner job, workspace resource, workspace agent, boundary
	// session) and returns the session along with a helper that inserts
	// boundary logs for that session.
	type logOpt struct {
		SeqNum  int64
		Allowed bool
		Proto   string
		Method  string
		Detail  string
		Rule    string
	}
	setupBoundarySession := func(t *testing.T, db database.Store) (database.BoundarySession, func(opts ...logOpt)) {
		t.Helper()
		// Use an owner-level context to bypass authorization
		// checks when inserting prerequisite test data.
		//nolint:gocritic // Test helper needs owner context.
		sysCtx := dbauthz.AsSystemRestricted(context.Background())
		//nolint:gocritic // Test helper needs owner context for audit-log creates.
		ownerCtx := dbauthz.As(context.Background(), rbac.Subject{
			ID:    "owner",
			Roles: rbac.Roles(must(rbac.RoleIdentifiers{rbac.RoleOwner()}.Expand())),
			Scope: rbac.ExpandableScope(rbac.ScopeAll),
		})

		defOrg, err := db.GetDefaultOrganization(sysCtx)
		require.NoError(t, err)

		job, err := db.InsertProvisionerJob(sysCtx, database.InsertProvisionerJobParams{
			ID:             uuid.New(),
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			OrganizationID: defOrg.ID,
			InitiatorID:    uuid.New(),
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         uuid.New(),
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte("{}"),
			Tags:           database.StringMap{},
		})
		require.NoError(t, err)
		resource, err := db.InsertWorkspaceResource(sysCtx, database.InsertWorkspaceResourceParams{
			ID:         uuid.New(),
			CreatedAt:  dbtime.Now(),
			JobID:      job.ID,
			Transition: database.WorkspaceTransitionStart,
			Type:       "fake",
			Name:       "test-resource",
		})
		require.NoError(t, err)
		agent, err := db.InsertWorkspaceAgent(sysCtx, database.InsertWorkspaceAgentParams{
			ID:                       uuid.New(),
			CreatedAt:                dbtime.Now(),
			UpdatedAt:                dbtime.Now(),
			ResourceID:               resource.ID,
			AuthToken:                uuid.New(),
			Name:                     "test-agent",
			Architecture:             "amd64",
			OperatingSystem:          "linux",
			ConnectionTimeoutSeconds: 3600,
			APIKeyScope:              database.AgentKeyScopeEnumAll,
		})
		require.NoError(t, err)
		session := dbgen.BoundarySession(t, db, database.BoundarySession{
			WorkspaceAgentID: agent.ID,
			ConfinedProcess:  "claude-code",
		})
		insertLogs := func(opts ...logOpt) {
			t.Helper()
			for _, o := range opts {
				var matchedRule sql.NullString
				if o.Rule != "" {
					matchedRule = sql.NullString{String: o.Rule, Valid: true}
				}
				_, err := db.InsertBoundaryLog(ownerCtx, database.InsertBoundaryLogParams{
					ID:             uuid.New(),
					SessionID:      session.ID,
					SequenceNumber: o.SeqNum,
					Allowed:        o.Allowed,
					Proto:          o.Proto,
					Method:         o.Method,
					Detail:         o.Detail,
					MatchedRule:    matchedRule,
					CapturedAt:     dbtime.Now(),
					CreatedAt:      dbtime.Now(),
				})
				require.NoError(t, err, "insert boundary log seq=%d", o.SeqNum)
			}
		}
		return session, insertLogs
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		session, insertLogs := setupBoundarySession(t, db)
		insertLogs(
			logOpt{SeqNum: 0, Allowed: true, Proto: "http", Method: "GET", Detail: "https://github.com/coder/coder", Rule: "domain=github.com"},
			logOpt{SeqNum: 1, Allowed: false, Proto: "http", Method: "POST", Detail: "https://evil.com/exfil"},
			logOpt{SeqNum: 2, Allowed: true, Proto: "http", Method: "POST", Detail: "https://api.anthropic.com/v1/messages", Rule: "domain=api.anthropic.com"},
		)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.BoundarySessionLogs(ctx, session.ID, codersdk.BoundarySessionLogsParams{})
		require.NoError(t, err)
		require.Len(t, resp.Results, 3)

		// Verify ordering by sequence number ascending.
		require.Equal(t, int64(0), resp.Results[0].SequenceNumber)
		require.Equal(t, int64(1), resp.Results[1].SequenceNumber)
		require.Equal(t, int64(2), resp.Results[2].SequenceNumber)

		// Verify fields.
		require.True(t, resp.Results[0].Allowed)
		require.Equal(t, "https://github.com/coder/coder", resp.Results[0].Detail)
		require.NotNil(t, resp.Results[0].MatchedRule)
		require.Equal(t, "domain=github.com", *resp.Results[0].MatchedRule)

		require.False(t, resp.Results[1].Allowed)
		require.Nil(t, resp.Results[1].MatchedRule)
	})

	t.Run("SeqAfter", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		session, insertLogs := setupBoundarySession(t, db)
		insertLogs(
			logOpt{SeqNum: 0, Allowed: true, Proto: "http", Method: "GET", Detail: "https://a.com"},
			logOpt{SeqNum: 1, Allowed: true, Proto: "http", Method: "GET", Detail: "https://b.com"},
			logOpt{SeqNum: 2, Allowed: true, Proto: "http", Method: "GET", Detail: "https://c.com"},
		)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		after := int64(0)
		resp, err := client.BoundarySessionLogs(ctx, session.ID, codersdk.BoundarySessionLogsParams{
			SeqAfter: &after,
		})
		require.NoError(t, err)
		require.Len(t, resp.Results, 2)
		require.Equal(t, int64(1), resp.Results[0].SequenceNumber)
		require.Equal(t, int64(2), resp.Results[1].SequenceNumber)
	})

	t.Run("SeqBefore", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		session, insertLogs := setupBoundarySession(t, db)
		insertLogs(
			logOpt{SeqNum: 0, Allowed: true, Proto: "http", Method: "GET", Detail: "https://a.com"},
			logOpt{SeqNum: 1, Allowed: true, Proto: "http", Method: "GET", Detail: "https://b.com"},
			logOpt{SeqNum: 2, Allowed: true, Proto: "http", Method: "GET", Detail: "https://c.com"},
		)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		before := int64(2)
		resp, err := client.BoundarySessionLogs(ctx, session.ID, codersdk.BoundarySessionLogsParams{
			SeqBefore: &before,
		})
		require.NoError(t, err)
		require.Len(t, resp.Results, 2)
		require.Equal(t, int64(0), resp.Results[0].SequenceNumber)
		require.Equal(t, int64(1), resp.Results[1].SequenceNumber)
	})

	t.Run("BetweenTwoInterceptions", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		// Simulate the RFC scenario: two AI Bridge interceptions at
		// sequence numbers 5 and 12. The boundary events between them
		// (6, 7, 11) are the ones the frontend needs to fetch.
		session, insertLogs := setupBoundarySession(t, db)
		insertLogs(
			// Interception at seq 5 (the prompt itself).
			logOpt{SeqNum: 5, Allowed: true, Proto: "http", Method: "POST", Detail: "https://api.anthropic.com/v1/messages", Rule: "domain=api.anthropic.com"},
			// Agentic loop events.
			logOpt{SeqNum: 6, Allowed: true, Proto: "http", Method: "GET", Detail: "https://github.com/coder/coder/pulls", Rule: "domain=github.com"},
			logOpt{SeqNum: 7, Allowed: false, Proto: "http", Method: "GET", Detail: "https://evil.com/exfil"},
			logOpt{SeqNum: 11, Allowed: true, Proto: "http", Method: "POST", Detail: "https://api.anthropic.com/v1/messages", Rule: "domain=api.anthropic.com"},
			// Next interception at seq 12.
			logOpt{SeqNum: 12, Allowed: true, Proto: "http", Method: "POST", Detail: "https://api.anthropic.com/v1/messages", Rule: "domain=api.anthropic.com"},
		)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Query for events strictly between the two interceptions.
		after := int64(5)
		before := int64(12)
		resp, err := client.BoundarySessionLogs(ctx, session.ID, codersdk.BoundarySessionLogsParams{
			SeqAfter:  &after,
			SeqBefore: &before,
		})
		require.NoError(t, err)
		require.Len(t, resp.Results, 3, "should return events at seq 6, 7, 11")
		require.Equal(t, int64(6), resp.Results[0].SequenceNumber)
		require.Equal(t, int64(7), resp.Results[1].SequenceNumber)
		require.Equal(t, int64(11), resp.Results[2].SequenceNumber)
	})

	t.Run("Limit", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		session, insertLogs := setupBoundarySession(t, db)
		insertLogs(
			logOpt{SeqNum: 0, Allowed: true, Proto: "http", Method: "GET", Detail: "https://a.com"},
			logOpt{SeqNum: 1, Allowed: true, Proto: "http", Method: "GET", Detail: "https://b.com"},
			logOpt{SeqNum: 2, Allowed: true, Proto: "http", Method: "GET", Detail: "https://c.com"},
		)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		limit := int32(2)
		resp, err := client.BoundarySessionLogs(ctx, session.ID, codersdk.BoundarySessionLogsParams{
			Limit: &limit,
		})
		require.NoError(t, err)
		require.Len(t, resp.Results, 2)
		require.Equal(t, int64(0), resp.Results[0].SequenceNumber)
		require.Equal(t, int64(1), resp.Results[1].SequenceNumber)
	})

	t.Run("EmptySession", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		session, _ := setupBoundarySession(t, db)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.BoundarySessionLogs(ctx, session.ID, codersdk.BoundarySessionLogsParams{})
		require.NoError(t, err)
		require.Empty(t, resp.Results)
	})

	t.Run("InvalidSessionID", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// A well-formed UUID that has no corresponding session should
		// return an empty result set, not an error.
		resp, err := client.BoundarySessionLogs(ctx, uuid.New(), codersdk.BoundarySessionLogsParams{})
		require.NoError(t, err)
		require.Empty(t, resp.Results)
	})

	t.Run("MemberForbidden", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)

		// Create a regular member, who should not have access.
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := memberClient.BoundarySessionLogs(ctx, uuid.New(), codersdk.BoundarySessionLogsParams{})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, 403, sdkErr.StatusCode())
	})

	t.Run("AuditorAllowed", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)

		auditorClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleAuditor())

		session, insertLogs := setupBoundarySession(t, db)
		insertLogs(
			logOpt{SeqNum: 0, Allowed: true, Proto: "http", Method: "GET", Detail: "https://a.com"},
		)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := auditorClient.BoundarySessionLogs(ctx, session.ID, codersdk.BoundarySessionLogsParams{})
		require.NoError(t, err)
		require.Len(t, resp.Results, 1)
	})
}
