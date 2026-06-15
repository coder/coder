package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestBoundarySessionByID(t *testing.T) {
	t.Parallel()

	// seedBoundarySession creates the workspace -> build -> resource ->
	// agent chain via dbfake, then inserts a boundary session linked to
	// that agent. It returns the session.
	//
	// The raw (unwrapped) database store is used for seeding because:
	//  1. dbgen.ProvisionerJob reads the job back through dbauthz, which
	//     calls authorizeProvisionerJob and looks up the workspace build
	//     before it exists.
	//  2. The owner role only has read permission on boundary_log, so
	//     InsertBoundarySession fails through dbauthz.
	seedBoundarySession := func(t *testing.T, rawDB database.Store, ownerID, orgID uuid.UUID) database.BoundarySession {
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
		return session
	}

	t.Run("Owner", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		ownerClient, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
			Database: db,
			Pubsub:   pubsub,
		})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		session := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID)

		// Owner should be able to read the session.
		//nolint:gocritic // Testing owner role.
		got, err := ownerClient.BoundarySessionByID(ctx, session.ID)
		require.NoError(t, err)
		require.Equal(t, session.ID, got.ID)
		require.Equal(t, owner.UserID, got.OwnerID)
		require.Equal(t, session.ConfinedProcessName, got.ConfinedProcess)
		require.NotEqual(t, uuid.Nil, got.WorkspaceID)
	})

	t.Run("Auditor", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		ownerClient, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
			Database: db,
			Pubsub:   pubsub,
		})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		session := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID)

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

		db, pubsub := dbtestutil.NewDB(t)
		ownerClient, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
			Database: db,
			Pubsub:   pubsub,
		})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		session := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID)

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

	db, pubsub := dbtestutil.NewDB(t)
	ownerClient, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Seed data through the raw database to avoid dbauthz ordering and
	// permission issues during setup.
	resp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        owner.UserID,
		OrganizationID: owner.OrganizationID,
	}).WithAgent().Do()

	require.NotEmpty(t, resp.Agents, "expected at least one agent")

	session := dbgen.BoundarySession(t, db, database.BoundarySession{
		WorkspaceAgentID:    resp.Agents[0].ID,
		OwnerID:             uuid.NullUUID{UUID: owner.UserID, Valid: true},
		ConfinedProcessName: "codex",
	})

	// AsSystemRestricted should succeed (read permission).
	got, err := api.Database.GetBoundarySessionByID(dbauthz.AsSystemRestricted(ctx), session.ID)
	require.NoError(t, err)
	require.Equal(t, session.ID, got.ID)
}
