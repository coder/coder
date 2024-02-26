package cli_test

import (
	"testing"

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
