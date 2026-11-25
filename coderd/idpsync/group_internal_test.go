package idpsync

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestPartitionOutInvalidSettings(t *testing.T) {
	t.Parallel()

	// fixture holds the database state each case partitions against. The
	// organization being synced is org. Groups in other belong to a second
	// organization. deleted was a group in org and has since been deleted.
	// never was never a group anywhere.
	type fixture struct {
		org      uuid.UUID
		everyone uuid.UUID
		inside1  uuid.UUID
		inside2  uuid.UUID
		other1   uuid.UUID
		other2   uuid.UUID
		deleted  uuid.UUID
		never    uuid.UUID
	}

	setup := func(ctx context.Context, t *testing.T, db database.Store) fixture {
		t.Helper()

		org := dbgen.Organization(t, db, database.Organization{})
		everyone, err := db.InsertAllUsersGroup(ctx, org.ID)
		require.NoError(t, err)
		other := dbgen.Organization(t, db, database.Organization{})
		_, err = db.InsertAllUsersGroup(ctx, other.ID)
		require.NoError(t, err)

		deleted := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
		require.NoError(t, db.DeleteGroupByID(ctx, deleted.ID))

		return fixture{
			org:      org.ID,
			everyone: everyone.ID,
			inside1:  dbgen.Group(t, db, database.Group{OrganizationID: org.ID}).ID,
			inside2:  dbgen.Group(t, db, database.Group{OrganizationID: org.ID}).ID,
			other1:   dbgen.Group(t, db, database.Group{OrganizationID: other.ID}).ID,
			other2:   dbgen.Group(t, db, database.Group{OrganizationID: other.ID}).ID,
			deleted:  deleted.ID,
			never:    uuid.New(),
		}
	}

	// cases builds each list from the fixture, because the IDs are only known
	// once the database is populated. Every expected group is tagged with the
	// syncing organization, as ParseClaims tags them.
	type lists struct {
		input, inside, outside, missing []ExpectedGroup
	}
	testCases := []struct {
		Name  string
		Lists func(f fixture) lists
	}{
		{
			Name: "Empty",
			Lists: func(fixture) lists {
				return lists{}
			},
		},
		{
			// A valid mapping passes through unchanged.
			Name: "AllInside",
			Lists: func(f fixture) lists {
				in := []ExpectedGroup{byID(f.org, f.inside1), byID(f.org, f.inside2)}
				return lists{input: in, inside: in}
			},
		},
		{
			// The Everyone group's ID is the organization ID, and it is a
			// group in that organization.
			Name: "EveryoneIsInside",
			Lists: func(f fixture) lists {
				in := []ExpectedGroup{byID(f.org, f.everyone)}
				return lists{input: in, inside: in}
			},
		},
		{
			// Name-only entries are resolved within the organization later,
			// so they count as inside. The organization holds other groups,
			// and none of them may appear in the result: with no IDs to look
			// up, an unfiltered GetGroups would return all of them.
			Name: "NamesOnly",
			Lists: func(f fixture) lists {
				in := []ExpectedGroup{byName(f.org, "alpha"), byName(f.org, "beta")}
				return lists{input: in, inside: in}
			},
		},
		{
			Name: "AllOutside",
			Lists: func(f fixture) lists {
				out := []ExpectedGroup{byID(f.org, f.other1), byID(f.org, f.other2)}
				return lists{input: out, outside: out}
			},
		},
		{
			Name: "InsideAndOutside",
			Lists: func(f fixture) lists {
				return lists{
					input: []ExpectedGroup{
						byID(f.org, f.inside1), byID(f.org, f.other1),
						byName(f.org, "alpha"), byID(f.org, f.other2),
					},
					inside:  []ExpectedGroup{byID(f.org, f.inside1), byName(f.org, "alpha")},
					outside: []ExpectedGroup{byID(f.org, f.other1), byID(f.org, f.other2)},
				}
			},
		},
		{
			Name: "NeverExisted",
			Lists: func(f fixture) lists {
				in := []ExpectedGroup{byID(f.org, f.never)}
				return lists{input: in, missing: in}
			},
		},
		{
			// A deleted group cannot be told apart from one that never
			// existed, so both land in missing.
			Name: "Deleted",
			Lists: func(f fixture) lists {
				in := []ExpectedGroup{byID(f.org, f.deleted)}
				return lists{input: in, missing: in}
			},
		},
		{
			// No ID is inside, and none is outside. Neither the in-org lookup
			// returning nothing nor the second lookup may be mistaken for an
			// unfiltered query.
			Name: "OnlyMissing",
			Lists: func(f fixture) lists {
				in := []ExpectedGroup{byID(f.org, f.deleted), byID(f.org, f.never)}
				return lists{input: in, missing: in}
			},
		},
		{
			Name: "AllThree",
			Lists: func(f fixture) lists {
				return lists{
					input: []ExpectedGroup{
						byID(f.org, f.inside1), byID(f.org, f.other1), byID(f.org, f.never),
						byName(f.org, "alpha"), byID(f.org, f.deleted), byID(f.org, f.other2),
						byID(f.org, f.inside2),
					},
					inside: []ExpectedGroup{
						byID(f.org, f.inside1), byID(f.org, f.inside2), byName(f.org, "alpha"),
					},
					outside: []ExpectedGroup{byID(f.org, f.other1), byID(f.org, f.other2)},
					missing: []ExpectedGroup{byID(f.org, f.never), byID(f.org, f.deleted)},
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitMedium)
			f := setup(ctx, t, db)
			want := tc.Lists(f)

			inside, outside, missing, err := partitionOutInvalidSettings(ctx, db, f.org, want.input)
			require.NoError(t, err)
			require.ElementsMatch(t, want.inside, inside, "inside")
			require.ElementsMatch(t, want.outside, outside, "outside")
			require.ElementsMatch(t, want.missing, missing, "missing")
		})
	}
}

func byID(orgID, groupID uuid.UUID) ExpectedGroup {
	return ExpectedGroup{OrganizationID: orgID, GroupID: &groupID}
}

func byName(orgID uuid.UUID, name string) ExpectedGroup {
	return ExpectedGroup{OrganizationID: orgID, GroupName: &name}
}
