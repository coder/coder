package cli_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisionerd/proto"
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
	pty.ExpectNoMatchBefore(ctx, "check entitlement", "starting provisioner daemon")
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
	require.Equal(t, buildinfo.Version(), daemons[0].Version)
	require.Equal(t, proto.CurrentVersion.String(), daemons[0].APIVersion)
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
		assert.Equal(t, buildinfo.Version(), daemons[0].Version)
		assert.Equal(t, proto.CurrentVersion.String(), daemons[0].APIVersion)
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
		assert.Equal(t, buildinfo.Version(), daemons[0].Version)
		assert.Equal(t, proto.CurrentVersion.String(), daemons[0].APIVersion)
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
		assert.Equal(t, buildinfo.Version(), daemons[0].Version)
		assert.Equal(t, proto.CurrentVersion.String(), daemons[0].APIVersion)
	})

	t.Run("PrometheusEnabled", func(t *testing.T) {
		t.Parallel()

		prometheusPort := randomPort(t)

		// Configure CLI client
		client, admin := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleTemplateAdmin())
		inv, conf := newCLI(t, "provisionerd", "start", "--name", "daemon-with-prometheus", "--prometheus-enable", "--prometheus-address", fmt.Sprintf("127.0.0.1:%d", prometheusPort))
		clitest.SetupConfig(t, anotherClient, conf)
		pty := ptytest.New(t).Attach(inv)
		ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
		defer cancel()

		// Start "provisionerd" command
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
		require.Equal(t, "daemon-with-prometheus", daemons[0].Name)

		// Fetch metrics from Prometheus endpoint
		var res *http.Response
		require.Eventually(t, func() bool {
			req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d", prometheusPort), nil)
			assert.NoError(t, err)
			// nolint:bodyclose
			res, err = http.DefaultClient.Do(req)
			return err == nil
		}, testutil.WaitShort, testutil.IntervalFast)
		defer res.Body.Close()

		// Scan for metric patterns
		scanner := bufio.NewScanner(res.Body)
		hasOneDaemon := false
		hasGoStats := false
		hasPromHTTP := false
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "coderd_provisionerd_num_daemons 1") {
				hasOneDaemon = true
				continue
			}
			if strings.HasPrefix(scanner.Text(), "go_goroutines") {
				hasGoStats = true
				continue
			}
			if strings.HasPrefix(scanner.Text(), "promhttp_metric_handler_requests_total") {
				hasPromHTTP = true
				continue
			}
			t.Logf("scanned %s", scanner.Text())
		}
		require.NoError(t, scanner.Err())

		// Verify patterns
		require.True(t, hasOneDaemon, "should be one daemon running")
		require.True(t, hasGoStats, "Go stats are missing")
		require.True(t, hasPromHTTP, "Prometheus HTTP metrics are missing")
	})
}

// randomPort is a helper function to find a free random port, for instance to spawn Prometheus endpoint.
func randomPort(t *testing.T) int {
	random, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_ = random.Close()
	tcpAddr, valid := random.Addr().(*net.TCPAddr)
	require.True(t, valid)
	return tcpAddr.Port
}
