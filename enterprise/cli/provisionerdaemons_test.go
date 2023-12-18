package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisionersdk"
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
	inv, conf := newCLI(t, "provisionerd", "start", "--psk=provisionersftw", "--name=matt-daemon")
	err := conf.URL().Write(client.URL.String())
	require.NoError(t, err)
	pty := ptytest.New(t).Attach(inv)
	ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
	defer cancel()
	clitest.Start(t, inv)
	pty.ExpectMatchContext(ctx, "starting provisioner daemon")
	pty.ExpectMatchContext(ctx, "matt-daemon")

	var daemons []codersdk.ProvisionerDaemon
	require.Eventually(t, func() bool {
		daemons, err = client.ProvisionerDaemons(ctx)
		if err != nil {
			return false
		}
		return len(daemons) == 1
	}, testutil.WaitShort, testutil.IntervalSlow)
	require.Equal(t, "matt-daemon", daemons[0].Name)
	require.Equal(t, provisionersdk.ScopeOrganization, daemons[0].Tags[provisionersdk.TagScope])
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
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		inv, conf := newCLI(t, "provisionerd", "start", "--tag", "scope=user", "--name", "my-daemon")
		clitest.SetupConfig(t, anotherClient, conf)
		pty := ptytest.New(t).Attach(inv)
		ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
		defer cancel()
		clitest.Start(t, inv)
		pty.ExpectMatchContext(ctx, "starting provisioner daemon")

		var daemons []codersdk.ProvisionerDaemon
		var err error
		require.Eventually(t, func() bool {
			daemons, err = client.ProvisionerDaemons(ctx)
			if err != nil {
				return false
			}
			return len(daemons) == 1
		}, testutil.WaitShort, testutil.IntervalSlow)
		assert.Equal(t, "my-daemon", daemons[0].Name)
		assert.Equal(t, provisionersdk.ScopeUser, daemons[0].Tags[provisionersdk.TagScope])
		assert.Equal(t, anotherUser.ID.String(), daemons[0].Tags[provisionersdk.TagOwner])
	})

	t.Run("ScopeAnotherUser", func(t *testing.T) {
		t.Parallel()
		client, admin := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		inv, conf := newCLI(t, "provisionerd", "start", "--tag", "scope=user", "--tag", "owner="+admin.UserID.String(), "--name", "my-daemon")
		clitest.SetupConfig(t, anotherClient, conf)
		pty := ptytest.New(t).Attach(inv)
		ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
		defer cancel()
		clitest.Start(t, inv)
		pty.ExpectMatchContext(ctx, "starting provisioner daemon")

		var daemons []codersdk.ProvisionerDaemon
		var err error
		require.Eventually(t, func() bool {
			daemons, err = client.ProvisionerDaemons(ctx)
			if err != nil {
				return false
			}
			return len(daemons) == 1
		}, testutil.WaitShort, testutil.IntervalSlow)
		assert.Equal(t, "my-daemon", daemons[0].Name)
		assert.Equal(t, provisionersdk.ScopeUser, daemons[0].Tags[provisionersdk.TagScope])
		// This should get clobbered to the user who started the daemon.
		assert.Equal(t, anotherUser.ID.String(), daemons[0].Tags[provisionersdk.TagOwner])
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
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleTemplateAdmin())
		inv, conf := newCLI(t, "provisionerd", "start", "--tag", "scope=organization", "--name", "org-daemon")
		clitest.SetupConfig(t, anotherClient, conf)
		pty := ptytest.New(t).Attach(inv)
		ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
		defer cancel()
		clitest.Start(t, inv)
		pty.ExpectMatchContext(ctx, "starting provisioner daemon")

		var daemons []codersdk.ProvisionerDaemon
		var err error
		require.Eventually(t, func() bool {
			daemons, err = client.ProvisionerDaemons(ctx)
			if err != nil {
				return false
			}
			return len(daemons) == 1
		}, testutil.WaitShort, testutil.IntervalSlow)
		assert.Equal(t, "org-daemon", daemons[0].Name)
		assert.Equal(t, provisionersdk.ScopeOrganization, daemons[0].Tags[provisionersdk.TagScope])
	})
}
