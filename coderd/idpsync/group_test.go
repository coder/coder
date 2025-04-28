package idpsync_test

import (
	"context"
	"database/sql"
	"regexp"
	"slices"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestParseGroupClaims(t *testing.T) {
	t.Parallel()

	t.Run("EmptyConfig", func(t *testing.T) {
		t.Parallel()

		s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{}),
			runtimeconfig.NewManager(),
			idpsync.DeploymentSyncSettings{})

		ctx := testutil.Context(t, testutil.WaitMedium)

		params, err := s.ParseGroupClaims(ctx, jwt.MapClaims{})
		require.Nil(t, err)

		require.False(t, params.SyncEntitled)
	})

	// AllowList has no effect in AGPL
	t.Run("AllowList", func(t *testing.T) {
		t.Parallel()

		s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{}),
			runtimeconfig.NewManager(),
			idpsync.DeploymentSyncSettings{
				GroupField: "groups",
				GroupAllowList: map[string]struct{}{
					"foo": {},
				},
			})

		ctx := testutil.Context(t, testutil.WaitMedium)

		params, err := s.ParseGroupClaims(ctx, jwt.MapClaims{})
		require.Nil(t, err)
		require.False(t, params.SyncEntitled)
	})
}

func TestGroupSyncTable(t *testing.T) {
	t.Parallel()

	// Last checked, takes 30s with postgres on a fast machine.
	if dbtestutil.WillUsePostgres() {
		t.Skip("Skipping test because it populates a lot of db entries, which is slow on postgres.")
	}

	userClaims := jwt.MapClaims{
		"groups": []string{
			"foo", "bar", "baz",
			"create-bar", "create-baz",
			"legacy-bar",
		},
	}

	ids := coderdtest.NewDeterministicUUIDGenerator()
	testCases := []orgSetupDefinition{
		{
			Name: "SwitchGroups",
			GroupSettings: &codersdk.GroupSyncSettings{
				Field: "groups",
				Mapping: map[string][]uuid.UUID{
					"foo": {ids.ID("sg-foo"), ids.ID("sg-foo-2")},
					"bar": {ids.ID("sg-bar")},
					"baz": {ids.ID("sg-baz")},
				},
			},
			Groups: map[uuid.UUID]bool{
				uuid.New(): true,
				uuid.New(): true,
				// Extra groups
				ids.ID("sg-foo"):   false,
				ids.ID("sg-foo-2"): false,
				ids.ID("sg-bar"):   false,
				ids.ID("sg-baz"):   false,
			},
			assertGroups: &orgGroupAssert{
				ExpectedGroups: []uuid.UUID{
					ids.ID("sg-foo"),
					ids.ID("sg-foo-2"),
					ids.ID("sg-bar"),
					ids.ID("sg-baz"),
				},
			},
		},
		{
			Name: "StayInGroup",
			GroupSettings: &codersdk.GroupSyncSettings{
				Field: "groups",
				// Only match foo, so bar does not map
				RegexFilter: regexp.MustCompile("^foo$"),
				Mapping: map[string][]uuid.UUID{
					"foo": {ids.ID("gg-foo"), uuid.New()},
					"bar": {ids.ID("gg-bar")},
					"baz": {ids.ID("gg-baz")},
				},
			},
			Groups: map[uuid.UUID]bool{
				ids.ID("gg-foo"): true,
				ids.ID("gg-bar"): false,
			},
			assertGroups: &orgGroupAssert{
				ExpectedGroups: []uuid.UUID{
					ids.ID("gg-foo"),
				},
			},
		},
		{
			Name: "UserJoinsGroups",
			GroupSettings: &codersdk.GroupSyncSettings{
				Field: "groups",
				Mapping: map[string][]uuid.UUID{
					"foo": {ids.ID("ng-foo"), uuid.New()},
					"bar": {ids.ID("ng-bar"), ids.ID("ng-bar-2")},
					"baz": {ids.ID("ng-baz")},
				},
			},
			Groups: map[uuid.UUID]bool{
				ids.ID("ng-foo"):   false,
				ids.ID("ng-bar"):   false,
				ids.ID("ng-bar-2"): false,
				ids.ID("ng-baz"):   false,
			},
			assertGroups: &orgGroupAssert{
				ExpectedGroups: []uuid.UUID{
					ids.ID("ng-foo"),
					ids.ID("ng-bar"),
					ids.ID("ng-bar-2"),
					ids.ID("ng-baz"),
				},
			},
		},
		{
			Name: "CreateGroups",
			GroupSettings: &codersdk.GroupSyncSettings{
				Field:             "groups",
				RegexFilter:       regexp.MustCompile("^create"),
				AutoCreateMissing: true,
			},
			Groups: map[uuid.UUID]bool{},
			assertGroups: &orgGroupAssert{
				ExpectedGroupNames: []string{
					"create-bar",
					"create-baz",
				},
			},
		},
		{
			Name: "GroupNamesNoMapping",
			GroupSettings: &codersdk.GroupSyncSettings{
				Field:             "groups",
				RegexFilter:       regexp.MustCompile(".*"),
				AutoCreateMissing: false,
			},
			GroupNames: map[string]bool{
				"foo":  false,
				"bar":  false,
				"goob": true,
			},
			assertGroups: &orgGroupAssert{
				ExpectedGroupNames: []string{
					"foo",
					"bar",
				},
			},
		},
		{
			Name: "NoUser",
			GroupSettings: &codersdk.GroupSyncSettings{
				Field: "groups",
				Mapping: map[string][]uuid.UUID{
					// Extra ID that does not map to a group
					"foo": {ids.ID("ow-foo"), uuid.New()},
				},
				RegexFilter:       nil,
				AutoCreateMissing: false,
			},
			NotMember: true,
			Groups: map[uuid.UUID]bool{
				ids.ID("ow-foo"): false,
				ids.ID("ow-bar"): false,
			},
		},
		{
			Name:          "NoSettings",
			GroupSettings: nil,
			Groups:        map[uuid.UUID]bool{},
			assertGroups: &orgGroupAssert{
				ExpectedGroups: []uuid.UUID{},
			},
		},
		{
			Name: "LegacyMapping",
			GroupSettings: &codersdk.GroupSyncSettings{
				Field:       "groups",
				RegexFilter: regexp.MustCompile("^legacy"),
				LegacyNameMapping: map[string]string{
					"create-bar": "legacy-bar",
					"foo":        "legacy-foo",
					"bop":        "legacy-bop",
				},
				AutoCreateMissing: true,
			},
			Groups: map[uuid.UUID]bool{
				ids.ID("lg-foo"): true,
			},
			GroupNames: map[string]bool{
				"legacy-foo": false,
				"extra":      true,
				"legacy-bop": true,
			},
			assertGroups: &orgGroupAssert{
				ExpectedGroupNames: []string{
					"legacy-bar",
					"legacy-foo",
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		// The final test, "AllTogether", cannot run in parallel.
		// These tests are nearly instant using the memory db, so
		// this is still fast without being in parallel.
		//nolint:paralleltest, tparallel
		t.Run(tc.Name, func(t *testing.T) {

			db, _ := dbtestutil.NewDB(t)
			manager := runtimeconfig.NewManager()
			s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{}),
				manager,
				idpsync.DeploymentSyncSettings{
					GroupField: "groups",
					Legacy: idpsync.DefaultOrgLegacySettings{
						GroupField: "groups",
						GroupMapping: map[string]string{
							"foo": "legacy-foo",
							"baz": "legacy-baz",
						},
						GroupFilter:         regexp.MustCompile("^legacy"),
						CreateMissingGroups: true,
					},
				},
			)

			ctx := testutil.Context(t, testutil.WaitSuperLong)
			user := dbgen.User(t, db, database.User{})
			orgID := uuid.New()
			SetupOrganization(t, s, db, user, orgID, tc)

			// Do the group sync!
			err := s.SyncGroups(ctx, db, user, idpsync.GroupParams{
				SyncEntitled: true,
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
	//nolint:paralleltest, tparallel // This should run after all the individual tests
	t.Run("AllTogether", func(t *testing.T) {

		db, _ := dbtestutil.NewDB(t)
		manager := runtimeconfig.NewManager()
		s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{}),
			manager,
			// Also sync the default org!
			idpsync.DeploymentSyncSettings{
				GroupField: "groups",
				// This legacy field will fail any tests if the legacy override code
				// has any bugs.
				Legacy: idpsync.DefaultOrgLegacySettings{
					GroupField: "groups",
					GroupMapping: map[string]string{
						"foo": "legacy-foo",
						"baz": "legacy-baz",
					},
					GroupFilter:         regexp.MustCompile("^legacy"),
					CreateMissingGroups: true,
				},
			},
		)

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		user := dbgen.User(t, db, database.User{})

		var asserts []func(t *testing.T)
		// The default org is also going to do something
		def := orgSetupDefinition{
			Name: "DefaultOrg",
			GroupNames: map[string]bool{
				"legacy-foo": false,
				"legacy-baz": true,
				"random":     true,
			},
			// No settings, because they come from the deployment values
			GroupSettings: nil,
			assertGroups: &orgGroupAssert{
				ExpectedGroupNames: []string{"legacy-foo", "legacy-baz", "legacy-bar"},
			},
		}

		//nolint:gocritic // testing
		defOrg, err := db.GetDefaultOrganization(dbauthz.AsSystemRestricted(ctx))
		require.NoError(t, err)
		SetupOrganization(t, s, db, user, defOrg.ID, def)
		asserts = append(asserts, func(t *testing.T) {
			t.Run(def.Name, func(t *testing.T) {
				t.Parallel()
				def.Assert(t, defOrg.ID, db, user)
			})
		})

		for _, tc := range testCases {
			tc := tc

			orgID := uuid.New()
			SetupOrganization(t, s, db, user, orgID, tc)
			asserts = append(asserts, func(t *testing.T) {
				t.Run(tc.Name, func(t *testing.T) {
					t.Parallel()
					tc.Assert(t, orgID, db, user)
				})
			})
		}

		asserts = append(asserts, func(t *testing.T) {
			t.Helper()
			def.Assert(t, defOrg.ID, db, user)
		})

		// Do the group sync!
		err = s.SyncGroups(ctx, db, user, idpsync.GroupParams{
			SyncEntitled: true,
			MergedClaims: userClaims,
		})
		require.NoError(t, err)

		for _, assert := range asserts {
			assert(t)
		}
	})
}

func TestSyncDisabled(t *testing.T) {
	t.Parallel()

	if dbtestutil.WillUsePostgres() {
		t.Skip("Skipping test because it populates a lot of db entries, which is slow on postgres.")
	}

	db, _ := dbtestutil.NewDB(t)
	manager := runtimeconfig.NewManager()
	s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{}),
		manager,
		idpsync.DeploymentSyncSettings{},
	)

	ids := coderdtest.NewDeterministicUUIDGenerator()
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	user := dbgen.User(t, db, database.User{})
	orgID := uuid.New()

	def := orgSetupDefinition{
		Name: "SyncDisabled",
		Groups: map[uuid.UUID]bool{
			ids.ID("foo"): true,
			ids.ID("bar"): true,
			ids.ID("baz"): false,
			ids.ID("bop"): false,
		},
		GroupSettings: &codersdk.GroupSyncSettings{
			Field: "groups",
			Mapping: map[string][]uuid.UUID{
				"foo": {ids.ID("foo")},
				"baz": {ids.ID("baz")},
			},
		},
		assertGroups: &orgGroupAssert{
			ExpectedGroups: []uuid.UUID{
				ids.ID("foo"),
				ids.ID("bar"),
			},
		},
	}

	SetupOrganization(t, s, db, user, orgID, def)

	// Do the group sync!
	err := s.SyncGroups(ctx, db, user, idpsync.GroupParams{
		SyncEntitled: false,
		MergedClaims: jwt.MapClaims{
			"groups": []string{"baz", "bop"},
		},
	})
	require.NoError(t, err)

	def.Assert(t, orgID, db, user)
}

// TestApplyGroupDifference is mainly testing the database functions
func TestApplyGroupDifference(t *testing.T) {
	t.Parallel()

	ids := coderdtest.NewDeterministicUUIDGenerator()
	testCase := []struct {
		Name   string
		Before map[uuid.UUID]bool
		Add    []uuid.UUID
		Remove []uuid.UUID
		Expect []uuid.UUID
	}{
		{
			Name: "Empty",
		},
		{
			Name: "AddFromNone",
			Before: map[uuid.UUID]bool{
				ids.ID("g1"): false,
			},
			Add: []uuid.UUID{
				ids.ID("g1"),
			},
			Expect: []uuid.UUID{
				ids.ID("g1"),
			},
		},
		{
			Name: "AddSome",
			Before: map[uuid.UUID]bool{
				ids.ID("g1"): true,
				ids.ID("g2"): false,
				ids.ID("g3"): false,
				uuid.New():   false,
			},
			Add: []uuid.UUID{
				ids.ID("g2"),
				ids.ID("g3"),
			},
			Expect: []uuid.UUID{
				ids.ID("g1"),
				ids.ID("g2"),
				ids.ID("g3"),
			},
		},
		{
			Name: "RemoveAll",
			Before: map[uuid.UUID]bool{
				uuid.New():   false,
				ids.ID("g2"): true,
				ids.ID("g3"): true,
			},
			Remove: []uuid.UUID{
				ids.ID("g2"),
				ids.ID("g3"),
			},
			Expect: []uuid.UUID{},
		},
		{
			Name: "Mixed",
			Before: map[uuid.UUID]bool{
				// adds
				ids.ID("a1"): true,
				ids.ID("a2"): true,
				ids.ID("a3"): false,
				ids.ID("a4"): false,
				// removes
				ids.ID("r1"): true,
				ids.ID("r2"): true,
				ids.ID("r3"): false,
				ids.ID("r4"): false,
				// stable
				ids.ID("s1"): true,
				ids.ID("s2"): true,
				// noise
				uuid.New(): false,
				uuid.New(): false,
			},
			Add: []uuid.UUID{
				ids.ID("a1"), ids.ID("a2"),
				ids.ID("a3"), ids.ID("a4"),
				// Double up to try and confuse
				ids.ID("a1"),
				ids.ID("a4"),
			},
			Remove: []uuid.UUID{
				ids.ID("r1"), ids.ID("r2"),
				ids.ID("r3"), ids.ID("r4"),
				// Double up to try and confuse
				ids.ID("r1"),
				ids.ID("r4"),
			},
			Expect: []uuid.UUID{
				ids.ID("a1"), ids.ID("a2"), ids.ID("a3"), ids.ID("a4"),
				ids.ID("s1"), ids.ID("s2"),
			},
		},
	}

	for _, tc := range testCase {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			mgr := runtimeconfig.NewManager()
			db, _ := dbtestutil.NewDB(t)

			ctx := testutil.Context(t, testutil.WaitMedium)
			//nolint:gocritic // testing
			ctx = dbauthz.AsSystemRestricted(ctx)

			org := dbgen.Organization(t, db, database.Organization{})
			_, err := db.InsertAllUsersGroup(ctx, org.ID)
			require.NoError(t, err)

			user := dbgen.User(t, db, database.User{})
			_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
				UserID:         user.ID,
				OrganizationID: org.ID,
			})

			for gid, in := range tc.Before {
				group := dbgen.Group(t, db, database.Group{
					ID:             gid,
					OrganizationID: org.ID,
				})
				if in {
					_ = dbgen.GroupMember(t, db, database.GroupMemberTable{
						UserID:  user.ID,
						GroupID: group.ID,
					})
				}
			}

			s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{}), mgr, idpsync.FromDeploymentValues(coderdtest.DeploymentValues(t)))
			err = s.ApplyGroupDifference(context.Background(), db, user, tc.Add, tc.Remove)
			require.NoError(t, err)

			userGroups, err := db.GetGroups(ctx, database.GetGroupsParams{
				HasMemberID: user.ID,
			})
			require.NoError(t, err)

			// assert
			found := db2sdk.List(userGroups, func(g database.GetGroupsRow) uuid.UUID {
				return g.Group.ID
			})

			// Add everyone group
			require.ElementsMatch(t, append(tc.Expect, org.ID), found)
		})
	}
}

func TestExpectedGroupEqual(t *testing.T) {
	t.Parallel()

	ids := coderdtest.NewDeterministicUUIDGenerator()
	testCases := []struct {
		Name  string
		A     idpsync.ExpectedGroup
		B     idpsync.ExpectedGroup
		Equal bool
	}{
		{
			Name:  "Empty",
			A:     idpsync.ExpectedGroup{},
			B:     idpsync.ExpectedGroup{},
			Equal: true,
		},
		{
			Name: "DifferentOrgs",
			A: idpsync.ExpectedGroup{
				OrganizationID: uuid.New(),
				GroupID:        ptr.Ref(ids.ID("g1")),
				GroupName:      nil,
			},
			B: idpsync.ExpectedGroup{
				OrganizationID: uuid.New(),
				GroupID:        ptr.Ref(ids.ID("g1")),
				GroupName:      nil,
			},
			Equal: false,
		},
		{
			Name: "SameID",
			A: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        ptr.Ref(ids.ID("g1")),
				GroupName:      nil,
			},
			B: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        ptr.Ref(ids.ID("g1")),
				GroupName:      nil,
			},
			Equal: true,
		},
		{
			Name: "DifferentIDs",
			A: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        ptr.Ref(uuid.New()),
				GroupName:      nil,
			},
			B: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        ptr.Ref(uuid.New()),
				GroupName:      nil,
			},
			Equal: false,
		},
		{
			Name: "SameName",
			A: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        nil,
				GroupName:      ptr.Ref("foo"),
			},
			B: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        nil,
				GroupName:      ptr.Ref("foo"),
			},
			Equal: true,
		},
		{
			Name: "DifferentName",
			A: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        nil,
				GroupName:      ptr.Ref("foo"),
			},
			B: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        nil,
				GroupName:      ptr.Ref("bar"),
			},
			Equal: false,
		},
		// Edge cases
		{
			// A bit strange, but valid as ID takes priority.
			// We assume 2 groups with the same ID are equal, even if
			// their names are different. Names are mutable, IDs are not,
			// so there is 0% chance they are different groups.
			Name: "DifferentIDSameName",
			A: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        ptr.Ref(ids.ID("g1")),
				GroupName:      ptr.Ref("foo"),
			},
			B: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        ptr.Ref(ids.ID("g1")),
				GroupName:      ptr.Ref("bar"),
			},
			Equal: true,
		},
		{
			Name: "MixedNils",
			A: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        ptr.Ref(ids.ID("g1")),
				GroupName:      nil,
			},
			B: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        nil,
				GroupName:      ptr.Ref("bar"),
			},
			Equal: false,
		},
		{
			Name: "NoComparable",
			A: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        ptr.Ref(ids.ID("g1")),
				GroupName:      nil,
			},
			B: idpsync.ExpectedGroup{
				OrganizationID: ids.ID("org"),
				GroupID:        nil,
				GroupName:      nil,
			},
			Equal: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.Equal, tc.A.Equal(tc.B))
		})
	}
}

func SetupOrganization(t *testing.T, s *idpsync.AGPLIDPSync, db database.Store, user database.User, orgID uuid.UUID, def orgSetupDefinition) {
	t.Helper()

	// Account that the org might be the default organization
	org, err := db.GetOrganizationByID(context.Background(), orgID)
	if xerrors.Is(err, sql.ErrNoRows) {
		org = dbgen.Organization(t, db, database.Organization{
			ID: orgID,
		})
	}

	_, err = db.InsertAllUsersGroup(context.Background(), org.ID)
	if !database.IsUniqueViolation(err) {
		require.NoError(t, err, "Everyone group for an org")
	}

	manager := runtimeconfig.NewManager()
	orgResolver := manager.OrganizationResolver(db, org.ID)
	if def.GroupSettings != nil {
		err = s.Group.SetRuntimeValue(context.Background(), orgResolver, (*idpsync.GroupSyncSettings)(def.GroupSettings))
		require.NoError(t, err)
	}

	if def.RoleSettings != nil {
		err = s.Role.SetRuntimeValue(context.Background(), orgResolver, def.RoleSettings)
		require.NoError(t, err)
	}

	if !def.NotMember {
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
	}

	if len(def.OrganizationRoles) > 0 {
		_, err := db.UpdateMemberRoles(context.Background(), database.UpdateMemberRolesParams{
			GrantedRoles: def.OrganizationRoles,
			UserID:       user.ID,
			OrgID:        org.ID,
		})
		require.NoError(t, err)
	}

	if len(def.CustomRoles) > 0 {
		for _, cr := range def.CustomRoles {
			_, err := db.InsertCustomRole(context.Background(), database.InsertCustomRoleParams{
				Name:        cr,
				DisplayName: cr,
				OrganizationID: uuid.NullUUID{
					UUID:  org.ID,
					Valid: true,
				},
				SitePermissions: nil,
				OrgPermissions:  nil,
				UserPermissions: nil,
			})
			require.NoError(t, err)
		}
	}

	for groupID, in := range def.Groups {
		dbgen.Group(t, db, database.Group{
			ID:             groupID,
			OrganizationID: org.ID,
		})
		if in {
			dbgen.GroupMember(t, db, database.GroupMemberTable{
				UserID:  user.ID,
				GroupID: groupID,
			})
		}
	}
	for groupName, in := range def.GroupNames {
		group := dbgen.Group(t, db, database.Group{
			Name:           groupName,
			OrganizationID: org.ID,
		})
		if in {
			dbgen.GroupMember(t, db, database.GroupMemberTable{
				UserID:  user.ID,
				GroupID: group.ID,
			})
		}
	}
}

type orgSetupDefinition struct {
	Name string
	// True if the user is a member of the group
	Groups            map[uuid.UUID]bool
	GroupNames        map[string]bool
	OrganizationRoles []string
	CustomRoles       []string
	// NotMember if true will ensure the user is not a member of the organization.
	NotMember bool

	GroupSettings *codersdk.GroupSyncSettings
	RoleSettings  *idpsync.RoleSyncSettings

	assertGroups *orgGroupAssert
	assertRoles  *orgRoleAssert
}

type orgRoleAssert struct {
	ExpectedOrgRoles []string
}

type orgGroupAssert struct {
	ExpectedGroups     []uuid.UUID
	ExpectedGroupNames []string
}

func (o orgSetupDefinition) Assert(t *testing.T, orgID uuid.UUID, db database.Store, user database.User) {
	t.Helper()

	ctx := context.Background()

	members, err := db.OrganizationMembers(ctx, database.OrganizationMembersParams{
		OrganizationID: orgID,
		UserID:         user.ID,
	})
	require.NoError(t, err)
	if o.NotMember {
		require.Len(t, members, 0, "should not be a member")
	} else {
		require.Len(t, members, 1, "should be a member")
	}

	if o.assertGroups != nil {
		o.assertGroups.Assert(t, orgID, db, user)
	}
	if o.assertRoles != nil {
		o.assertRoles.Assert(t, orgID, db, o.NotMember, user)
	}

	// If the user is not a member, there is nothing to really assert in the org
	if o.assertGroups == nil && o.assertRoles == nil && !o.NotMember {
		t.Errorf("no group or role asserts present, must have at least one")
		t.FailNow()
	}
}

func (o *orgGroupAssert) Assert(t *testing.T, orgID uuid.UUID, db database.Store, user database.User) {
	t.Helper()

	ctx := context.Background()

	userGroups, err := db.GetGroups(ctx, database.GetGroupsParams{
		OrganizationID: orgID,
		HasMemberID:    user.ID,
	})
	require.NoError(t, err)
	if o.ExpectedGroups == nil {
		o.ExpectedGroups = make([]uuid.UUID, 0)
	}
	if len(o.ExpectedGroupNames) > 0 && len(o.ExpectedGroups) > 0 {
		t.Fatal("ExpectedGroups and ExpectedGroupNames are mutually exclusive")
	}

	// Everyone groups mess up our asserts
	userGroups = slices.DeleteFunc(userGroups, func(row database.GetGroupsRow) bool {
		return row.Group.ID == row.Group.OrganizationID
	})

	if len(o.ExpectedGroupNames) > 0 {
		found := db2sdk.List(userGroups, func(g database.GetGroupsRow) string {
			return g.Group.Name
		})
		require.ElementsMatch(t, o.ExpectedGroupNames, found, "user groups by name")
		require.Len(t, o.ExpectedGroups, 0, "ExpectedGroups should be empty")
	} else {
		// Check by ID, recommended
		found := db2sdk.List(userGroups, func(g database.GetGroupsRow) uuid.UUID {
			return g.Group.ID
		})
		require.ElementsMatch(t, o.ExpectedGroups, found, "user groups")
		require.Len(t, o.ExpectedGroupNames, 0, "ExpectedGroupNames should be empty")
	}
}

//nolint:revive
func (o orgRoleAssert) Assert(t *testing.T, orgID uuid.UUID, db database.Store, notMember bool, user database.User) {
	t.Helper()

	ctx := context.Background()

	members, err := db.OrganizationMembers(ctx, database.OrganizationMembersParams{
		OrganizationID: orgID,
		UserID:         user.ID,
	})
	if notMember {
		require.ErrorIs(t, err, sql.ErrNoRows)
		return
	}
	require.NoError(t, err)
	require.Len(t, members, 1)
	member := members[0]
	require.ElementsMatch(t, member.OrganizationMember.Roles, o.ExpectedOrgRoles)
}
