package prebuilds_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestReconcileAll verifies that StoreMembershipReconciler correctly updates membership
// for the prebuilds system user.
func TestReconcileAll(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)

	tests := []struct {
		name                       string
		includePreset              bool
		preExistingOrgMembership   []bool
		preExistingGroup           []bool
		preExistingGroupMembership []bool
		// Expected outcomes
		expectOrgMembershipExists bool
		expectGroupExists         bool
		expectUserInGroup         bool
	}{
		{
			name:                       "if there are no presets, membership reconciliation is a no-op",
			includePreset:              false,
			preExistingOrgMembership:   []bool{true, false},
			preExistingGroup:           []bool{true, false},
			preExistingGroupMembership: []bool{true, false},
			expectOrgMembershipExists:  false,
			expectGroupExists:          false,
			expectUserInGroup:          false,
		},
		{
			name:                       "if there is a preset, then we should enforce org and group membership in all cases",
			includePreset:              true,
			preExistingOrgMembership:   []bool{true, false},
			preExistingGroup:           []bool{true, false},
			preExistingGroupMembership: []bool{true, false},
			expectOrgMembershipExists:  true,
			expectGroupExists:          true,
			expectUserInGroup:          true,
		},
	}

	for _, tc := range tests {
		tc := tc
		includePreset := tc.includePreset
		for _, preExistingOrgMembership := range tc.preExistingOrgMembership {
			preExistingOrgMembership := preExistingOrgMembership
			for _, preExistingGroup := range tc.preExistingGroup {
				preExistingGroup := preExistingGroup
				for _, preExistingGroupMembership := range tc.preExistingGroupMembership {
					preExistingGroupMembership := preExistingGroupMembership
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()

						// nolint:gocritic // Reconciliation happens as prebuilds system user, not a human user.
						ctx := dbauthz.AsPrebuildsOrchestrator(testutil.Context(t, testutil.WaitLong))
						client, db := coderdtest.NewWithDatabase(t, nil)
						owner := coderdtest.CreateFirstUser(t, client)

						defaultOrg, err := db.GetDefaultOrganization(ctx)
						require.NoError(t, err)

						// Introduce an unrelated organization to ensure that the membership reconciler doesn't interfere with it.
						unrelatedOrg := dbgen.Organization(t, db, database.Organization{})
						dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: unrelatedOrg.ID, UserID: database.PrebuildsSystemUserID})

						// Organization to test
						targetOrg := dbgen.Organization(t, db, database.Organization{})

						// Prebuilds system user is a member of the organization
						if preExistingOrgMembership {
							dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: targetOrg.ID, UserID: database.PrebuildsSystemUserID})
						}

						// Organization has the prebuilds group
						var prebuildsGroup database.Group
						if preExistingGroup {
							prebuildsGroup = dbgen.Group(t, db, database.Group{
								Name:           prebuilds.PrebuiltWorkspacesGroupName,
								DisplayName:    prebuilds.PrebuiltWorkspacesGroupDisplayName,
								OrganizationID: targetOrg.ID,
								QuotaAllowance: 0,
							})

							// Add the system user to the group if required by test case
							if preExistingGroupMembership {
								dbgen.GroupMember(t, db, database.GroupMemberTable{
									GroupID: prebuildsGroup.ID,
									UserID:  database.PrebuildsSystemUserID,
								})
							}
						}

						// Setup unrelated org preset
						dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
							OrganizationID: unrelatedOrg.ID,
							CreatedBy:      owner.UserID,
						}).Preset(database.TemplateVersionPreset{
							DesiredInstances: sql.NullInt32{
								Int32: 1,
								Valid: true,
							},
						}).Do()

						// Setup target org preset
						dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
							OrganizationID: targetOrg.ID,
							CreatedBy:      owner.UserID,
						}).Preset(database.TemplateVersionPreset{
							DesiredInstances: sql.NullInt32{
								Int32: 0,
								Valid: includePreset,
							},
						}).Do()

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
						reconciler := prebuilds.NewStoreMembershipReconciler(db, clock, slogtest.Make(t, nil))
						require.NoError(t, reconciler.ReconcileAll(ctx, database.PrebuildsSystemUserID, prebuilds.PrebuiltWorkspacesGroupName))

						// Verify memberships after reconciliation.
						postReconcileMemberships, err := db.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
							UserID: database.PrebuildsSystemUserID,
						})
						require.NoError(t, err)
						expectedMembershipsAfter := expectedMembershipsBefore
						if !preExistingOrgMembership && tc.expectOrgMembershipExists {
							expectedMembershipsAfter = append(expectedMembershipsAfter, targetOrg.ID)
						}
						require.ElementsMatch(t, expectedMembershipsAfter, extractOrgIDs(postReconcileMemberships))

						// Verify prebuilds group behavior based on expected outcomes
						prebuildsGroup, err = db.GetGroupByOrgAndName(ctx, database.GetGroupByOrgAndNameParams{
							OrganizationID: targetOrg.ID,
							Name:           prebuilds.PrebuiltWorkspacesGroupName,
						})
						if tc.expectGroupExists {
							require.NoError(t, err)
							require.Equal(t, prebuilds.PrebuiltWorkspacesGroupName, prebuildsGroup.Name)
							require.Equal(t, prebuilds.PrebuiltWorkspacesGroupDisplayName, prebuildsGroup.DisplayName)
							require.Equal(t, int32(0), prebuildsGroup.QuotaAllowance) // Default quota should be 0

							if tc.expectUserInGroup {
								// Check that the system user is a member of the prebuilds group
								groupMembers, err := db.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
									GroupID:       prebuildsGroup.ID,
									IncludeSystem: true,
								})
								require.NoError(t, err)
								require.Len(t, groupMembers, 1)
								require.Equal(t, database.PrebuildsSystemUserID, groupMembers[0].UserID)
							}

							// If no preset exists, then we do not enforce group membership:
							if !tc.expectUserInGroup {
								// Check that the system user is NOT a member of the prebuilds group
								groupMembers, err := db.GetGroupMembersByGroupID(ctx, database.GetGroupMembersByGroupIDParams{
									GroupID:       prebuildsGroup.ID,
									IncludeSystem: true,
								})
								require.NoError(t, err)
								require.Len(t, groupMembers, 0)
							}
						}

						if !preExistingGroup && !tc.expectGroupExists {
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

func extractOrgIDs(orgs []database.Organization) []uuid.UUID {
	ids := make([]uuid.UUID, len(orgs))
	for i, o := range orgs {
		ids[i] = o.ID
	}
	return ids
}
