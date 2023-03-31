package coderd_test

import (
	"testing"

	"github.com/moby/moby/pkg/namesgenerator"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/testutil"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceProxyCRUD(t *testing.T) {
	t.Parallel()

	t.Run("create", func(t *testing.T) {
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
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceProxy: 1,
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		proxy, err := client.CreateWorkspaceProxy(ctx, user.OrganizationID, codersdk.CreateWorkspaceProxyRequest{
			Name:        namesgenerator.GetRandomName(1),
			Icon:        "/emojis/flag.png",
			URL:         "https://" + namesgenerator.GetRandomName(1) + ".com",
			WildcardURL: "https://*." + namesgenerator.GetRandomName(1) + ".com",
		})
		require.NoError(t, err)

		proxies, err := client.WorkspaceProxiesByOrganization(ctx, user.OrganizationID)
		require.NoError(t, err)
		require.Len(t, proxies, 1)
		require.Equal(t, proxy, proxies[0])
	})
}
