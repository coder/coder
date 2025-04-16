package idpsync_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/testutil"
)

func TestFromLegacySettings(t *testing.T) {
	t.Parallel()

	legacy := func(assignDefault bool) string {
		return fmt.Sprintf(`{
		   "Field":"groups",
		   "Mapping":{
			  "engineering":[
				 "10b2bd19-f5ca-4905-919f-bf02e95e3b6a"
			  ]
		   },
		   "AssignDefault":%t
		}`, assignDefault)
	}

	t.Run("AssignDefault,True", func(t *testing.T) {
		t.Parallel()

		var settings idpsync.OrganizationSyncSettings
		settings.AssignDefault = true
		err := settings.Set(legacy(true))
		require.NoError(t, err)

		require.Equal(t, settings.Field, "groups", "field")
		require.Equal(t, settings.Mapping, map[string][]uuid.UUID{
			"engineering": {
				uuid.MustParse("10b2bd19-f5ca-4905-919f-bf02e95e3b6a"),
			},
		}, "mapping")
		require.True(t, settings.AssignDefault, "assign default")
	})

	t.Run("AssignDefault,False", func(t *testing.T) {
		t.Parallel()

		var settings idpsync.OrganizationSyncSettings
		settings.AssignDefault = true
		err := settings.Set(legacy(false))
		require.NoError(t, err)

		require.Equal(t, settings.Field, "groups", "field")
		require.Equal(t, settings.Mapping, map[string][]uuid.UUID{
			"engineering": {
				uuid.MustParse("10b2bd19-f5ca-4905-919f-bf02e95e3b6a"),
			},
		}, "mapping")
		require.False(t, settings.AssignDefault, "assign default")
	})

	t.Run("CorrectAssign", func(t *testing.T) {
		t.Parallel()

		var settings idpsync.OrganizationSyncSettings
		settings.AssignDefault = true
		err := settings.Set(legacy(false))
		require.NoError(t, err)

		require.Equal(t, settings.Field, "groups", "field")
		require.Equal(t, settings.Mapping, map[string][]uuid.UUID{
			"engineering": {
				uuid.MustParse("10b2bd19-f5ca-4905-919f-bf02e95e3b6a"),
			},
		}, "mapping")
		require.False(t, settings.AssignDefault, "assign default")
	})
}

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

	// This test creates some deleted organizations and checks the behavior is
	// correct.
	t.Run("SyncUserToDeletedOrg", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})

		// Create orgs for:
		//  - stays  = User is a member, and stays
		//  - leaves = User is a member, and leaves
		//  - joins  = User is not a member, and joins
		// For deleted orgs, the user **should not** be a member of afterwards.
		//  - deletedStays  = User is a member of deleted org, and wants to stay
		//  - deletedLeaves = User is a member of deleted org, and wants to leave
		//  - deletedJoins  = User is not a member of deleted org, and wants to join
		stays := dbfake.Organization(t, db).Members(user).Do()
		leaves := dbfake.Organization(t, db).Members(user).Do()
		joins := dbfake.Organization(t, db).Do()

		deletedStays := dbfake.Organization(t, db).Members(user).Deleted(true).Do()
		deletedLeaves := dbfake.Organization(t, db).Members(user).Deleted(true).Do()
		deletedJoins := dbfake.Organization(t, db).Deleted(true).Do()

		// Now sync the user to the deleted organization
		s := idpsync.NewAGPLSync(
			slogtest.Make(t, &slogtest.Options{}),
			runtimeconfig.NewManager(),
			idpsync.DeploymentSyncSettings{
				OrganizationField: "orgs",
				OrganizationMapping: map[string][]uuid.UUID{
					"stay":  {stays.Org.ID, deletedStays.Org.ID},
					"leave": {leaves.Org.ID, deletedLeaves.Org.ID},
					"join":  {joins.Org.ID, deletedJoins.Org.ID},
				},
				OrganizationAssignDefault: false,
			},
		)

		err := s.SyncOrganizations(ctx, db, user, idpsync.OrganizationParams{
			SyncEntitled: true,
			MergedClaims: map[string]interface{}{
				"orgs": []string{"stay", "join"},
			},
		})
		require.NoError(t, err)

		orgs, err := db.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
			UserID:  user.ID,
			Deleted: sql.NullBool{},
		})
		require.NoError(t, err)
		require.Len(t, orgs, 2)

		// Verify the user only exists in 2 orgs. The one they stayed, and the one they
		// joined.
		inIDs := db2sdk.List(orgs, func(org database.Organization) uuid.UUID {
			return org.ID
		})
		require.ElementsMatch(t, []uuid.UUID{stays.Org.ID, joins.Org.ID}, inIDs)
	})

	t.Run("UserToZeroOrgs", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})

		deletedLeaves := dbfake.Organization(t, db).Members(user).Deleted(true).Do()

		// Now sync the user to the deleted organization
		s := idpsync.NewAGPLSync(
			slogtest.Make(t, &slogtest.Options{}),
			runtimeconfig.NewManager(),
			idpsync.DeploymentSyncSettings{
				OrganizationField: "orgs",
				OrganizationMapping: map[string][]uuid.UUID{
					"leave": {deletedLeaves.Org.ID},
				},
				OrganizationAssignDefault: false,
			},
		)

		err := s.SyncOrganizations(ctx, db, user, idpsync.OrganizationParams{
			SyncEntitled: true,
			MergedClaims: map[string]interface{}{
				"orgs": []string{},
			},
		})
		require.NoError(t, err)

		orgs, err := db.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
			UserID:  user.ID,
			Deleted: sql.NullBool{},
		})
		require.NoError(t, err)
		require.Len(t, orgs, 0)
	})
}
