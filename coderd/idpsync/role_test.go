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
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/testutil"
)

func TestRoleSyncTable(t *testing.T) {
	t.Parallel()

	// Last checked, takes 30s with postgres on a fast machine.
	if dbtestutil.WillUsePostgres() {
		t.Skip("Skipping test because it populates a lot of db entries, which is slow on postgres.")
	}

	userClaims := jwt.MapClaims{
		"roles": []string{
			"foo", "bar", "baz",
			"create-bar", "create-baz",
			"legacy-bar", rbac.RoleOrgAuditor(),
		},
		// bad-claim is a number, and will fail any role sync
		"bad-claim": 100,
	}

	//ids := coderdtest.NewDeterministicUUIDGenerator()
	testCases := []orgSetupDefinition{
		{
			Name:              "NoSync",
			OrganizationRoles: []string{},
			assertRoles: &orgRoleAssert{
				ExpectedOrgRoles: []string{},
			},
		},
		{
			Name: "SyncDisabled",
			OrganizationRoles: []string{
				rbac.RoleOrgAdmin(),
			},
			RoleSettings: &idpsync.RoleSyncSettings{},
			assertRoles: &orgRoleAssert{
				ExpectedOrgRoles: []string{
					rbac.RoleOrgAdmin(),
				},
			},
		},
		{
			// Audit role from claim
			Name: "RawAudit",
			OrganizationRoles: []string{
				rbac.RoleOrgAdmin(),
			},
			RoleSettings: &idpsync.RoleSyncSettings{
				Field:   "roles",
				Mapping: map[string][]string{},
			},
			assertRoles: &orgRoleAssert{
				ExpectedOrgRoles: []string{
					rbac.RoleOrgAuditor(),
				},
			},
		},
		{
			Name: "CustomRole",
			OrganizationRoles: []string{
				rbac.RoleOrgAdmin(),
			},
			CustomRoles: []string{"foo"},
			RoleSettings: &idpsync.RoleSyncSettings{
				Field:   "roles",
				Mapping: map[string][]string{},
			},
			assertRoles: &orgRoleAssert{
				ExpectedOrgRoles: []string{
					rbac.RoleOrgAuditor(),
					"foo",
				},
			},
		},
		{
			Name: "RoleMapping",
			OrganizationRoles: []string{
				rbac.RoleOrgAdmin(),
				"invalid", // Throw in an extra invalid role that will be removed
			},
			CustomRoles: []string{"custom"},
			RoleSettings: &idpsync.RoleSyncSettings{
				Field: "roles",
				Mapping: map[string][]string{
					"foo": {"custom", rbac.RoleOrgTemplateAdmin()},
				},
			},
			assertRoles: &orgRoleAssert{
				ExpectedOrgRoles: []string{
					rbac.RoleOrgAuditor(),
					rbac.RoleOrgTemplateAdmin(),
					"custom",
				},
			},
		},
		{
			// InvalidClaims will log an error, but do not block authentication.
			// This is to prevent a misconfigured organization from blocking
			// a user from authenticating.
			Name:              "InvalidClaim",
			OrganizationRoles: []string{rbac.RoleOrgAdmin()},
			RoleSettings: &idpsync.RoleSyncSettings{
				Field: "bad-claim",
			},
			assertRoles: &orgRoleAssert{
				ExpectedOrgRoles: []string{
					rbac.RoleOrgAdmin(),
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			manager := runtimeconfig.NewManager()
			s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{
				IgnoreErrors: true,
			}),
				manager,
				idpsync.DeploymentSyncSettings{
					SiteRoleField: "roles",
				},
			)

			ctx := testutil.Context(t, testutil.WaitSuperLong)
			user := dbgen.User(t, db, database.User{})
			orgID := uuid.New()
			SetupOrganization(t, s, db, user, orgID, tc)

			// Do the group sync!
			err := s.SyncRoles(ctx, db, user, idpsync.RoleParams{
				SyncEnabled:  true,
				SyncSiteWide: false,
				MergedClaims: userClaims,
			})
			require.NoError(t, err)

			tc.Assert(t, orgID, db, user)
		})
	}

	// AllTogether runs the entire tabled test as a singular user and
	// deployment. This tests all organizations being synced together.
	// The reason we do them individually, is that it is much easier to
	// debug a single test case.
	//t.Run("AllTogether", func(t *testing.T) {
	//	t.Parallel()
	//
	//	db, _ := dbtestutil.NewDB(t)
	//	manager := runtimeconfig.NewManager()
	//	s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{}),
	//		manager,
	//		// Also sync the default org!
	//		idpsync.DeploymentSyncSettings{
	//			GroupField: "groups",
	//			Legacy: idpsync.DefaultOrgLegacySettings{
	//				GroupField: "groups",
	//				GroupMapping: map[string]string{
	//					"foo": "legacy-foo",
	//					"baz": "legacy-baz",
	//				},
	//				GroupFilter:         regexp.MustCompile("^legacy"),
	//				CreateMissingGroups: true,
	//			},
	//		},
	//	)
	//
	//	ctx := testutil.Context(t, testutil.WaitSuperLong)
	//	user := dbgen.User(t, db, database.User{})
	//
	//	var asserts []func(t *testing.T)
	//	// The default org is also going to do something
	//	def := orgSetupDefinition{
	//		Name: "DefaultOrg",
	//		GroupNames: map[string]bool{
	//			"legacy-foo": false,
	//			"legacy-baz": true,
	//			"random":     true,
	//		},
	//		// No settings, because they come from the deployment values
	//		GroupSettings: nil,
	//		assertGroups: &orgGroupAssert{
	//			ExpectedGroupNames: []string{"legacy-foo", "legacy-baz", "legacy-bar"},
	//		},
	//	}
	//
	//	//nolint:gocritic // testing
	//	defOrg, err := db.GetDefaultOrganization(dbauthz.AsSystemRestricted(ctx))
	//	require.NoError(t, err)
	//	SetupOrganization(t, s, db, user, defOrg.ID, def)
	//	asserts = append(asserts, func(t *testing.T) {
	//		t.Run(def.Name, func(t *testing.T) {
	//			t.Parallel()
	//			def.Assert(t, defOrg.ID, db, user)
	//		})
	//	})
	//
	//	for _, tc := range testCases {
	//		tc := tc
	//
	//		orgID := uuid.New()
	//		SetupOrganization(t, s, db, user, orgID, tc)
	//		asserts = append(asserts, func(t *testing.T) {
	//			t.Run(tc.Name, func(t *testing.T) {
	//				t.Parallel()
	//				tc.Assert(t, orgID, db, user)
	//			})
	//		})
	//	}
	//
	//	asserts = append(asserts, func(t *testing.T) {
	//		t.Helper()
	//		def.Assert(t, defOrg.ID, db, user)
	//	})
	//
	//	// Do the group sync!
	//	err = s.SyncGroups(ctx, db, user, idpsync.GroupParams{
	//		SyncEnabled:  true,
	//		MergedClaims: userClaims,
	//	})
	//	require.NoError(t, err)
	//
	//	for _, assert := range asserts {
	//		assert(t)
	//	}
	//})
}
