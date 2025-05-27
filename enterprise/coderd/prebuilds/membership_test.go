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
	agplprebuilds "github.com/coder/coder/v2/coderd/prebuilds"
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
		{name: "no presets, no memberships", includePreset: false, preExistingMembership: false},
		{name: "preset, but no membership", includePreset: true, preExistingMembership: false},
		{name: "preset, but already a member", includePreset: true, preExistingMembership: true},
		{name: "member, but no presets", includePreset: false, preExistingMembership: true},
	}

	for _, tc := range tests {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)

			defaultOrg, err := db.GetDefaultOrganization(ctx)
			require.NoError(t, err)
			backgroundOrg := dbgen.Organization(t, db, database.Organization{})
			targetOrg := dbgen.Organization(t, db, database.Organization{})

			dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: backgroundOrg.ID, UserID: agplprebuilds.SystemUserID})
			if tc.preExistingMembership {
				// System user already a member of both orgs.
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: targetOrg.ID, UserID: agplprebuilds.SystemUserID})
			}

			presets := []database.GetTemplatePresetsWithPrebuildsRow{newPresetRow(backgroundOrg.ID)}
			if tc.includePreset {
				presets = append(presets, newPresetRow(targetOrg.ID))
			}

			// Verify memberships before reconciliation.
			preReconcileMemberships, err := db.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
				UserID: agplprebuilds.SystemUserID,
			})
			require.NoError(t, err)
			expected := []uuid.UUID{defaultOrg.ID, backgroundOrg.ID}
			if tc.preExistingMembership {
				expected = append(expected, targetOrg.ID)
			}
			require.ElementsMatch(t, expected, extractOrgIDs(preReconcileMemberships))

			// Reconcile
			reconciler := prebuilds.NewStoreMembershipReconciler(db, clock)
			require.NoError(t, reconciler.ReconcileAll(ctx, agplprebuilds.SystemUserID, presets))

			// Verify memberships after reconciliation.
			postReconcileMemberships, err := db.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
				UserID: agplprebuilds.SystemUserID,
			})
			require.NoError(t, err)
			if !tc.preExistingMembership && tc.includePreset {
				expected = append(expected, targetOrg.ID)
			}
			require.ElementsMatch(t, expected, extractOrgIDs(postReconcileMemberships))
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
