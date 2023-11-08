package cli_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func Test_ProxyCRUD(t *testing.T) {
	t.Parallel()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceProxy: 1,
				},
			},
		})

		expectedName := "test-proxy"
		ctx := testutil.Context(t, testutil.WaitLong)
		inv, conf := newCLI(
			t,
			"wsproxy", "create",
			"--name", expectedName,
			"--display-name", "Test Proxy",
			"--icon", "/emojis/1f4bb.png",
			"--only-token",
		)

		pty := ptytest.New(t)
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // create wsproxy requires owner

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		line := pty.ReadLine(ctx)
		parts := strings.Split(line, ":")
		require.Len(t, parts, 2, "expected 2 parts")
		_, err = uuid.Parse(parts[0])
		require.NoError(t, err, "expected token to be a uuid")

		// Fetch proxies and check output
		inv, conf = newCLI(
			t,
			"wsproxy", "ls",
		)

		pty = ptytest.New(t)
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		pty.ExpectMatch(expectedName)

		// Also check via the api
		proxies, err := client.WorkspaceProxies(ctx) //nolint:gocritic // requires owner
		require.NoError(t, err, "failed to get workspace proxies")
		// Include primary
		require.Len(t, proxies.Regions, 2, "expected 1 proxy")
		found := false
		for _, proxy := range proxies.Regions {
			if proxy.Name == expectedName {
				found = true
			}
		}
		require.True(t, found, "expected proxy to be found")
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspaceProxy: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		expectedName := "test-proxy"
		_, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name:        expectedName,
			DisplayName: "Test Proxy",
			Icon:        "/emojis/us.png",
		})
		require.NoError(t, err, "failed to create workspace proxy")

		inv, conf := newCLI(
			t,
			"wsproxy", "delete", "-y", expectedName,
		)

		pty := ptytest.New(t)
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		proxies, err := client.WorkspaceProxies(ctx) //nolint:gocritic // requires owner
		require.NoError(t, err, "failed to get workspace proxies")
		require.Len(t, proxies.Regions, 1, "expected only primary proxy")
	})
}
