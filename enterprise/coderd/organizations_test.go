package coderd_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestMultiOrgFetch(t *testing.T) {
	t.Parallel()
	client, _ := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})

	ctx := testutil.Context(t, testutil.WaitLong)

	makeOrgs := []string{"foo", "bar", "baz"}
	for _, name := range makeOrgs {
		_, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        name,
			DisplayName: name,
		})
		require.NoError(t, err)
	}

	//nolint:gocritic // using the owner intentionally since only they can make orgs
	myOrgs, err := client.OrganizationsByUser(ctx, codersdk.Me)
	require.NoError(t, err)
	require.NotNil(t, myOrgs)
	require.Len(t, myOrgs, len(makeOrgs)+1)

	orgs, err := client.Organizations(ctx)
	require.NoError(t, err)
	require.NotNil(t, orgs)
	require.ElementsMatch(t, myOrgs, orgs)
}

func TestOrganizationsByUser(t *testing.T) {
	t.Parallel()

	t.Run("IsDefault", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // owner is required to make orgs
		orgs, err := client.OrganizationsByUser(ctx, codersdk.Me)
		require.NoError(t, err)
		require.NotNil(t, orgs)
		require.Len(t, orgs, 1)
		require.True(t, orgs[0].IsDefault, "first org is always default")

		// Make an extra org, and it should not be defaulted.
		notDefault, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "another",
			DisplayName: "Another",
		})
		require.NoError(t, err)
		require.False(t, notDefault.IsDefault, "only 1 default org allowed")
	})

	t.Run("NoMember", func(t *testing.T) {
		t.Parallel()
		client, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		other, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // owner is required to make orgs
		org, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "another",
			DisplayName: "Another",
		})
		require.NoError(t, err)

		_, err = other.OrganizationByUserAndName(ctx, codersdk.Me, org.Name)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})
}

func TestAddOrganizationMembers(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

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

		inv, root := clitest.New(t, "organization", "members", "add", "-O", otherOrg.ID.String(), user.Username)
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

func TestDeleteOrganizationsByUser(t *testing.T) {
	t.Parallel()
	t.Run("Default", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		// nolint:gocritic // owner used below to delete
		o, err := client.Organization(ctx, user.OrganizationID)
		require.NoError(t, err)

		// nolint:gocritic // only owners can delete orgs
		err = client.DeleteOrganization(ctx, o.ID.String())
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("DeleteById", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		o := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		// nolint:gocritic // only owners can delete orgs
		err := client.DeleteOrganization(ctx, o.ID.String())
		require.NoError(t, err)
	})

	t.Run("DeleteByName", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		o := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		// nolint:gocritic // only owners can delete orgs
		err := client.DeleteOrganization(ctx, o.Name)
		require.NoError(t, err)
	})
}

func TestPatchOrganizationsByUser(t *testing.T) {
	t.Parallel()
	t.Run("Conflict", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		// nolint:gocritic // owner used below as only they can create orgs
		originalOrg, err := client.Organization(ctx, user.OrganizationID)
		require.NoError(t, err)

		o := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		// nolint:gocritic // owner used above to make the org
		_, err = client.UpdateOrganization(ctx, o.ID.String(), codersdk.UpdateOrganizationRequest{
			Name: originalOrg.Name,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("ReservedName", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		var err error
		o := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		_, err = client.UpdateOrganization(ctx, o.ID.String(), codersdk.UpdateOrganizationRequest{
			Name: codersdk.DefaultOrganization,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("InvalidName", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		var err error
		o := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		_, err = client.UpdateOrganization(ctx, o.ID.String(), codersdk.UpdateOrganizationRequest{
			Name: "something unique but not url safe",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("UpdateById", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		var err error
		o := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		o, err = client.UpdateOrganization(ctx, o.ID.String(), codersdk.UpdateOrganizationRequest{
			Name: "new-new-org",
		})
		require.NoError(t, err)
		require.Equal(t, "new-new-org", o.Name)
	})

	t.Run("UpdateByName", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		const displayName = "New Organization"
		var err error
		o := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{}, func(request *codersdk.CreateOrganizationRequest) {
			request.DisplayName = displayName
		})

		o, err = client.UpdateOrganization(ctx, o.Name, codersdk.UpdateOrganizationRequest{
			Name: "new-new-org",
		})
		require.NoError(t, err)
		require.Equal(t, "new-new-org", o.Name)
		require.Equal(t, displayName, o.DisplayName) // didn't change
	})

	t.Run("UpdateDisplayName", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		var err error
		const name = "new-org"
		o := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{}, func(request *codersdk.CreateOrganizationRequest) {
			request.Name = name
		})

		const displayName = "The Newest One"
		o, err = client.UpdateOrganization(ctx, o.Name, codersdk.UpdateOrganizationRequest{
			DisplayName: "The Newest One",
		})
		require.NoError(t, err)
		require.Equal(t, "new-org", o.Name) // didn't change
		require.Equal(t, displayName, o.DisplayName)
	})

	t.Run("UpdateDescription", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		const displayName = "New Organization"
		var err error
		o := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{}, func(request *codersdk.CreateOrganizationRequest) {
			request.DisplayName = displayName
			request.Name = "new-org"
		})

		const description = "wow, this organization description is so updated!"
		o, err = client.UpdateOrganization(ctx, o.Name, codersdk.UpdateOrganizationRequest{
			Description: ptr.Ref(description),
		})

		require.NoError(t, err)
		require.Equal(t, "new-org", o.Name)          // didn't change
		require.Equal(t, displayName, o.DisplayName) // didn't change
		require.Equal(t, description, o.Description)
	})

	t.Run("UpdateIcon", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		const displayName = "New Organization"
		var err error
		o := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{}, func(request *codersdk.CreateOrganizationRequest) {
			request.DisplayName = displayName
			request.Icon = "/emojis/random.png"
			request.Name = "new-org"
		})

		const icon = "/emojis/1f48f-1f3ff.png"
		o, err = client.UpdateOrganization(ctx, o.Name, codersdk.UpdateOrganizationRequest{
			Icon: ptr.Ref(icon),
		})

		require.NoError(t, err)
		require.Equal(t, "new-org", o.Name)          // didn't change
		require.Equal(t, displayName, o.DisplayName) // didn't change
		require.Equal(t, icon, o.Icon)
	})

	t.Run("RevokedLicense", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitMedium)

		const displayName = "New Organization"
		o := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{}, func(request *codersdk.CreateOrganizationRequest) {
			request.DisplayName = displayName
			request.Icon = "/emojis/random.png"
			request.Name = "new-org"
		})

		// Remove the license to block premium functionality.
		licenses, err := client.Licenses(ctx)
		require.NoError(t, err, "get licenses")
		for _, license := range licenses {
			// Should be only 1...
			err := client.DeleteLicense(ctx, license.ID)
			require.NoError(t, err, "delete license")
		}

		// Verify functionality is lost.
		const icon = "/emojis/1f48f-1f3ff.png"
		o, err = client.UpdateOrganization(ctx, o.Name, codersdk.UpdateOrganizationRequest{
			Icon: ptr.Ref(icon),
		})
		require.ErrorContains(t, err, "Multiple Organizations is a Premium feature")
	})
}

func TestPostOrganizationsByUser(t *testing.T) {
	t.Parallel()
	t.Run("Conflict", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // using owner for below
		org, err := client.Organization(ctx, user.OrganizationID)
		require.NoError(t, err)

		//nolint:gocritic // only owners can create orgs
		_, err = client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        org.Name,
			DisplayName: org.DisplayName,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("InvalidName", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // only owners can create orgs
		_, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "A name which is definitely not url safe",
			DisplayName: "New",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // only owners can create orgs
		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "new-org",
			DisplayName: "New organization",
			Description: "A new organization to love and cherish forever.",
			Icon:        "/emojis/1f48f-1f3ff.png",
		})
		require.NoError(t, err)
		require.Equal(t, "new-org", o.Name)
		require.Equal(t, "New organization", o.DisplayName)
		require.Equal(t, "A new organization to love and cherish forever.", o.Description)
		require.Equal(t, "/emojis/1f48f-1f3ff.png", o.Icon)
	})

	t.Run("CreateWithoutExplicitDisplayName", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // only owners can create orgs
		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name: "new-org",
		})
		require.NoError(t, err)
		require.Equal(t, "new-org", o.Name)
		require.Equal(t, "new-org", o.DisplayName) // should match the given `Name`
	})
}
