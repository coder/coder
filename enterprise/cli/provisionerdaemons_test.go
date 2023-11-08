package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerDaemon_PSK(t *testing.T) {
	t.Parallel()

	client, _ := coderdenttest.New(t, &coderdenttest.Options{
		ProvisionerDaemonPSK: "provisionersftw",
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		},
	})
	inv, conf := newCLI(t, "provisionerd", "start", "--psk=provisionersftw")
	err := conf.URL().Write(client.URL.String())
	require.NoError(t, err)
	pty := ptytest.New(t).Attach(inv)
	ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
	defer cancel()
	clitest.Start(t, inv)
	pty.ExpectMatchContext(ctx, "starting provisioner daemon")
}

func TestProvisionerDaemon_SessionToken(t *testing.T) {
	t.Parallel()
	t.Run("ScopeUser", func(t *testing.T) {
		t.Parallel()
		client, admin := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		inv, conf := newCLI(t, "provisionerd", "start", "--tag", "scope=user")
		clitest.SetupConfig(t, anotherClient, conf)
		pty := ptytest.New(t).Attach(inv)
		ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
		defer cancel()
		clitest.Start(t, inv)
		pty.ExpectMatchContext(ctx, "starting provisioner daemon")
	})

	t.Run("ScopeOrg", func(t *testing.T) {
		t.Parallel()
		client, admin := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		inv, conf := newCLI(t, "provisionerd", "start", "--tag", "scope=organization")
		clitest.SetupConfig(t, anotherClient, conf)
		pty := ptytest.New(t).Attach(inv)
		ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
		defer cancel()
		clitest.Start(t, inv)
		pty.ExpectMatchContext(ctx, "starting provisioner daemon")
	})
}
