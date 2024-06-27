package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestListOrganizationMembers(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organization", "members", "list", "-c", "user_id,username,roles")
		clitest.SetupConfig(t, client, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, buf.String(), user.Username)
		require.Contains(t, buf.String(), owner.UserID.String())
	})
}

func TestAddOrganizationMembers(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		_, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitMedium)
		//nolint:gocritic // must be an owner, only owners can create orgs
		otherOrg, err := ownerClient.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "Other",
			DisplayName: "",
			Description: "",
			Icon:        "",
		})
		require.NoError(t, err, "create another organization")

		inv, root := clitest.New(t, "organization", "members", "add", "--organization", otherOrg.ID.String(), user.Username)
		//nolint:gocritic // must be an owner
		clitest.SetupConfig(t, ownerClient, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		//nolint:gocritic // must be an owner
		members, err := ownerClient.OrganizationMembers(ctx, otherOrg.ID)
		require.NoError(t, err)

		require.Len(t, members, 2)
	})
}

func TestRemoveOrganizationMembers(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
		_, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitMedium)

		inv, root := clitest.New(t, "organization", "members", "remove", "--organization", owner.OrganizationID.String(), user.Username)
		clitest.SetupConfig(t, orgAdminClient, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		members, err := orgAdminClient.OrganizationMembers(ctx, owner.OrganizationID)
		require.NoError(t, err)

		require.Len(t, members, 2)
	})

	t.Run("UserNotExists", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))

		ctx := testutil.Context(t, testutil.WaitMedium)

		inv, root := clitest.New(t, "organization", "members", "remove", "--organization", owner.OrganizationID.String(), "random_name")
		clitest.SetupConfig(t, orgAdminClient, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "must be an existing uuid or username")
	})
}
