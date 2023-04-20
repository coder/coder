package coderd_test

import (
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
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

	t.Run("delete", func(t *testing.T) {
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

		err = client.DeleteWorkspaceProxyByID(ctx, proxyRes.Proxy.ID)
		require.NoError(t, err, "failed to delete workspace proxy")

		proxies, err := client.WorkspaceProxies(ctx)
		require.NoError(t, err)
		require.Len(t, proxies, 0)
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
	build := coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	workspace.LatestBuild = build

	// Connect an agent to the workspace
	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: slogtest.Make(t, nil).Named("agent").Leveled(slog.LevelDebug),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})

	coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	createProxyCtx := testutil.Context(t, testutil.WaitLong)
	proxyRes, err := client.CreateWorkspaceProxy(createProxyCtx, codersdk.CreateWorkspaceProxyRequest{
		Name:             namesgenerator.GetRandomName(1),
		Icon:             "/emojis/flag.png",
		URL:              "https://" + namesgenerator.GetRandomName(1) + ".com",
		WildcardHostname: "*.sub.example.com",
	})
	require.NoError(t, err)

	proxyClient := wsproxysdk.New(client.URL)
	proxyClient.SetSessionToken(proxyRes.ProxyToken)

	t.Run("BadAppRequest", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err = proxyClient.IssueSignedAppToken(ctx, workspaceapps.IssueTokenRequest{
			// Invalid request.
			AppRequest:   workspaceapps.Request{},
			SessionToken: client.SessionToken(),
		})
		require.Error(t, err)
	})

	goodRequest := workspaceapps.IssueTokenRequest{
		AppRequest: workspaceapps.Request{
			BasePath:      "/app",
			AccessMethod:  workspaceapps.AccessMethodTerminal,
			AgentNameOrID: build.Resources[0].Agents[0].ID.String(),
		},
		SessionToken: client.SessionToken(),
	}
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err = proxyClient.IssueSignedAppToken(ctx, goodRequest)
		require.NoError(t, err)
	})

	t.Run("OKHTML", func(t *testing.T) {
		t.Parallel()

		rw := httptest.NewRecorder()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, ok := proxyClient.IssueSignedAppTokenHTML(ctx, rw, goodRequest)
		if !assert.True(t, ok, "expected true") {
			resp := rw.Result()
			defer resp.Body.Close()
			dump, err := httputil.DumpResponse(resp, true)
			require.NoError(t, err)
			t.Log(string(dump))
		}
	})
}
