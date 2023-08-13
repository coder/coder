package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
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

	client, _ := coderdenttest.New(t, &coderdenttest.Options{
		ProvisionerDaemonPSK: "provisionersftw",
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		},
	})
	inv, conf := newCLI(t, "provisionerd", "start")
	clitest.SetupConfig(t, client, conf)
	pty := ptytest.New(t).Attach(inv)
	ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
	defer cancel()
	clitest.Start(t, inv)
	pty.ExpectMatchContext(ctx, "starting provisioner daemon")
}
