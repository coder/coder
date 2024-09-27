package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerKeys(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong*10)
	t.Cleanup(cancel)
	dv := coderdtest.DeploymentValues(t)
	client, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dv,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	orgAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	otherOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
	outsideOrgAdmin, _ := coderdtest.CreateAnotherUser(t, client, otherOrg.ID, rbac.ScopedRoleOrgAdmin(otherOrg.ID))

	// member cannot create a provisioner key
	_, err := member.CreateProvisionerKey(ctx, otherOrg.ID, codersdk.CreateProvisionerKeyRequest{
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

	// org admin cannot create reserved provisioner keys
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: codersdk.ProvisionerKeyNameBuiltIn,
	})
	require.ErrorContains(t, err, "reserved")
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: codersdk.ProvisionerKeyNameUserAuth,
	})
	require.ErrorContains(t, err, "reserved")
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: codersdk.ProvisionerKeyNamePSK,
	})
	require.ErrorContains(t, err, "reserved")

	// org admin can list provisioner keys and get an empty list
	keys, err := orgAdmin.ListProvisionerKeys(ctx, owner.OrganizationID)
	require.NoError(t, err, "org admin list provisioner keys")
	require.Len(t, keys, 0, "org admin list provisioner keys")

	tags := map[string]string{
		"my": "way",
	}
	// org admin can create a provisioner key
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: "Key", // case insensitive
		Tags: tags,
	})
	require.NoError(t, err, "org admin create provisioner key")

	// org admin can conflict on name creating a provisioner key
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: "KEY", // still conflicts
	})
	require.ErrorContains(t, err, "already exists in organization")

	// key name cannot be too long
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: "Everyone please pass your watermelons to the front of the pool, the storm is approaching.",
	})
	require.ErrorContains(t, err, "must be at most 64 characters")

	// key name cannot be empty
	_, err = orgAdmin.CreateProvisionerKey(ctx, owner.OrganizationID, codersdk.CreateProvisionerKeyRequest{
		Name: "",
	})
	require.ErrorContains(t, err, "is required")

	// org admin can list provisioner keys
	keys, err = orgAdmin.ListProvisionerKeys(ctx, owner.OrganizationID)
	require.NoError(t, err, "org admin list provisioner keys")
	require.Len(t, keys, 1, "org admin list provisioner keys")
	require.Equal(t, "key", keys[0].Name, "org admin list provisioner keys name matches")
	require.EqualValues(t, tags, keys[0].Tags, "org admin list provisioner keys tags match")

	// org admin can delete a provisioner key
	err = orgAdmin.DeleteProvisionerKey(ctx, owner.OrganizationID, "key") // using lowercase here works
	require.NoError(t, err, "org admin delete provisioner key")

	// org admin cannot delete a provisioner key that doesn't exist
	err = orgAdmin.DeleteProvisionerKey(ctx, owner.OrganizationID, "key")
	require.ErrorContains(t, err, "Resource not found")

	// org admin cannot delete reserved provisioner keys
	err = orgAdmin.DeleteProvisionerKey(ctx, owner.OrganizationID, codersdk.ProvisionerKeyNameBuiltIn)
	require.ErrorContains(t, err, "reserved")
	err = orgAdmin.DeleteProvisionerKey(ctx, owner.OrganizationID, codersdk.ProvisionerKeyNameUserAuth)
	require.ErrorContains(t, err, "reserved")
	err = orgAdmin.DeleteProvisionerKey(ctx, owner.OrganizationID, codersdk.ProvisionerKeyNamePSK)
	require.ErrorContains(t, err, "reserved")
}
