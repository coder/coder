package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMultiOrgFetch(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	makeOrgs := []string{"foo", "bar", "baz"}
	for _, name := range makeOrgs {
		_, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        name,
			DisplayName: name,
		})
		require.NoError(t, err)
	}

	orgs, err := client.OrganizationsByUser(ctx, codersdk.Me)
	require.NoError(t, err)
	require.NotNil(t, orgs)
	require.Len(t, orgs, len(makeOrgs)+1)
}

func TestOrganizationsByUser(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

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
}

func TestOrganizationByUserAndName(t *testing.T) {
	t.Parallel()
	t.Run("NoExist", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := client.OrganizationByUserAndName(ctx, codersdk.Me, "nothing")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("NoMember", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		other, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)

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

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		org, err := client.Organization(ctx, user.OrganizationID)
		require.NoError(t, err)
		_, err = client.OrganizationByUserAndName(ctx, codersdk.Me, org.Name)
		require.NoError(t, err)
	})
}

func TestPostOrganizationsByUser(t *testing.T) {
	t.Parallel()
	t.Run("Conflict", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		org, err := client.Organization(ctx, user.OrganizationID)
		require.NoError(t, err)
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
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

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
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "new",
			DisplayName: "New",
			Description: "A new organization to love and cherish forever.",
		})
		require.NoError(t, err)
		require.Equal(t, "new", o.Name)
		require.Equal(t, "New", o.DisplayName)
		require.Equal(t, "A new organization to love and cherish forever.", o.Description)
	})

	t.Run("CreateWithoutExplicitDisplayName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name: "new",
		})
		require.NoError(t, err)
		require.Equal(t, "new", o.Name)
		require.Equal(t, "new", o.DisplayName) // should match the given `Name`
	})
}

func TestPatchOrganizationsByUser(t *testing.T) {
	t.Parallel()
	t.Run("Conflict", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		originalOrg, err := client.Organization(ctx, user.OrganizationID)
		require.NoError(t, err)
		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "something-unique",
			DisplayName: "Something Unique",
		})
		require.NoError(t, err)

		_, err = client.UpdateOrganization(ctx, o.ID.String(), codersdk.UpdateOrganizationRequest{
			Name: originalOrg.Name,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("ReservedName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "something-unique",
			DisplayName: "Something Unique",
		})
		require.NoError(t, err)

		_, err = client.UpdateOrganization(ctx, o.ID.String(), codersdk.UpdateOrganizationRequest{
			Name: codersdk.DefaultOrganization,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("InvalidName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "something-unique",
			DisplayName: "Something Unique",
		})
		require.NoError(t, err)

		_, err = client.UpdateOrganization(ctx, o.ID.String(), codersdk.UpdateOrganizationRequest{
			Name: "something unique but not url safe",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("UpdateById", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "new",
			DisplayName: "New",
		})
		require.NoError(t, err)

		o, err = client.UpdateOrganization(ctx, o.ID.String(), codersdk.UpdateOrganizationRequest{
			Name: "new-new",
		})
		require.NoError(t, err)
		require.Equal(t, "new-new", o.Name)
	})

	t.Run("UpdateByName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "new",
			DisplayName: "New",
		})
		require.NoError(t, err)

		o, err = client.UpdateOrganization(ctx, o.Name, codersdk.UpdateOrganizationRequest{
			Name: "new-new",
		})
		require.NoError(t, err)
		require.Equal(t, "new-new", o.Name)
		require.Equal(t, "New", o.DisplayName) // didn't change
	})

	t.Run("UpdateDisplayName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "new",
			DisplayName: "New",
		})
		require.NoError(t, err)

		o, err = client.UpdateOrganization(ctx, o.Name, codersdk.UpdateOrganizationRequest{
			DisplayName: "The Newest One",
		})
		require.NoError(t, err)
		require.Equal(t, "new", o.Name) // didn't change
		require.Equal(t, "The Newest One", o.DisplayName)
	})

	t.Run("UpdateDescription", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "new",
			DisplayName: "New",
		})
		require.NoError(t, err)

		o, err = client.UpdateOrganization(ctx, o.Name, codersdk.UpdateOrganizationRequest{
			Description: "wow, this organization description is so updated!",
		})

		require.NoError(t, err)
		require.Equal(t, "new", o.Name)        // didn't change
		require.Equal(t, "New", o.DisplayName) // didn't change
		require.Equal(t, "wow, this organization description is so updated!", o.Description)
	})
}

func TestDeleteOrganizationsByUser(t *testing.T) {
	t.Parallel()
	t.Run("Default", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		o, err := client.Organization(ctx, user.OrganizationID)
		require.NoError(t, err)

		err = client.DeleteOrganization(ctx, o.ID.String())
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("DeleteById", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "doomed",
			DisplayName: "Doomed",
		})
		require.NoError(t, err)

		err = client.DeleteOrganization(ctx, o.ID.String())
		require.NoError(t, err)
	})

	t.Run("DeleteByName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		o, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "doomed",
			DisplayName: "Doomed",
		})
		require.NoError(t, err)

		err = client.DeleteOrganization(ctx, o.Name)
		require.NoError(t, err)
	})
}
