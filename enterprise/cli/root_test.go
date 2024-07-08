package cli_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/enterprise/cli"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/serpent"
)

func newCLI(t *testing.T, args ...string) (*serpent.Invocation, config.Root) {
	var root cli.RootCmd
	cmd, err := root.Command(root.EnterpriseSubcommands())
	require.NoError(t, err)
	return clitest.NewWithCommand(t, cmd, args...)
}

func TestEnterpriseHandlersOK(t *testing.T) {
	t.Parallel()

	var root cli.RootCmd
	cmd, err := root.Command(root.EnterpriseSubcommands())
	require.NoError(t, err)

	clitest.HandlersOK(t, cmd)
}

func TestCheckWarnings(t *testing.T) {
	t.Parallel()

	t.Run("LicenseWarningForPrivilegedRoles", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				ExpiresAt: time.Now().Add(time.Hour * 24),
			},
		})

		inv, conf := newCLI(t, "list")

		var buf bytes.Buffer
		inv.Stderr = &buf
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // owners should see this

		err := inv.Run()
		require.NoError(t, err)

		require.Contains(t, buf.String(), "Your license expires in 1 day.")
	})

	t.Run("NoLicenseWarningForRegularUser", func(t *testing.T) {
		t.Parallel()
		adminClient, admin := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				ExpiresAt: time.Now().Add(time.Hour * 24),
			},
		})

		client, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)

		inv, conf := newCLI(t, "list")

		var buf bytes.Buffer
		inv.Stderr = &buf
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.NoError(t, err)

		require.NotContains(t, buf.String(), "Your license expires")
	})
}
