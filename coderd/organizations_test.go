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
			Name: name,
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
		Name: "another",
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
			Name: "another",
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
			Name: org.Name,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name: "new",
		})
		require.NoError(t, err)
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
			Name: "something-unique",
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
			Name: "something-unique",
		})
		require.NoError(t, err)

		_, err = client.UpdateOrganization(ctx, o.ID.String(), codersdk.UpdateOrganizationRequest{
			Name: codersdk.DefaultOrganization,
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
			Name: "new",
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
			Name: "new",
		})
		require.NoError(t, err)

		o, err = client.UpdateOrganization(ctx, o.Name, codersdk.UpdateOrganizationRequest{
			Name: "new-new",
		})
		require.NoError(t, err)
		require.Equal(t, "new-new", o.Name)
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
			Name: "doomed",
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
			Name: "doomed",
		})
		require.NoError(t, err)

		err = client.DeleteOrganization(ctx, o.Name)
		require.NoError(t, err)
	})
}
