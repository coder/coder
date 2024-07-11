package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerKeys(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong*10)
	t.Cleanup(cancel)
	client := coderdtest.New(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	orgAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	otherOrg, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
		Name: "other",
	})
	require.NoError(t, err, "create org")
	outsideOrgAdmin, _ := coderdtest.CreateAnotherUser(t, client, otherOrg.ID, rbac.ScopedRoleOrgAdmin(otherOrg.ID))

	// member cannot create a provisioner key
	_, err = member.CreateProvisionerKey(ctx, otherOrg.ID, codersdk.CreateProvisionerKeyRequest{
		Name: "key",
	})
	require.ErrorContains(t, err, "Resource not found")

	// member cannot list provisioner keys
	_, err = member.ListProvisionerKeys(ctx, otherOrg.ID)
	require.ErrorContains(t, err, "Resource not found")

	// member cannot delete a provisioner key
	err = member.DeleteProvisionerKey(ctx, otherOrg.ID, "key")
	require.ErrorContains(t, err, "Resource not found")

	// outside org admin cannot create a provisioner key
	_, err = outsideOrgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: "key",
	})
	require.ErrorContains(t, err, "Resource not found")

	// outside org admin cannot list provisioner keys
	_, err = outsideOrgAdmin.ListProvisionerKeys(ctx, owner.OrganizationID)
	require.ErrorContains(t, err, "Resource not found")

	// outside org admin cannot delete a provisioner key
	err = outsideOrgAdmin.DeleteProvisionerKey(ctx, owner.OrganizationID, "key")
	require.ErrorContains(t, err, "Resource not found")

	// org admin can list provisioner keys and get an empty list
	keys, err := orgAdmin.ListProvisionerKeys(ctx, owner.OrganizationID)
	require.NoError(t, err, "org admin list provisioner keys")
	require.Len(t, keys, 0, "org admin list provisioner keys")

	// org admin can create a provisioner key
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: "key",
	})
	require.NoError(t, err, "org admin create provisioner key")

	// org admin can conflict on name creating a provisioner key
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: "key",
	})
	require.ErrorContains(t, err, "already exists")

	// key name cannot have special characters
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: "key with spaces",
	})
	require.ErrorContains(t, err, "org admin create provisioner key")

	// key name cannot be too long
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: "key with spaces",
	})
	require.ErrorContains(t, err, "less than 64 characters")

	// key name cannot be empty
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: "",
	})
	require.ErrorContains(t, err, "cannot be empty")

	// org admin can list provisioner keys
	keys, err = orgAdmin.ListProvisionerKeys(ctx, owner.OrganizationID)
	require.NoError(t, err, "org admin list provisioner keys")
	require.Len(t, keys, 1, "org admin list provisioner keys")

	// org admin can delete a provisioner key
	err = orgAdmin.DeleteProvisionerKey(ctx, owner.OrganizationID, "key")
	require.NoError(t, err, "org admin delete provisioner key")

	// org admin cannot delete a provisioner key that doesn't exist
	err = orgAdmin.DeleteProvisionerKey(ctx, owner.OrganizationID, "key")
	require.ErrorContains(t, err, "Resource not found")
}
