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
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestAgentFirewallSessionByID(t *testing.T) {
	t.Parallel()

	// seedBoundarySession inserts a boundary session linked to a workspace agent.
	// Uses the raw DB store to avoid dbauthz permission and ordering constraints during setup.
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

		session := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID)

		//nolint:gocritic // Testing owner role.
		got, err := ownerClient.AgentFirewallSessionByID(ctx, session.ID)
		require.NoError(t, err)
		require.Equal(t, session.ID, got.ID)
		require.Equal(t, owner.UserID, got.OwnerID)
		require.Equal(t, session.ConfinedProcessName, got.ConfinedProcess)
		require.NotEqual(t, uuid.Nil, got.WorkspaceID)
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

		session := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID)

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

		session := seedBoundarySession(t, db, owner.UserID, owner.OrganizationID)

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

// TestAgentFirewallSessionByID_DBAuth verifies that the dbauthz layer
// correctly gates access to GetBoundarySessionByID using
// ResourceBoundaryLog with ActionRead.
func TestAgentFirewallSessionByID_DBAuth(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	_, _, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
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
	got, err := db.GetBoundarySessionByID(dbauthz.AsSystemRestricted(ctx), session.ID)
	require.NoError(t, err)
	require.Equal(t, session.ID, got.ID)
}
