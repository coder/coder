package idpsync_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/testutil"
)

func TestParseGroupClaims(t *testing.T) {
	t.Parallel()

	t.Run("EmptyConfig", func(t *testing.T) {
		t.Parallel()

		s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{}),
			runtimeconfig.NewStoreManager(),
			idpsync.DeploymentSyncSettings{})

		ctx := testutil.Context(t, testutil.WaitMedium)

		params, err := s.ParseGroupClaims(ctx, jwt.MapClaims{})
		require.Nil(t, err)

		require.False(t, params.SyncEnabled)
	})

	// AllowList has no effect in AGPL
	t.Run("AllowList", func(t *testing.T) {
		t.Parallel()

		s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{}),
			runtimeconfig.NewStoreManager(),
			idpsync.DeploymentSyncSettings{
				GroupField: "groups",
				GroupAllowList: map[string]struct{}{
					"foo": {},
				},
			})

		ctx := testutil.Context(t, testutil.WaitMedium)

		params, err := s.ParseGroupClaims(ctx, jwt.MapClaims{})
		require.Nil(t, err)
		require.False(t, params.SyncEnabled)
	})
}

func TestGroupSyncTable(t *testing.T) {
	t.Parallel()

	if dbtestutil.WillUsePostgres() {
		t.Skip("Skipping test because it populates a lot of db entries, which is slow on postgres.")
	}

	userClaims := jwt.MapClaims{
		"groups": []string{
			"foo", "bar", "baz",
			"create-bar", "create-baz",
		},
	}

	ids := coderdtest.NewDeterministicUUIDGenerator()
	testCases := []orgSetupDefinition{
		{
			Name: "SwitchGroups",
			Settings: &idpsync.GroupSyncSettings{
				GroupField: "groups",
				GroupMapping: map[string][]uuid.UUID{
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
			ExpectedGroups: []uuid.UUID{
				ids.ID("sg-foo"),
				ids.ID("sg-foo-2"),
				ids.ID("sg-bar"),
				ids.ID("sg-baz"),
			},
		},
		{
			Name: "StayInGroup",
			Settings: &idpsync.GroupSyncSettings{
				GroupField: "groups",
				// Only match foo, so bar does not map
				RegexFilter: regexp.MustCompile("^foo$"),
				GroupMapping: map[string][]uuid.UUID{
					"foo": {ids.ID("gg-foo"), uuid.New()},
					"bar": {ids.ID("gg-bar")},
					"baz": {ids.ID("gg-baz")},
				},
			},
			Groups: map[uuid.UUID]bool{
				ids.ID("gg-foo"): true,
				ids.ID("gg-bar"): false,
			},
			ExpectedGroups: []uuid.UUID{
				ids.ID("gg-foo"),
			},
		},
		{
			Name: "UserJoinsGroups",
			Settings: &idpsync.GroupSyncSettings{
				GroupField: "groups",
				GroupMapping: map[string][]uuid.UUID{
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
			ExpectedGroups: []uuid.UUID{
				ids.ID("ng-foo"),
				ids.ID("ng-bar"),
				ids.ID("ng-bar-2"),
				ids.ID("ng-baz"),
			},
		},
		{
			Name: "CreateGroups",
			Settings: &idpsync.GroupSyncSettings{
				GroupField:              "groups",
				RegexFilter:             regexp.MustCompile("^create"),
				AutoCreateMissingGroups: true,
			},
			Groups: map[uuid.UUID]bool{},
			ExpectedGroupNames: []string{
				"create-bar",
				"create-baz",
			},
		},
		{
			Name: "GroupNamesNoMapping",
			Settings: &idpsync.GroupSyncSettings{
				GroupField:              "groups",
				RegexFilter:             regexp.MustCompile(".*"),
				AutoCreateMissingGroups: false,
			},
			GroupNames: map[string]bool{
				"foo":  false,
				"bar":  false,
				"goob": true,
			},
			ExpectedGroupNames: []string{
				"foo",
				"bar",
			},
		},
		{
			Name: "NoUser",
			Settings: &idpsync.GroupSyncSettings{
				GroupField: "groups",
				GroupMapping: map[string][]uuid.UUID{
					// Extra ID that does not map to a group
					"foo": {ids.ID("ow-foo"), uuid.New()},
				},
				RegexFilter:             nil,
				AutoCreateMissingGroups: false,
			},
			NotMember: true,
			Groups: map[uuid.UUID]bool{
				ids.ID("ow-foo"): false,
				ids.ID("ow-bar"): false,
			},
		},
		{
			Name:     "NoSettingsNoUser",
			Settings: nil,
			Groups:   map[uuid.UUID]bool{},
		},
		{
			Name: "LegacyMapping",
			Settings: &idpsync.GroupSyncSettings{
				GroupField:  "groups",
				RegexFilter: regexp.MustCompile("^legacy"),
				LegacyGroupNameMapping: map[string]string{
					"create-bar": "legacy-bar",
					"foo":        "legacy-foo",
				},
				AutoCreateMissingGroups: true,
			},
			Groups: map[uuid.UUID]bool{},
			GroupNames: map[string]bool{
				"legacy-foo": false,
			},
			ExpectedGroupNames: []string{
				"legacy-bar",
				"legacy-foo",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			if tc.OrgID == uuid.Nil {
				tc.OrgID = uuid.New()
			}

			db, _ := dbtestutil.NewDB(t)
			manager := runtimeconfig.NewStoreManager()
			s := idpsync.NewAGPLSync(slogtest.Make(t, &slogtest.Options{}),
				manager,
				idpsync.DeploymentSyncSettings{
					GroupField: "groups",
				},
			)

			ctx := testutil.Context(t, testutil.WaitMedium)
			user := dbgen.User(t, db, database.User{})
			SetupOrganization(t, s, db, user, tc)

			// Do the group sync!
			err := s.SyncGroups(ctx, db, user, idpsync.GroupParams{
				SyncEnabled:  true,
				MergedClaims: userClaims,
			})
			require.NoError(t, err)

			tc.Assert(t, tc.OrgID, db, user)
		})
	}
}

func SetupOrganization(t *testing.T, s *idpsync.AGPLIDPSync, db database.Store, user database.User, def orgSetupDefinition) {
	org := dbgen.Organization(t, db, database.Organization{
		ID: def.OrgID,
	})
	_, err := db.InsertAllUsersGroup(context.Background(), org.ID)
	require.NoError(t, err, "Everyone group for an org")

	manager := runtimeconfig.NewStoreManager()
	orgResolver := manager.OrganizationResolver(db, org.ID)
	err = s.Group.SetRuntimeValue(context.Background(), orgResolver, def.Settings)
	require.NoError(t, err)

	if !def.NotMember {
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
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
	Name  string
	OrgID uuid.UUID
	// True if the user is a member of the group
	Groups     map[uuid.UUID]bool
	GroupNames map[string]bool
	NotMember  bool

	Settings           *idpsync.GroupSyncSettings
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
	} else {
		// Check by ID, recommended
		found := db2sdk.List(userGroups, func(g database.GetGroupsRow) uuid.UUID {
			return g.Group.ID
		})
		require.ElementsMatch(t, o.ExpectedGroups, found, "user groups")
	}
}
