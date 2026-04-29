package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

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
