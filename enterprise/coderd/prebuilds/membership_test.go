package prebuilds_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
)

// TestReconcileAll verifies that StoreMembershipReconciler correctly updates membership
// for the prebuilds system user.
func TestReconcileAll(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clock := quartz.NewMock(t)

	// Helper to build a minimal Preset row belonging to a given org.
	newPresetRow := func(orgID uuid.UUID) database.GetTemplatePresetsWithPrebuildsRow {
		return database.GetTemplatePresetsWithPrebuildsRow{
			ID:             uuid.New(),
			OrganizationID: orgID,
		}
	}

	tests := []struct {
		name                  string
		includePreset         bool
		preExistingMembership bool
	}{
		// The StoreMembershipReconciler acts based on the provided agplprebuilds.GlobalSnapshot.
		// These test cases must therefore trust any valid snapshot, so the only relevant functional test cases are:

		// No presets to act on and the prebuilds user does not belong to any organizations.
		// Reconciliation should be a no-op
		{name: "no presets, no memberships", includePreset: false, preExistingMembership: false},
		// If we have a preset that requires prebuilds, but the prebuilds user is not a member of
		// that organization, then we should add the membership.
		{name: "preset, but no membership", includePreset: true, preExistingMembership: false},
		// If the prebuilds system user is already a member of the organization to which a preset belongs,
		// then reconciliation should be a no-op:
		{name: "preset, but already a member", includePreset: true, preExistingMembership: true},
		// If the prebuilds system user is a member of an organization that doesn't have need any prebuilds,
		// then it must have required prebuilds in the past. The membership is not currently necessary, but
		// the reconciler won't remove it, because there's little cost to keeping it and prebuilds might be
		// enabled again.
		{name: "member, but no presets", includePreset: false, preExistingMembership: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)

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

			dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: unrelatedOrg.ID, UserID: database.PrebuildsSystemUserID})
			if tc.preExistingMembership {
				// System user already a member of both orgs.
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: targetOrg.ID, UserID: database.PrebuildsSystemUserID})
			}

			presets := []database.GetTemplatePresetsWithPrebuildsRow{newPresetRow(unrelatedOrg.ID)}
			if tc.includePreset {
				presets = append(presets, newPresetRow(targetOrg.ID))
			}

			// Verify memberships before reconciliation.
			preReconcileMemberships, err := db.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
				UserID: database.PrebuildsSystemUserID,
			})
			require.NoError(t, err)
			expectedMembershipsBefore := []uuid.UUID{defaultOrg.ID, unrelatedOrg.ID}
			if tc.preExistingMembership {
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
			if !tc.preExistingMembership && tc.includePreset {
				expectedMembershipsAfter = append(expectedMembershipsAfter, targetOrg.ID)
			}
			require.ElementsMatch(t, expectedMembershipsAfter, extractOrgIDs(postReconcileMemberships))
		})
	}
}

func extractOrgIDs(orgs []database.Organization) []uuid.UUID {
	ids := make([]uuid.UUID, len(orgs))
	for i, o := range orgs {
		ids[i] = o.ID
	}
	return ids
}
