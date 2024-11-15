package cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/provisionerkey"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerKeys(t *testing.T) {
	t.Parallel()

	t.Run("CRUD", func(t *testing.T) {
		t.Parallel()

		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})
		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))

		name := "dont-TEST-me"
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, conf := newCLI(
			t,
			"provisioner", "keys", "create", name, "--tag", "foo=bar", "--tag", "my=way",
		)

		pty := ptytest.New(t)
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, orgAdminClient, conf)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		line := pty.ReadLine(ctx)
		require.Contains(t, line, "Successfully created provisioner key")
		require.Contains(t, line, strings.ToLower(name))
		// empty line
		_ = pty.ReadLine(ctx)
		key := pty.ReadLine(ctx)
		require.NotEmpty(t, key)
		require.NoError(t, provisionerkey.Validate(key))

		inv, conf = newCLI(
			t,
			"provisioner", "keys", "ls",
		)
		pty = ptytest.New(t)
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, orgAdminClient, conf)

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		line = pty.ReadLine(ctx)
		require.Contains(t, line, "NAME")
		require.Contains(t, line, "CREATED AT")
		require.Contains(t, line, "TAGS")
		line = pty.ReadLine(ctx)
		require.Contains(t, line, strings.ToLower(name))
		require.Contains(t, line, "foo=bar my=way")

		inv, conf = newCLI(
			t,
			"provisioner", "keys", "delete", "-y", name,
		)

		pty = ptytest.New(t)
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, orgAdminClient, conf)

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		line = pty.ReadLine(ctx)
		require.Contains(t, line, "Successfully deleted provisioner key")
		require.Contains(t, line, strings.ToLower(name))

		inv, conf = newCLI(
			t,
			"provisioner", "keys", "ls",
		)
		pty = ptytest.New(t)
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, orgAdminClient, conf)

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		line = pty.ReadLine(ctx)
		require.Contains(t, line, "No provisioner keys found")
	})
}
