package cli_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
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

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := client.CreateGroup(ctx, admin.OrganizationID, codersdk.CreateGroupRequest{
			Name: "alpha",
		})
		require.NoError(t, err)

		inv, conf := newCLI(t,
			"groups", "delete", group.Name,
		)

		pty := ptytest.New(t)

		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, client, conf)

		err = inv.Run()
		require.NoError(t, err)

		pty.ExpectMatch(fmt.Sprintf("Successfully deleted group %s", cliui.DefaultStyles.Keyword.Render(group.Name)))
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

		inv, conf := newCLI(
			t,
			"groups", "delete",
		)

		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.Error(t, err)
	})
}
