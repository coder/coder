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

func TestGroupDelete(t *testing.T) {
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
		group, err := client.CreateGroup(ctx, admin.OrganizationID, codersdk.CreateGroupRequest{
			Name: "alpha",
		})
		require.NoError(t, err)

		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(),
			"groups", "delete", group.Name,
		)

		pty := ptytest.New(t)

		cmd.SetOut(pty.Output())
		clitest.SetupConfig(t, client, root)

		err = cmd.Execute()
		require.NoError(t, err)

		pty.ExpectMatch(fmt.Sprintf("Successfully deleted group %s", cliui.Styles.Keyword.Render(group.Name)))
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

		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(),
			"groups", "delete")

		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.Error(t, err)
	})
}
