package coderd_test

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestAgentFirewallSessionByID(t *testing.T) {
	t.Parallel()

	// seedBoundarySession inserts a boundary session linked to a workspace agent.
	// Uses the raw DB store to avoid dbauthz permission and ordering constraints during setup.
	seedBoundarySession := func(t *testing.T, rawDB database.Store, ownerID, orgID uuid.UUID) (database.BoundarySession, database.WorkspaceTable) {
		t.Helper()

		resp := dbfake.WorkspaceBuild(t, rawDB, database.WorkspaceTable{
			OwnerID:        ownerID,
			OrganizationID: orgID,
		}).WithAgent().Do()

		require.NotEmpty(t, resp.Agents, "expected at least one agent")

		session := dbgen.BoundarySession(t, rawDB, database.BoundarySession{
			WorkspaceAgentID:    resp.Agents[0].ID,
			OwnerID:             uuid.NullUUID{UUID: ownerID, Valid: true},
			ConfinedProcessName: "claude-code",
		})
		return session, resp.Workspace
	}

	t.Run("Owner", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		ownerClient, _, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   pubsub,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureBoundary: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		session, ws := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID)

		//nolint:gocritic // Testing owner role.
		got, err := ownerClient.AgentFirewallSessionByID(ctx, session.ID)
		require.NoError(t, err)
		require.Equal(t, session.ID, got.ID)
		require.Equal(t, ws.OwnerID, got.OwnerID)
		require.Equal(t, session.ConfinedProcessName, got.ConfinedProcess)
		require.Equal(t, ws.ID, got.WorkspaceID)
	})

	t.Run("Auditor", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		ownerClient, _, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   pubsub,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureBoundary: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		session, _ := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID)

		auditorClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleAuditor())

		got, err := auditorClient.AgentFirewallSessionByID(ctx, session.ID)
		require.NoError(t, err)
		require.Equal(t, session.ID, got.ID)
		require.Equal(t, "claude-code", got.ConfinedProcess)
	})

	t.Run("MemberDenied", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		ownerClient, _, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   pubsub,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureBoundary: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		session, _ := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID)

		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		_, err := memberClient.AgentFirewallSessionByID(ctx, session.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureBoundary: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Testing owner role.
		_, err := ownerClient.AgentFirewallSessionByID(ctx, uuid.New())
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

// TestInsertBoundaryLogs_AgentAuth verifies that a workspace agent context
// can insert boundary logs through the dbauthz layer. Create is user-scoped
// in the member role; the agent's owner ID must match the resource owner.
func TestInsertBoundaryLogs_AgentAuth(t *testing.T) {
	t.Parallel()

	rawDB, _ := dbtestutil.NewDB(t)
	authorizer := rbac.NewStrictAuthorizer(prometheus.NewRegistry())
	authzDB := dbauthz.New(rawDB, authorizer, slogtest.Make(t, nil), &atomic.Pointer[dbauthz.AccessControlStore]{})

	ctx := testutil.Context(t, testutil.WaitLong)

	// Seed a workspace with an agent.
	user := dbgen.User(t, rawDB, database.User{})
	org := dbgen.Organization(t, rawDB, database.Organization{})
	_ = dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	tmpl := dbgen.Template(t, rawDB, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tmplVer := dbgen.TemplateVersion(t, rawDB, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{Valid: true, UUID: tmpl.ID},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	ws := dbgen.Workspace(t, rawDB, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     tmpl.ID,
		OwnerID:        user.ID,
	})
	job := dbgen.ProvisionerJob(t, rawDB, nil, database.ProvisionerJob{
		Type: database.ProvisionerJobTypeWorkspaceBuild,
	})
	build := dbgen.WorkspaceBuild(t, rawDB, database.WorkspaceBuild{
		JobID:             job.ID,
		WorkspaceID:       ws.ID,
		TemplateVersionID: tmplVer.ID,
	})
	resource := dbgen.WorkspaceResource(t, rawDB, database.WorkspaceResource{
		JobID: build.JobID,
	})
	agent := dbgen.WorkspaceAgent(t, rawDB, database.WorkspaceAgent{
		ResourceID: resource.ID,
	})

	// Insert a boundary session using the raw DB (no auth check).
	now := time.Now().UTC()
	sessionID := uuid.New()
	_, err := rawDB.InsertBoundarySession(ctx, database.InsertBoundarySessionParams{
		ID:                  sessionID,
		WorkspaceAgentID:    agent.ID,
		OwnerID:             uuid.NullUUID{UUID: user.ID, Valid: true},
		ConfinedProcessName: "claude-code",
		StartedAt:           now,
		UpdatedAt:           now,
	})
	require.NoError(t, err)

	// Build a workspace agent RBAC subject.
	memberRole, err := rbac.RoleByName(rbac.RoleMember())
	require.NoError(t, err)
	agentSubject := rbac.Subject{
		ID:    user.ID.String(),
		Roles: rbac.Roles{memberRole},
		Scope: rbac.WorkspaceAgentScope(rbac.WorkspaceAgentScopeParams{
			WorkspaceID: ws.ID,
			OwnerID:     user.ID,
			TemplateID:  tmpl.ID,
			VersionID:   tmplVer.ID,
		}),
	}.WithCachedASTValue()
	agentCtx := dbauthz.As(ctx, agentSubject)

	// Insert boundary logs through dbauthz with the correct owner.
	// User-scoped create succeeds because the agent subject ID matches.
	logID := uuid.New()
	_, err = authzDB.InsertBoundaryLogs(agentCtx, database.InsertBoundaryLogsParams{
		SessionID:      sessionID,
		OwnerID:        user.ID,
		ID:             []uuid.UUID{logID},
		SequenceNumber: []int32{1},
		CapturedAt:     []time.Time{now},
		CreatedAt:      []time.Time{now},
		Proto:          []string{"tcp"},
		Method:         []string{"connect"},
		Detail:         []string{"example.com:443"},
		MatchedRule:    []string{"allow-all"},
	})
	require.NoError(t, err, "agent should be able to insert boundary logs for own owner")

	// Verify the logs were actually persisted.
	got, err := rawDB.GetBoundaryLogByID(ctx, logID)
	require.NoError(t, err)
	require.Equal(t, sessionID, got.SessionID)

	// Inserting with a different owner ID must fail (user-scoped create).
	otherUser := dbgen.User(t, rawDB, database.User{})
	_, err = authzDB.InsertBoundaryLogs(agentCtx, database.InsertBoundaryLogsParams{
		SessionID:      sessionID,
		OwnerID:        otherUser.ID,
		ID:             []uuid.UUID{uuid.New()},
		SequenceNumber: []int32{2},
		CapturedAt:     []time.Time{now},
		CreatedAt:      []time.Time{now},
		Proto:          []string{"tcp"},
		Method:         []string{"connect"},
		Detail:         []string{"evil.com:443"},
		MatchedRule:    []string{"allow-all"},
	})
	require.Error(t, err, "agent must not insert boundary logs for a different owner")
}
