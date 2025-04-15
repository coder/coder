package idpsync_test

import (
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/testutil"
)

func TestParseOrganizationClaims(t *testing.T) {
	t.Parallel()

	t.Run("AGPL", func(t *testing.T) {
		t.Parallel()

		// AGPL has limited behavior
		s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{}),
			runtimeconfig.NewManager(),
			idpsync.DeploymentSyncSettings{
				OrganizationField: "orgs",
				OrganizationMapping: map[string][]uuid.UUID{
					"random": {uuid.New()},
				},
				OrganizationAssignDefault: false,
			})

		ctx := testutil.Context(t, testutil.WaitMedium)

		params, err := s.ParseOrganizationClaims(ctx, jwt.MapClaims{})
		require.Nil(t, err)

		require.False(t, params.SyncEntitled)
	})
}

func TestSyncOrganizations(t *testing.T) {
	t.Parallel()

	t.Run("SyncUserToDeletedOrg", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)

		// Create a new organization, add in the user as a member, then delete
		// the org.
		org := dbgen.Organization(t, db, database.Organization{})
		user := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			Roles:          nil,
		})

		err := db.UpdateOrganizationDeletedByID(ctx, database.UpdateOrganizationDeletedByIDParams{
			UpdatedAt: dbtime.Now(),
			ID:        org.ID,
		})
		require.NoError(t, err)

		// Now sync the user to the deleted organization
		s := idpsync.NewAGPLSync(
			slogtest.Make(t, &slogtest.Options{}),
			runtimeconfig.NewManager(),
			idpsync.DeploymentSyncSettings{
				OrganizationField: "orgs",
				OrganizationMapping: map[string][]uuid.UUID{
					"random": {org.ID},
				},
				OrganizationAssignDefault: false,
			},
		)

		err = s.SyncOrganizations(ctx, db, user, idpsync.OrganizationParams{
			SyncEntitled: true,
			MergedClaims: map[string]interface{}{
				"orgs": []string{"random"},
			},
		})
		require.NoError(t, err)

		mems, err := db.OrganizationMembers(ctx, database.OrganizationMembersParams{
			OrganizationID: org.ID,
			UserID:         user.ID,
			IncludeSystem:  false,
		})
		require.NoError(t, err)
		require.Len(t, mems, 1)
	})
}
