package cli_test

import (
	"bufio"
	"context"
	"fmt"
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

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
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
	})

	t.Run("AnotherOrgByNameWithUser", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
				},
			},
		})
		anotherOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, anotherOrg.ID, rbac.RoleTemplateAdmin())
		inv, conf := newCLI(t, "provisionerd", "start", "--name", "org-daemon", "--org", anotherOrg.Name)
		clitest.SetupConfig(t, anotherClient, conf)
		pty := ptytest.New(t).Attach(inv)
		ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
		defer cancel()
		clitest.Start(t, inv)
		pty.ExpectMatchContext(ctx, "starting provisioner daemon")
	})

	t.Run("NoUserNoPSK", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})
		inv, conf := newCLI(t, "provisionerd", "start", "--name", "org-daemon")
		err := conf.URL().Write(client.URL.String())
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
		defer cancel()
		err = inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "must provide a pre-shared key or provisioner key when not authenticated as a user")
	})
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

	t.Run("ScopeUserAnotherOrg", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
				},
			},
		})
		anotherOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, anotherOrg.ID, rbac.RoleTemplateAdmin())
		inv, conf := newCLI(t, "provisionerd", "start", "--tag", "scope=user", "--name", "org-daemon", "--org", anotherOrg.ID.String())
		clitest.SetupConfig(t, anotherClient, conf)
		pty := ptytest.New(t).Attach(inv)
		ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
		defer cancel()
		clitest.Start(t, inv)
		pty.ExpectMatchContext(ctx, "starting provisioner daemon")

		var daemons []codersdk.ProvisionerDaemon
		var err error
		require.Eventually(t, func() bool {
			daemons, err = client.OrganizationProvisionerDaemons(ctx, anotherOrg.ID, nil)
			if err != nil {
				return false
			}
			return len(daemons) == 1
		}, testutil.WaitShort, testutil.IntervalSlow)
		assert.Equal(t, "org-daemon", daemons[0].Name)
		assert.Equal(t, provisionersdk.ScopeUser, daemons[0].Tags[provisionersdk.TagScope])
		assert.Equal(t, anotherUser.ID.String(), daemons[0].Tags[provisionersdk.TagOwner])
		assert.Equal(t, buildinfo.Version(), daemons[0].Version)
		assert.Equal(t, proto.CurrentVersion.String(), daemons[0].APIVersion)
	})
}

func TestProvisionerDaemon_ProvisionerKey(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
				},
			},
		})
		// nolint:gocritic // test
		res, err := client.CreateProvisionerKey(ctx, user.OrganizationID, codersdk.CreateProvisionerKeyRequest{
			Name: "dont-TEST-me",
		})
		require.NoError(t, err)
		inv, conf := newCLI(t, "provisionerd", "start", "--key", res.Key, "--name=matt-daemon")
		err = conf.URL().Write(client.URL.String())
		require.NoError(t, err)
		pty := ptytest.New(t).Attach(inv)
		clitest.Start(t, inv)
		pty.ExpectNoMatchBefore(ctx, "check entitlement", "starting provisioner daemon")
		pty.ExpectMatchContext(ctx, "matt-daemon")

		var daemons []codersdk.ProvisionerDaemon
		require.Eventually(t, func() bool {
			daemons, err = client.OrganizationProvisionerDaemons(ctx, user.OrganizationID, nil)
			if err != nil {
				return false
			}
			return len(daemons) == 1
		}, testutil.WaitShort, testutil.IntervalSlow)
		require.Equal(t, "matt-daemon", daemons[0].Name)
		require.Equal(t, provisionersdk.ScopeOrganization, daemons[0].Tags[provisionersdk.TagScope])
		require.Equal(t, buildinfo.Version(), daemons[0].Version)
		require.Equal(t, proto.CurrentVersion.String(), daemons[0].APIVersion)
	})

	t.Run("OKWithTags", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
				},
			},
		})
		//nolint:gocritic // ignore This client is operating as the owner user, which has unrestricted permissions
		res, err := client.CreateProvisionerKey(ctx, user.OrganizationID, codersdk.CreateProvisionerKeyRequest{
			Name: "dont-TEST-me",
			Tags: map[string]string{
				"tag1": "value1",
				"tag2": "value2",
			},
		})
		require.NoError(t, err)
		inv, conf := newCLI(t, "provisionerd", "start", "--key", res.Key, "--name=matt-daemon")
		err = conf.URL().Write(client.URL.String())
		require.NoError(t, err)
		pty := ptytest.New(t).Attach(inv)
		clitest.Start(t, inv)
		pty.ExpectNoMatchBefore(ctx, "check entitlement", "starting provisioner daemon")
		pty.ExpectMatchContext(ctx, `tags={"tag1":"value1","tag2":"value2"}`)

		var daemons []codersdk.ProvisionerDaemon
		require.Eventually(t, func() bool {
			daemons, err = client.OrganizationProvisionerDaemons(ctx, user.OrganizationID, nil)
			if err != nil {
				return false
			}
			return len(daemons) == 1
		}, testutil.WaitShort, testutil.IntervalSlow)
		require.Equal(t, "matt-daemon", daemons[0].Name)
		require.Equal(t, provisionersdk.ScopeOrganization, daemons[0].Tags[provisionersdk.TagScope])
		require.Equal(t, buildinfo.Version(), daemons[0].Version)
		require.Equal(t, proto.CurrentVersion.String(), daemons[0].APIVersion)
	})

	t.Run("NoProvisionerKeyFound", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
				},
			},
		})

		inv, conf := newCLI(t, "provisionerd", "start", "--key", "ThisKeyDoesNotExist", "--name=matt-daemon")
		err := conf.URL().Write(client.URL.String())
		require.NoError(t, err)
		err = inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "unable to get provisioner key details")
	})

	t.Run("NoPSK", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
				},
			},
		})
		// nolint:gocritic // test
		res, err := client.CreateProvisionerKey(ctx, user.OrganizationID, codersdk.CreateProvisionerKeyRequest{
			Name: "dont-TEST-me",
		})
		require.NoError(t, err)
		inv, conf := newCLI(t, "provisionerd", "start", "--psk", "provisionersftw", "--key", res.Key, "--name=matt-daemon")
		err = conf.URL().Write(client.URL.String())
		require.NoError(t, err)
		err = inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "cannot provide both provisioner key --key and pre-shared key --psk")
	})

	t.Run("NoTags", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
				},
			},
		})
		// nolint:gocritic // test
		res, err := client.CreateProvisionerKey(ctx, user.OrganizationID, codersdk.CreateProvisionerKeyRequest{
			Name: "dont-TEST-me",
		})
		require.NoError(t, err)
		inv, conf := newCLI(t, "provisionerd", "start", "--tag", "mykey=yourvalue", "--key", res.Key, "--name=matt-daemon")
		err = conf.URL().Write(client.URL.String())
		require.NoError(t, err)
		err = inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "cannot provide tags when using provisioner key")
	})

	t.Run("AnotherOrg", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			ProvisionerDaemonPSK: "provisionersftw",
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
				},
			},
		})
		anotherOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
		// nolint:gocritic // test
		res, err := client.CreateProvisionerKey(ctx, anotherOrg.ID, codersdk.CreateProvisionerKeyRequest{
			Name: "dont-TEST-me",
		})
		require.NoError(t, err)
		inv, conf := newCLI(t, "provisionerd", "start", "--key", res.Key, "--name=matt-daemon")
		err = conf.URL().Write(client.URL.String())
		require.NoError(t, err)
		pty := ptytest.New(t).Attach(inv)
		clitest.Start(t, inv)
		pty.ExpectNoMatchBefore(ctx, "check entitlement", "starting provisioner daemon")
		pty.ExpectMatchContext(ctx, "matt-daemon")
		var daemons []codersdk.ProvisionerDaemon
		require.Eventually(t, func() bool {
			daemons, err = client.OrganizationProvisionerDaemons(ctx, anotherOrg.ID, nil)
			if err != nil {
				return false
			}
			return len(daemons) == 1
		}, testutil.WaitShort, testutil.IntervalSlow)
		require.Equal(t, "matt-daemon", daemons[0].Name)
		require.Equal(t, provisionersdk.ScopeOrganization, daemons[0].Tags[provisionersdk.TagScope])
		require.Equal(t, buildinfo.Version(), daemons[0].Version)
		require.Equal(t, proto.CurrentVersion.String(), daemons[0].APIVersion)
	})
}

//nolint:paralleltest,tparallel // Test uses a static port.
func TestProvisionerDaemon_PrometheusEnabled(t *testing.T) {
	// Ephemeral ports have a tendency to conflict and fail with `bind: address already in use` error.
	// This workaround forces a static port for Prometheus that hopefully won't be used by other tests.
	prometheusPort := 32001

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
	}, testutil.WaitLong, testutil.IntervalSlow)
	require.Equal(t, "daemon-with-prometheus", daemons[0].Name)

	// Fetch metrics from Prometheus endpoint
	var req *http.Request
	var res *http.Response
	require.Eventually(t, func() bool {
		req, err = http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d", prometheusPort), nil)
		if err != nil {
			t.Logf("unable to create new HTTP request: %s", err.Error())
			return false
		}

		// nolint:bodyclose
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Logf("unable to call Prometheus endpoint: %s", err.Error())
			return false
		}
		return true
	}, testutil.WaitShort, testutil.IntervalMedium)
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
}
