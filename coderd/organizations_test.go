package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
)

func TestOrganizationsByUser(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	orgs, err := client.OrganizationsByUser(context.Background(), codersdk.Me)
	require.NoError(t, err)
	require.NotNil(t, orgs)
	require.Len(t, orgs, 1)
}

func TestOrganizationByUserAndName(t *testing.T) {
	t.Parallel()
	t.Run("NoExist", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		_, err := client.OrganizationByName(context.Background(), codersdk.Me, "nothing")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("NoMember", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		other := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		org, err := other.CreateOrganization(context.Background(), codersdk.Me, codersdk.CreateOrganizationRequest{
			Name: "another",
		})
		require.NoError(t, err)
		_, err = client.OrganizationByName(context.Background(), codersdk.Me, org.Name)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		org, err := client.Organization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
		_, err = client.OrganizationByName(context.Background(), codersdk.Me, org.Name)
		require.NoError(t, err)
	})
}

func TestPostOrganizationsByUser(t *testing.T) {
	t.Parallel()
	t.Run("Conflict", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		org, err := client.Organization(context.Background(), user.OrganizationID)
		require.NoError(t, err)
		_, err = client.CreateOrganization(context.Background(), codersdk.Me, codersdk.CreateOrganizationRequest{
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
		_, err := client.CreateOrganization(context.Background(), codersdk.Me, codersdk.CreateOrganizationRequest{
			Name: "new",
		})
		require.NoError(t, err)
	})
}
