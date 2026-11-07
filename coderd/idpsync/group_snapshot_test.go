package idpsync_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/testutil"
)

// This file holds the test vectors for DatabaseGroupSnapshotReader, which
// reads a GroupSnapshot from the database. The tests that use these vectors
// follow them in this file.
//
// UUIDs exist only once the database is set up, so the vectors name groups
// by symbol, and a test builds a fresh database to match each vector. Two
// symbols have a fixed meaning:
//
//   - "everyone" is the target organization's Everyone group, whose ID is the
//     organization's ID;
//   - "other_everyone" is the other organization's Everyone group, whose ID
//     is that organization's ID.
//
// Every other symbol stands for a fresh UUID, which names a group when the
// vector creates one and names nothing otherwise.
//
// The tests read through the database store directly, which applies no
// authorization, so they need no system subject. In production the store is
// wrapped by dbauthz, and the reader's precondition requires one.

// snapshotReadVector is one test vector for DatabaseGroupSnapshotReader.
type snapshotReadVector struct {
	// name identifies the vector in test output. It is unique within the
	// table.
	name string
	// why says what the vector checks and why its result is the right one.
	why string
	// org is the state the target organization is set up in.
	org idpsync.OrganizationStatus
	// inside lists the symbols of groups created in the target organization.
	inside []string
	// outside lists the symbols of groups created in another organization,
	// which always exists and is active.
	outside []string
	// requested lists the symbols passed to the reader, in order.
	requested []string
	// wantOrganization is the organization status the snapshot must report.
	wantOrganization idpsync.OrganizationStatus
	// wantGroups is the status the snapshot must give each requested symbol.
	// The snapshot must have no other entries.
	wantGroups map[string]idpsync.GroupStatus
}

// snapshotReadVectors is the table of test vectors for
// DatabaseGroupSnapshotReader.
//
// The vectors are grouped in this order:
//
//  1. An active organization, one status at a time.
//  2. An active organization, statuses together and requests that do not
//     cover every group.
//  3. A deleted organization.
//  4. An absent organization.
var snapshotReadVectors = []snapshotReadVector{
	// Group 1: an active organization, one status at a time.
	{
		name:             "active/inside",
		why:              "Groups created in the target organization are inside it.",
		org:              idpsync.OrganizationActive,
		inside:           []string{"g1", "g2"},
		requested:        []string{"g1", "g2"},
		wantOrganization: idpsync.OrganizationActive,
		wantGroups: map[string]idpsync.GroupStatus{
			"g1": idpsync.GroupInsideOrganization,
			"g2": idpsync.GroupInsideOrganization,
		},
	},
	{
		name:             "active/outside",
		why:              "A group created in another organization is outside the target.",
		org:              idpsync.OrganizationActive,
		outside:          []string{"x"},
		requested:        []string{"x"},
		wantOrganization: idpsync.OrganizationActive,
		wantGroups:       map[string]idpsync.GroupStatus{"x": idpsync.GroupOutsideOrganization},
	},
	{
		name:             "active/does_not_exist",
		why:              "A UUID no group was created with does not exist.",
		org:              idpsync.OrganizationActive,
		requested:        []string{"m"},
		wantOrganization: idpsync.OrganizationActive,
		wantGroups:       map[string]idpsync.GroupStatus{"m": idpsync.GroupDoesNotExist},
	},
	{
		name:             "active/everyone_inside",
		why:              "The target's Everyone group belongs to the target, so it is inside; V5 is the validator's concern, not the reader's.",
		org:              idpsync.OrganizationActive,
		inside:           []string{"everyone"},
		requested:        []string{"everyone"},
		wantOrganization: idpsync.OrganizationActive,
		wantGroups:       map[string]idpsync.GroupStatus{"everyone": idpsync.GroupInsideOrganization},
	},
	{
		name:             "active/other_everyone_outside",
		why:              "Another organization's Everyone group belongs to that organization, so it is outside the target.",
		org:              idpsync.OrganizationActive,
		outside:          []string{"other_everyone"},
		requested:        []string{"other_everyone"},
		wantOrganization: idpsync.OrganizationActive,
		wantGroups:       map[string]idpsync.GroupStatus{"other_everyone": idpsync.GroupOutsideOrganization},
	},

	// Group 2: an active organization, statuses together and requests that
	// do not cover every group.
	{
		name:             "active/mixed",
		why:              "Inside, outside, and missing UUIDs in one request each get their own status.",
		org:              idpsync.OrganizationActive,
		inside:           []string{"g1"},
		outside:          []string{"x"},
		requested:        []string{"g1", "x", "m"},
		wantOrganization: idpsync.OrganizationActive,
		wantGroups: map[string]idpsync.GroupStatus{
			"g1": idpsync.GroupInsideOrganization,
			"x":  idpsync.GroupOutsideOrganization,
			"m":  idpsync.GroupDoesNotExist,
		},
	},
	{
		name:             "active/no_request_ignores_other_groups",
		why:              "With nothing requested the snapshot has no entries, though groups exist; a query with an empty ID list would otherwise match every group.",
		org:              idpsync.OrganizationActive,
		inside:           []string{"g1"},
		outside:          []string{"x"},
		requested:        []string{},
		wantOrganization: idpsync.OrganizationActive,
		wantGroups:       map[string]idpsync.GroupStatus{},
	},
	{
		name:             "active/unrequested_groups_left_out",
		why:              "Only requested UUIDs get entries, so an existing group that was not requested is not in the snapshot.",
		org:              idpsync.OrganizationActive,
		inside:           []string{"g1", "g2"},
		outside:          []string{"x"},
		requested:        []string{"g1"},
		wantOrganization: idpsync.OrganizationActive,
		wantGroups:       map[string]idpsync.GroupStatus{"g1": idpsync.GroupInsideOrganization},
	},
	{
		name:             "active/repeated_request",
		why:              "A UUID requested twice gets one entry.",
		org:              idpsync.OrganizationActive,
		inside:           []string{"g1"},
		requested:        []string{"g1", "g1"},
		wantOrganization: idpsync.OrganizationActive,
		wantGroups:       map[string]idpsync.GroupStatus{"g1": idpsync.GroupInsideOrganization},
	},

	// Group 3: a deleted organization. Deletion requires that it have no
	// groups but Everyone.
	{
		name:             "deleted/everyone_still_inside",
		why:              "A soft-deleted organization keeps its Everyone group, which is still inside it; a UUID that names nothing does not exist.",
		org:              idpsync.OrganizationDeleted,
		inside:           []string{"everyone"},
		requested:        []string{"everyone", "m"},
		wantOrganization: idpsync.OrganizationDeleted,
		wantGroups: map[string]idpsync.GroupStatus{
			"everyone": idpsync.GroupInsideOrganization,
			"m":        idpsync.GroupDoesNotExist,
		},
	},

	// Group 4: an absent organization. No organization has the target ID, so
	// no group is inside it.
	{
		name:             "absent/outside_and_missing",
		why:              "With no organization at the target ID, a group elsewhere is outside and a UUID that names nothing does not exist.",
		org:              idpsync.OrganizationAbsent,
		outside:          []string{"x"},
		requested:        []string{"x", "m"},
		wantOrganization: idpsync.OrganizationAbsent,
		wantGroups: map[string]idpsync.GroupStatus{
			"x": idpsync.GroupOutsideOrganization,
			"m": idpsync.GroupDoesNotExist,
		},
	},
}

// TestDatabaseGroupSnapshotReaderTable checks the table of vectors itself:
// names are unique, the setup is one the database allows, and the expected
// entries are exactly the requested symbols. A defect found here is in the
// table, not in the code under test.
func TestDatabaseGroupSnapshotReaderTable(t *testing.T) {
	t.Parallel()

	names := make(map[string]bool, len(snapshotReadVectors))
	for _, v := range snapshotReadVectors {
		require.False(t, names[v.name], "vector name %q is not unique", v.name)
		names[v.name] = true
		require.NotEmpty(t, v.why, "vector %q has no why", v.name)

		switch v.org {
		case idpsync.OrganizationActive:
		case idpsync.OrganizationDeleted:
			for _, sym := range v.inside {
				require.Equal(t, "everyone", sym, "vector %q: a deleted organization can keep only its Everyone group", v.name)
			}
		case idpsync.OrganizationAbsent:
			require.Empty(t, v.inside, "vector %q: an absent organization has no groups", v.name)
		default:
			require.Failf(t, "malformed vector", "vector %q: unknown organization state %q", v.name, v.org)
		}
		require.NotContains(t, v.inside, "other_everyone", "vector %q: other_everyone belongs to the other organization", v.name)
		require.NotContains(t, v.outside, "everyone", "vector %q: everyone belongs to the target organization", v.name)

		require.NotNil(t, v.requested, "vector %q: requested must be set, if only to an empty list", v.name)
		distinct := slices.Compact(slices.Sorted(slices.Values(v.requested)))
		wanted := make([]string, 0, len(v.wantGroups))
		for sym := range v.wantGroups {
			wanted = append(wanted, sym)
		}
		require.ElementsMatch(t, distinct, wanted, "vector %q: wantGroups must have exactly the requested symbols", v.name)
	}
}

// TestDatabaseGroupSnapshotReader builds a database for every vector, reads
// a snapshot for the requested UUIDs, and checks it against the vector.
func TestDatabaseGroupSnapshotReader(t *testing.T) {
	t.Parallel()

	for _, v := range snapshotReadVectors {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			db, _ := dbtestutil.NewDB(t)

			other := dbgen.Organization(t, db, database.Organization{})
			targetID := uuid.New()
			if v.org != idpsync.OrganizationAbsent {
				targetID = dbgen.Organization(t, db, database.Organization{ID: targetID}).ID
			}

			ids := map[string]uuid.UUID{
				"everyone":       targetID,
				"other_everyone": other.ID,
			}
			idOf := func(sym string) uuid.UUID {
				if _, ok := ids[sym]; !ok {
					ids[sym] = uuid.New()
				}
				return ids[sym]
			}
			for _, sym := range v.inside {
				dbgen.Group(t, db, database.Group{ID: idOf(sym), OrganizationID: targetID})
			}
			for _, sym := range v.outside {
				dbgen.Group(t, db, database.Group{ID: idOf(sym), OrganizationID: other.ID})
			}
			if v.org == idpsync.OrganizationDeleted {
				require.NoError(t, db.UpdateOrganizationDeletedByID(ctx, database.UpdateOrganizationDeletedByIDParams{
					ID:        targetID,
					UpdatedAt: dbtime.Now(),
				}))
			}

			requested := make([]uuid.UUID, 0, len(v.requested))
			for _, sym := range v.requested {
				requested = append(requested, idOf(sym))
			}
			want := make(map[uuid.UUID]idpsync.GroupStatus, len(v.wantGroups))
			for sym, status := range v.wantGroups {
				want[idOf(sym)] = status
			}

			snapshot, err := idpsync.DatabaseGroupSnapshotReader(db)(ctx, targetID, requested)
			require.NoError(t, err, v.why)
			require.Equal(t, targetID, snapshot.OrganizationID, v.why)
			require.Equal(t, v.wantOrganization, snapshot.Organization, v.why)
			require.Equal(t, want, snapshot.Groups, v.why)
		})
	}
}
