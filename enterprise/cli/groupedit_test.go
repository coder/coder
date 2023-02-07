package cli_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/cli"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestGroupEdit(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		})

		ctx, _ := testutil.Context(t)
		_, user1 := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		_, user2 := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

		group, err := client.CreateGroup(ctx, admin.OrganizationID, codersdk.CreateGroupRequest{
			Name: "alpha",
		})
		require.NoError(t, err)

		// We use the sdk here as opposed to the CLI since adding this user
		// is considered setup. They will be removed in the proper CLI test.
		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user3.ID.String()},
		})
		require.NoError(t, err)

		var (
			expectedName = "beta"
		)

		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(),
			"groups", "edit", group.Name,
			"--name", expectedName,
			"--avatar-url", "https://example.com",
			"-a", user1.ID.String(),
			"-a", user2.Email,
			"-r", user3.ID.String(),
		)

		pty := ptytest.New(t)

		cmd.SetOut(pty.Output())
		clitest.SetupConfig(t, client, root)

		err = cmd.Execute()
		require.NoError(t, err)

		pty.ExpectMatch(fmt.Sprintf("Successfully patched group %s", cliui.Styles.Keyword.Render(expectedName)))
	})

	t.Run("InvalidUserInput", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		})

		ctx, _ := testutil.Context(t)

		group, err := client.CreateGroup(ctx, admin.OrganizationID, codersdk.CreateGroupRequest{
			Name: "alpha",
		})
		require.NoError(t, err)

		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(),
			"groups", "edit", group.Name,
			"-a", "foo",
		)

		clitest.SetupConfig(t, client, root)

		err = cmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), "must be a valid UUID or email address")
	})

	t.Run("NoArg", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		})

		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(), "groups", "edit")

		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.Error(t, err)
	})
}
