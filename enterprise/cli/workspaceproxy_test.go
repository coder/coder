package cli_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
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

		client := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
		})
		_ = coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceProxy: 1,
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
		clitest.SetupConfig(t, client, conf)

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
		clitest.SetupConfig(t, client, conf)

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		pty.ExpectMatch(expectedName)

		// Also check via the api
		proxies, err := client.WorkspaceProxies(ctx)
		require.NoError(t, err, "failed to get workspace proxies")
		require.Len(t, proxies, 1, "expected 1 proxy")
		require.Equal(t, expectedName, proxies[0].Name, "expected proxy name to match")
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		client := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
		})
		_ = coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceProxy: 1,
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
			"wsproxy", "delete", expectedName,
		)

		pty := ptytest.New(t)
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, client, conf)

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		proxies, err := client.WorkspaceProxies(ctx)
		require.NoError(t, err, "failed to get workspace proxies")
		require.Len(t, proxies, 0, "expected no proxies")
	})
}
