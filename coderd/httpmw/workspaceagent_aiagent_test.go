package httpmw_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type workspaceAgentAIFixture struct {
	db        database.Store
	token     uuid.UUID
	owner     database.User
	workspace database.WorkspaceTable
	agent     database.WorkspaceAgent
	agentUser database.User
	siteID    uuid.UUID
}

func newWorkspaceAgentAIFixture(t *testing.T) workspaceAgentAIFixture {
	t.Helper()

	db, _ := dbtestutil.NewDB(t)
	organization := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{Status: database.UserStatusActive})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: organization.ID,
		UserID:         owner.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: organization.ID,
		CreatedBy:      owner.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  organization.ID,
		ActiveVersionID: templateVersion.ID,
		CreatedBy:       owner.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        owner.ID,
		OrganizationID: organization.ID,
		TemplateID:     template.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: organization.ID,
		JobStatus:      database.ProvisionerJobStatusSucceeded,
		StartedAt:      sql.NullTime{Time: dbtime.Now(), Valid: true},
		CompletedAt:    sql.NullTime{Time: dbtime.Now(), Valid: true},
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: job.ID})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       workspace.ID,
		JobID:             job.ID,
		TemplateVersionID: templateVersion.ID,
		BuildNumber:       1,
		Transition:        database.WorkspaceTransitionStart,
	})
	token := uuid.New()
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID:  resource.ID,
		AuthToken:   token,
		APIKeyScope: database.AgentKeyScopeEnumAll,
	})

	return workspaceAgentAIFixture{
		db:        db,
		token:     token,
		owner:     owner,
		workspace: workspace,
		agent:     agent,
	}
}

func newBoundWorkspaceAgentAIFixture(t *testing.T) workspaceAgentAIFixture {
	t.Helper()

	fixture := newWorkspaceAgentAIFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	siteID := uuid.New()
	agentUser, err := aiagentidentity.Create(ctx, fixture.db, aiagentidentity.CreateParams{
		OwnerID:        fixture.owner.ID,
		OrganizationID: fixture.workspace.OrganizationID,
		OriginType:     entity.CreationSiteTypeWorkspace,
		OriginID:       siteID,
	})
	require.NoError(t, err)
	_, err = fixture.db.UpdateWorkspaceAgentAIAgentID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAgentAIAgentIDParams{
		ID:        fixture.agent.ID,
		AIAgentID: uuid.NullUUID{UUID: agentUser.ID, Valid: true},
	})
	require.NoError(t, err)
	fixture.agentUser = agentUser
	fixture.siteID = siteID
	return fixture
}

func serveWorkspaceAgent(t *testing.T, fixture workspaceAgentAIFixture, handler http.Handler) int {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(codersdk.SessionTokenHeader, fixture.token.String())
	recorder := httptest.NewRecorder()
	rw := &tracing.StatusWriter{ResponseWriter: recorder}
	httpmw.ExtractWorkspaceAgentAndLatestBuild(httpmw.ExtractWorkspaceAgentAndLatestBuildConfig{
		DB: fixture.db,
	})(handler).ServeHTTP(rw, req)
	return recorder.Code
}

func TestWorkspaceAgentAIBinding(t *testing.T) {
	t.Parallel()

	t.Run("Bound", func(t *testing.T) {
		t.Parallel()
		fixture := newBoundWorkspaceAgentAIFixture(t)

		var subject rbac.Subject
		status := serveWorkspaceAgent(t, fixture, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			subject = httpmw.UserAuthorization(r.Context())
			actor, ok := aiagentidentity.ActorFromContext(r.Context())
			require.True(t, ok)
			require.Equal(t, fixture.agentUser.ID, actor.AgentUserID)
			require.Equal(t, fixture.owner.ID, actor.OwnerUserID)
			require.Equal(t, entity.CreationSiteTypeWorkspace, actor.OriginType)
			require.Equal(t, fixture.siteID, actor.OriginID)
			rw.WriteHeader(http.StatusNoContent)
		}))
		require.Equal(t, http.StatusNoContent, status)
		require.Equal(t, fixture.owner.ID.String(), subject.ID)
		require.Equal(t, rbac.SubjectTypeAIAgent, subject.Type)
		require.Equal(t, fixture.agentUser.Username, subject.FriendlyName)

		authorizer := rbac.NewAuthorizer(prometheus.NewRegistry())
		userResource := rbac.ResourceUser.WithOwner(fixture.owner.ID.String()).WithID(fixture.owner.ID)
		err := authorizer.Authorize(context.Background(), subject, policy.ActionReadPersonal, userResource)
		require.Error(t, err, "bound agents must force no_user_data even when api_key_scope is all")
	})

	t.Run("Unbound", func(t *testing.T) {
		t.Parallel()
		fixture := newWorkspaceAgentAIFixture(t)

		var subject rbac.Subject
		status := serveWorkspaceAgent(t, fixture, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			subject = httpmw.UserAuthorization(r.Context())
			_, ok := aiagentidentity.ActorFromContext(r.Context())
			require.False(t, ok)
			rw.WriteHeader(http.StatusNoContent)
		}))
		require.Equal(t, http.StatusNoContent, status)
		require.Equal(t, fixture.owner.ID.String(), subject.ID)
		require.Equal(t, rbac.SubjectTypeUser, subject.Type)

		authorizer := rbac.NewAuthorizer(prometheus.NewRegistry())
		userResource := rbac.ResourceUser.WithOwner(fixture.owner.ID.String()).WithID(fixture.owner.ID)
		require.NoError(t, authorizer.Authorize(context.Background(), subject, policy.ActionReadPersonal, userResource))
	})
}

func TestWorkspaceAgentAIAuditAttribution(t *testing.T) {
	t.Parallel()

	fixture := newBoundWorkspaceAgentAIFixture(t)
	auditor := audit.NewMock()
	status := serveWorkspaceAgent(t, fixture, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		r = r.WithContext(httpmw.WithRequestID(r.Context(), uuid.New()))
		aReq, commitAudit := audit.InitRequest[database.User](rw, &audit.RequestParams{
			Audit:   auditor,
			Log:     slogtest.Make(t, nil),
			Request: r,
			Action:  database.AuditActionCreate,
		})
		aReq.New = database.User{ID: uuid.New(), Username: "workspace-agent-audit-target-" + uuid.NewString()}
		rw.WriteHeader(http.StatusCreated)
		commitAudit()
	}))
	require.Equal(t, http.StatusCreated, status)

	logs := auditor.AuditLogs()
	require.Len(t, logs, 1)
	require.Equal(t, fixture.agentUser.ID, logs[0].UserID)
	require.True(t, logs[0].OnBehalfOfUserID.Valid)
	require.Equal(t, fixture.owner.ID, logs[0].OnBehalfOfUserID.UUID)
}

func TestWorkspaceAgentAIBindingLiveness(t *testing.T) {
	t.Parallel()

	t.Run("IdentityDeleted", func(t *testing.T) {
		t.Parallel()
		fixture := newBoundWorkspaceAgentAIFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		// Revocation retires the agent in the ledger, which is what
		// resolution reads.
		require.NoError(t, entity.RetireAIAgent(dbauthz.AsSystemRestricted(ctx), fixture.db, fixture.agentUser.ID,
			entity.EventAIAgentKill, entity.Ref{Type: entity.TypeUser, ID: fixture.owner.ID},
			dbtime.Now()))
		require.Equal(t, http.StatusUnauthorized, serveWorkspaceAgent(t, fixture, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("deleted AI identity reached handler")
		})))
	})

	t.Run("OwnerSuspended", func(t *testing.T) {
		t.Parallel()
		fixture := newBoundWorkspaceAgentAIFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := fixture.db.UpdateUserStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateUserStatusParams{
			ID:         fixture.owner.ID,
			Status:     database.UserStatusSuspended,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, serveWorkspaceAgent(t, fixture, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("suspended AI agent owner reached handler")
		})))
	})

	t.Run("OwnerDeleted", func(t *testing.T) {
		t.Parallel()
		fixture := newBoundWorkspaceAgentAIFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		require.NoError(t, fixture.db.UpdateUserDeletedByID(dbauthz.AsSystemRestricted(ctx), fixture.owner.ID))
		require.Equal(t, http.StatusUnauthorized, serveWorkspaceAgent(t, fixture, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("deleted AI agent owner reached handler")
		})))
	})
}
