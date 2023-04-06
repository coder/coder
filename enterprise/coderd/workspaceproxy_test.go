package coderd_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/enterprise/wsproxy/wsproxysdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/testutil"
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
		_ = coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceProxy: 1,
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		proxyRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name:             namesgenerator.GetRandomName(1),
			Icon:             "/emojis/flag.png",
			URL:              "https://" + namesgenerator.GetRandomName(1) + ".com",
			WildcardHostname: "*.sub.example.com",
		})
		require.NoError(t, err)

		proxies, err := client.WorkspaceProxies(ctx)
		require.NoError(t, err)
		require.Len(t, proxies, 1)
		require.Equal(t, proxyRes.Proxy, proxies[0])
		require.NotEmpty(t, proxyRes.ProxyToken)
	})
}

func TestIssueSignedAppToken(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{
		string(codersdk.ExperimentMoons),
		"*",
	}

	db, pubsub := dbtestutil.NewDB(t)
	client := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues:         dv,
			Database:                 db,
			Pubsub:                   pubsub,
			IncludeProvisionerDaemon: true,
		},
	})

	user := coderdtest.CreateFirstUser(t, client)
	_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
		Features: license.Features{
			codersdk.FeatureWorkspaceProxy: 1,
		},
	})

	// Create a workspace + apps
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	ctx := testutil.Context(t, testutil.WaitLong)
	proxyRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
		Name:             namesgenerator.GetRandomName(1),
		Icon:             "/emojis/flag.png",
		URL:              "https://" + namesgenerator.GetRandomName(1) + ".com",
		WildcardHostname: "*.sub.example.com",
	})
	require.NoError(t, err)

	proxyClient := wsproxysdk.New(client.URL)
	proxyClient.SetSessionToken(proxyRes.ProxyToken)

	// TODO: "OK" test, requires a workspace and apps

	t.Run("BadAppRequest", func(t *testing.T) {
		t.Parallel()

		_, err = proxyClient.IssueSignedAppToken(ctx, wsproxysdk.IssueSignedAppTokenRequest{
			// Invalid request.
			AppRequest:   workspaceapps.Request{},
			SessionToken: client.SessionToken(),
		})
		require.Error(t, err)
	})

	t.Run("OK", func(t *testing.T) {
		_, err = proxyClient.IssueSignedAppToken(ctx, wsproxysdk.IssueSignedAppTokenRequest{
			AppRequest: workspaceapps.Request{
				BasePath:          "/app",
				AccessMethod:      workspaceapps.AccessMethodTerminal,
				UsernameOrID:      user.UserID.String(),
				WorkspaceAndAgent: workspace.ID.String(),
			},
			SessionToken: client.SessionToken(),
		})
		require.NoError(t, err)
	})
}
