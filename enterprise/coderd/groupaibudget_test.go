package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestGroupAIBudget(t *testing.T) {
	t.Parallel()

	t.Run("UpsertCreatesThenUpdates", func(t *testing.T) {
		t.Parallel()

		client, admin, group := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		// PUT (create).
		created, err := client.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)
		require.Equal(t, group.ID, created.GroupID)
		require.EqualValues(t, 500_000_000, created.SpendLimitMicros)
		require.False(t, created.CreatedAt.IsZero())

		// PUT (update existing).
		updated, err := client.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 1_000_000_000,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1_000_000_000, updated.SpendLimitMicros)
		require.True(t, updated.UpdatedAt.After(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt))

		// GET reflects the latest value.
		got, err := client.GroupAIBudget(ctx, group.ID)
		require.NoError(t, err)
		require.EqualValues(t, 1_000_000_000, got.SpendLimitMicros)

		_ = admin
	})

	t.Run("GetWhenAbsent_404", func(t *testing.T) {
		t.Parallel()

		client, _, group := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := client.GroupAIBudget(ctx, group.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("DeleteWhenAbsent_404", func(t *testing.T) {
		t.Parallel()

		client, _, group := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		err := client.DeleteGroupAIBudget(ctx, group.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("DeleteWhenPresent", func(t *testing.T) {
		t.Parallel()

		client, _, group := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := client.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)

		require.NoError(t, client.DeleteGroupAIBudget(ctx, group.ID))

		_, err = client.GroupAIBudget(ctx, group.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("RejectsNonPositiveSpendLimit", func(t *testing.T) {
		t.Parallel()

		client, _, group := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		for _, spend := range []int64{0, -1} {
			_, err := client.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
				SpendLimitMicros: spend,
			})
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		}
	})

	t.Run("UnknownGroup_404", func(t *testing.T) {
		t.Parallel()

		client, _, _ := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := client.GroupAIBudget(ctx, uuid.New())
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("MemberCannotManage", func(t *testing.T) {
		t.Parallel()

		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureTemplateRBAC: 1},
			},
		})
		adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "budget-group",
		})
		require.NoError(t, err)

		// Member cannot read or write the budget.
		_, err = memberClient.GroupAIBudget(ctx, group.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

		_, err = memberClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 500_000_000,
		})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

// setupGroupAIBudgetTest returns a UserAdmin client for the test org along
// with a freshly-created group inside it. The owner identity is also
// returned for callers that need it.
func setupGroupAIBudgetTest(t *testing.T) (admin *codersdk.Client, owner codersdk.CreateFirstUserResponse, group codersdk.Group) {
	t.Helper()

	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureTemplateRBAC: 1},
		},
	})
	admin, _ = coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

	ctx := testutil.Context(t, testutil.WaitLong)
	g, err := admin.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
		Name: "budget-test-group",
	})
	require.NoError(t, err)
	return admin, owner, g
}
