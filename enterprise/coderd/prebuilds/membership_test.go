package prebuilds_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tailscale.com/types/ptr"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/testutil"
)

// TestReconcileAll verifies that StoreMembershipReconciler correctly updates membership
// for the prebuilds system user.
func TestReconcileAll(t *testing.T) {
	t.Parallel()

	ctx := dbauthz.AsPrebuildsOrchestrator(testutil.Context(t, testutil.WaitLong))
	clock := quartz.NewMock(t)

	// Helper to build a minimal Preset row belonging to a given org.
	newPresetRow := func(orgID uuid.UUID) database.GetTemplatePresetsWithPrebuildsRow {
		return database.GetTemplatePresetsWithPrebuildsRow{
			ID:             uuid.New(),
			OrganizationID: orgID,
		}
	}

	tests := []struct {
		name                       string
		includePreset              []bool
		preExistingOrgMembership   []bool
		preExistingGroup           []bool
		preExistingGroupMembership []bool
		// Expected outcomes
		expectOrgMembershipExists *bool
		expectGroupExists         *bool
		expectUserInGroup         *bool
	}{
		{
			name:                       "if there are no presets, membership reconciliation is a no-op",
			includePreset:              []bool{false},
			preExistingOrgMembership:   []bool{true, false},
			preExistingGroup:           []bool{true, false},
			preExistingGroupMembership: []bool{true, false},
			expectOrgMembershipExists:  ptr.To(false),
			expectGroupExists:          ptr.To(false),
		},
		{
			name:                       "if there is a preset, then we should enforce org and group membership in all cases",
			includePreset:              []bool{true},
			preExistingOrgMembership:   []bool{true, false},
			preExistingGroup:           []bool{true, false},
			preExistingGroupMembership: []bool{true, false},
			expectOrgMembershipExists:  ptr.To(true),
			expectGroupExists:          ptr.To(true),
			expectUserInGroup:          ptr.To(true),
		},
	}

	for _, tc := range tests {
		tc := tc
		for _, includePreset := range tc.includePreset {
			includePreset := includePreset
			for _, preExistingOrgMembership := range tc.preExistingOrgMembership {
				preExistingOrgMembership := preExistingOrgMembership
				for _, preExistingGroup := range tc.preExistingGroup {
					preExistingGroup := preExistingGroup
					for _, preExistingGroupMembership := range tc.preExistingGroupMembership {
						preExistingGroupMembership := preExistingGroupMembership
						t.Run(tc.name, func(t *testing.T) {
							t.Parallel()

							_, db := coderdtest.NewWithDatabase(t, nil)

							defaultOrg, err := db.GetDefaultOrganization(ctx)
							require.NoError(t, err)

							// introduce an unrelated organization to ensure that the membership reconciler don't interfere with it.
							unrelatedOrg := dbgen.Organization(t, db, database.Organization{})
							targetOrg := dbgen.Organization(t, db, database.Organization{})

							if !dbtestutil.WillUsePostgres() {
								// dbmem doesn't ensure membership to the default organization
								dbgen.OrganizationMember(t, db, database.OrganizationMember{
									OrganizationID: defaultOrg.ID,
									UserID:         database.PrebuildsSystemUserID,
								})
							}

							// Ensure membership to unrelated org.
							dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: unrelatedOrg.ID, UserID: database.PrebuildsSystemUserID})

							if preExistingOrgMembership {
								// System user already a member of both orgs.
								dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: targetOrg.ID, UserID: database.PrebuildsSystemUserID})
							}

							// Create pre-existing prebuilds group if required by test case
							var prebuildsGroup database.Group
							if preExistingGroup {
								prebuildsGroup = dbgen.Group(t, db, database.Group{
									Name:           "prebuilds",
									DisplayName:    "Prebuilds",
									OrganizationID: targetOrg.ID,
									QuotaAllowance: 0,
								})

								// Add the system user to the group if preExistingGroupMembership is true
								if preExistingGroupMembership {
									dbgen.GroupMember(t, db, database.GroupMemberTable{
										GroupID: prebuildsGroup.ID,
										UserID:  database.PrebuildsSystemUserID,
									})
								}
							}

							presets := []database.GetTemplatePresetsWithPrebuildsRow{newPresetRow(unrelatedOrg.ID)}
							if includePreset {
								presets = append(presets, newPresetRow(targetOrg.ID))
							}

							// Verify memberships before reconciliation.
							preReconcileMemberships, err := db.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
								UserID: database.PrebuildsSystemUserID,
							})
							require.NoError(t, err)
							expectedMembershipsBefore := []uuid.UUID{defaultOrg.ID, unrelatedOrg.ID}
							if preExistingOrgMembership {
								expectedMembershipsBefore = append(expectedMembershipsBefore, targetOrg.ID)
							}
							require.ElementsMatch(t, expectedMembershipsBefore, extractOrgIDs(preReconcileMemberships))

							// Reconcile
							reconciler := prebuilds.NewStoreMembershipReconciler(db, clock)
							require.NoError(t, reconciler.ReconcileAll(ctx, database.PrebuildsSystemUserID, presets))

							// Verify memberships after reconciliation.
							postReconcileMemberships, err := db.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
								UserID: database.PrebuildsSystemUserID,
							})
							require.NoError(t, err)
							expectedMembershipsAfter := expectedMembershipsBefore
							if !preExistingOrgMembership && tc.expectOrgMembershipExists != nil && *tc.expectOrgMembershipExists {
								expectedMembershipsAfter = append(expectedMembershipsAfter, targetOrg.ID)
							}
							require.ElementsMatch(t, expectedMembershipsAfter, extractOrgIDs(postReconcileMemberships))

							// Verify prebuilds group behavior based on expected outcomes
							prebuildsGroup, err = db.GetGroupByOrgAndName(ctx, database.GetGroupByOrgAndNameParams{
								OrganizationID: targetOrg.ID,
								Name:           "prebuilds",
							})
							if tc.expectGroupExists != nil && *tc.expectGroupExists {
								require.NoError(t, err)
								require.Equal(t, "prebuilds", prebuildsGroup.Name)
								require.Equal(t, "Prebuilds", prebuildsGroup.DisplayName)
								require.Equal(t, int32(0), prebuildsGroup.QuotaAllowance) // Default quota should be 0

								if tc.expectUserInGroup != nil && *tc.expectUserInGroup {
									// Check that the system user is a member of the prebuilds group
									groupMembers, err := db.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
										GroupID:       prebuildsGroup.ID,
										IncludeSystem: true,
									})
									require.NoError(t, err)
									require.Len(t, groupMembers, 1)
									require.Equal(t, database.PrebuildsSystemUserID, groupMembers[0].UserID)
								}

								if tc.expectUserInGroup != nil && !*tc.expectUserInGroup {
									// Check that the system user is NOT a member of the prebuilds group
									groupMembers, err := db.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
										GroupID:       prebuildsGroup.ID,
										IncludeSystem: true,
									})
									require.NoError(t, err)
									require.Len(t, groupMembers, 0)
								}
							}

							if !preExistingGroup && tc.expectGroupExists != nil && !*tc.expectGroupExists {
								// Verify that no prebuilds group exists
								require.Error(t, err)
								require.True(t, errors.Is(err, sql.ErrNoRows))
							}
						})
					}
				}
			}
		}
	}
}

func extractOrgIDs(orgs []database.Organization) []uuid.UUID {
	ids := make([]uuid.UUID, len(orgs))
	for i, o := range orgs {
		ids[i] = o.ID
	}
	return ids
}
