package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestCurrentOrganization(t *testing.T) {
	t.Parallel()

	// This test emulates 2 cases:
	// 1. The user is not a part of the default organization, but only belongs to one.
	// 2. The user is connecting to an older Coder instance.
	t.Run("no-default", func(t *testing.T) {
		t.Parallel()

		orgID := uuid.New()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode([]codersdk.Organization{
				{
					ID:        orgID,
					Name:      "not-default",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					IsDefault: false,
				},
			})
		}))
		defer srv.Close()

		client := codersdk.New(must(url.Parse(srv.URL)))
		inv, root := clitest.New(t, "organizations", "show", "current")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch(orgID.String())
	})

	t.Run("OnlyID", func(t *testing.T) {
		t.Parallel()
		ownerClient := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, ownerClient)
		// Owner is required to make orgs
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID, rbac.RoleOwner())

		ctx := testutil.Context(t, testutil.WaitMedium)
		orgs := []string{"foo", "bar"}
		for _, orgName := range orgs {
			_, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
				Name: orgName,
			})
			require.NoError(t, err)
		}

		inv, root := clitest.New(t, "organizations", "show", "--only-id")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch(first.OrganizationID.String())
	})

	t.Run("UsingFlag", func(t *testing.T) {
		t.Parallel()
		ownerClient := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, ownerClient)
		// Owner is required to make orgs
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID, rbac.RoleOwner())

		ctx := testutil.Context(t, testutil.WaitMedium)
		orgs := map[string]codersdk.Organization{
			"foo": {},
			"bar": {},
		}
		for orgName := range orgs {
			org, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
				Name: orgName,
			})
			require.NoError(t, err)
			orgs[orgName] = org
		}

		inv, root := clitest.New(t, "organizations", "show", "current", "--only-id", "-z=bar")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch(orgs["bar"].ID.String())
	})
}

func TestOrganizationSwitch(t *testing.T) {
	t.Parallel()

	t.Run("Switch", func(t *testing.T) {
		t.Parallel()
		ownerClient := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, ownerClient)
		// Owner is required to make orgs
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID, rbac.RoleOwner())

		ctx := testutil.Context(t, testutil.WaitMedium)
		orgs := []string{"foo", "bar"}
		for _, orgName := range orgs {
			_, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
				Name: orgName,
			})
			require.NoError(t, err)
		}

		exp, err := client.OrganizationByName(ctx, "foo")
		require.NoError(t, err)

		inv, root := clitest.New(t, "organizations", "set", "foo")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch(exp.ID.String())
	})
}

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}
