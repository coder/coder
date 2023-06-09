package coderd_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

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

func TestRegions(t *testing.T) {
	t.Parallel()

	const appHostname = "*.apps.coder.test"

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		db, pubsub := dbtestutil.NewDB(t)

		client := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AppHostname:      appHostname,
				Database:         db,
				Pubsub:           pubsub,
				DeploymentValues: dv,
			},
		})

		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		deploymentID, err := db.GetDeploymentID(ctx)
		require.NoError(t, err, "get deployment ID")

		regions, err := client.Regions(ctx)
		require.NoError(t, err)

		require.Len(t, regions, 1)
		require.NotEqual(t, uuid.Nil, regions[0].ID)
		require.Equal(t, regions[0].ID.String(), deploymentID)
		require.Equal(t, "primary", regions[0].Name)
		require.Equal(t, "Default", regions[0].DisplayName)
		require.NotEmpty(t, regions[0].IconURL)
		require.True(t, regions[0].Healthy)
		require.Equal(t, client.URL.String(), regions[0].PathAppURL)
		require.Equal(t, appHostname, regions[0].WildcardHostname)

		// Ensure the primary region ID is constant.
		regions2, err := client.Regions(ctx)
		require.NoError(t, err)
		require.Equal(t, regions[0].ID, regions2[0].ID)
	})

	t.Run("WithProxies", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		db, pubsub := dbtestutil.NewDB(t)

		client, closer, api := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AppHostname:      appHostname,
				Database:         db,
				Pubsub:           pubsub,
				DeploymentValues: dv,
			},
		})
		t.Cleanup(func() {
			_ = closer.Close()
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		deploymentID, err := db.GetDeploymentID(ctx)
		require.NoError(t, err, "get deployment ID")
		_ = coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceProxy: 1,
			},
		})

		const proxyName = "hello"
		_ = coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
			Name:        proxyName,
			AppHostname: appHostname + ".proxy",
		})
		proxy, err := db.GetWorkspaceProxyByName(ctx, proxyName)
		require.NoError(t, err)

		// Refresh proxy health.
		err = api.ProxyHealth.ForceUpdate(ctx)
		require.NoError(t, err)

		regions, err := client.Regions(ctx)
		require.NoError(t, err)
		require.Len(t, regions, 2)

		// Region 0 is the primary	require.Len(t, regions, 1)
		require.NotEqual(t, uuid.Nil, regions[0].ID)
		require.Equal(t, regions[0].ID.String(), deploymentID)
		require.Equal(t, "primary", regions[0].Name)
		require.Equal(t, "Default", regions[0].DisplayName)
		require.NotEmpty(t, regions[0].IconURL)
		require.True(t, regions[0].Healthy)
		require.Equal(t, client.URL.String(), regions[0].PathAppURL)
		require.Equal(t, appHostname, regions[0].WildcardHostname)

		// Region 1 is the proxy.
		require.NotEqual(t, uuid.Nil, regions[1].ID)
		require.Equal(t, proxy.ID, regions[1].ID)
		require.Equal(t, proxy.Name, regions[1].Name)
		require.Equal(t, proxy.DisplayName, regions[1].DisplayName)
		require.Equal(t, proxy.Icon, regions[1].IconURL)
		require.True(t, regions[1].Healthy)
		require.Equal(t, proxy.Url, regions[1].PathAppURL)
		require.Equal(t, proxy.WildcardHostname, regions[1].WildcardHostname)
	})

	t.Run("RequireAuth", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AppHostname:      appHostname,
				DeploymentValues: dv,
			},
		})
		_ = coderdtest.CreateFirstUser(t, client)

		unauthedClient := codersdk.New(client.URL)
		regions, err := unauthedClient.Regions(ctx)
		require.Error(t, err)
		require.Empty(t, regions)
	})

	t.Run("GoingAway", func(t *testing.T) {
		t.Skip("This is flakey in CI because it relies on internal go routine timing. Should refactor.")
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentMoons),
			"*",
		}

		db, pubsub := dbtestutil.NewDB(t)

		ctx := testutil.Context(t, testutil.WaitLong)

		client, closer, api := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				AppHostname:      appHostname,
				Database:         db,
				Pubsub:           pubsub,
				DeploymentValues: dv,
			},
			// The interval is set to 1 hour so the proxy health
			// check will never happen manually. All checks will be
			// forced updates.
			ProxyHealthInterval: time.Hour,
		})
		t.Cleanup(func() {
			_ = closer.Close()
		})
		_ = coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceProxy: 1,
			},
		})

		const proxyName = "testproxy"
		proxy := coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
			Name: proxyName,
		})
		_ = proxy

		require.Eventuallyf(t, func() bool {
			proxy, err := client.WorkspaceProxyByName(ctx, proxyName)
			if err != nil {
				// We are testing the going away, not the initial healthy.
				// Just force an update to change this to healthy.
				_ = api.ProxyHealth.ForceUpdate(ctx)
				return false
			}
			return proxy.Status.Status == codersdk.ProxyHealthy
		}, testutil.WaitShort, testutil.IntervalFast, "proxy never became healthy")

		_ = proxy.Close()
		// The proxy should tell the primary on close that is is no longer healthy.
		require.Eventuallyf(t, func() bool {
			proxy, err := client.WorkspaceProxyByName(ctx, proxyName)
			if err != nil {
				return false
			}
			return proxy.Status.Status == codersdk.ProxyUnhealthy
		}, testutil.WaitShort, testutil.IntervalFast, "proxy never became unhealthy after close")
	})
}

func TestWorkspaceProxyCRUD(t *testing.T) {
	t.Parallel()

	t.Run("CreateAndUpdate", func(t *testing.T) {
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
			Name: namesgenerator.GetRandomName(1),
			Icon: "/emojis/flag.png",
		})
		require.NoError(t, err)

		found, err := client.WorkspaceProxyByID(ctx, proxyRes.Proxy.ID)
		require.NoError(t, err)
		// This will be different, so set it to the same
		found.Status = proxyRes.Proxy.Status
		require.Equal(t, proxyRes.Proxy, found, "expected proxy")
		require.NotEmpty(t, proxyRes.ProxyToken)

		// Update the proxy
		expName := namesgenerator.GetRandomName(1)
		expDisplayName := namesgenerator.GetRandomName(1)
		expIcon := namesgenerator.GetRandomName(1)
		_, err = client.PatchWorkspaceProxy(ctx, codersdk.PatchWorkspaceProxy{
			ID:          proxyRes.Proxy.ID,
			Name:        expName,
			DisplayName: expDisplayName,
			Icon:        expIcon,
		})
		require.NoError(t, err, "expected no error updating proxy")

		found, err = client.WorkspaceProxyByID(ctx, proxyRes.Proxy.ID)
		require.NoError(t, err)
		require.Equal(t, expName, found.Name, "name")
		require.Equal(t, expDisplayName, found.DisplayName, "display name")
		require.Equal(t, expIcon, found.Icon, "icon")
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
		proxyRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name: namesgenerator.GetRandomName(1),
			Icon: "/emojis/flag.png",
		})
		require.NoError(t, err)

		err = client.DeleteWorkspaceProxyByID(ctx, proxyRes.Proxy.ID)
		require.NoError(t, err, "failed to delete workspace proxy")

		proxies, err := client.WorkspaceProxies(ctx)
		require.NoError(t, err)
		// Default proxy is always there
		require.Len(t, proxies, 1)
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
		Name: namesgenerator.GetRandomName(1),
		Icon: "/emojis/flag.png",
	})
	require.NoError(t, err)

	t.Run("BadAppRequest", func(t *testing.T) {
		t.Parallel()
		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(proxyRes.ProxyToken)

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
		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(proxyRes.ProxyToken)

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err = proxyClient.IssueSignedAppToken(ctx, goodRequest)
		require.NoError(t, err)
	})

	t.Run("OKHTML", func(t *testing.T) {
		t.Parallel()
		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(proxyRes.ProxyToken)

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

func TestReconnectingPTYSignedToken(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{
		string(codersdk.ExperimentMoons),
		"*",
	}

	db, pubsub := dbtestutil.NewDB(t)
	client, closer, api := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues:         dv,
			Database:                 db,
			Pubsub:                   pubsub,
			IncludeProvisionerDaemon: true,
		},
	})
	t.Cleanup(func() {
		closer.Close()
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
	agentID := build.Resources[0].Agents[0].ID
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

	proxyURL, err := url.Parse(fmt.Sprintf("https://%s.com", namesgenerator.GetRandomName(1)))
	require.NoError(t, err)

	_ = coderdenttest.NewWorkspaceProxy(t, api, client, &coderdenttest.ProxyOptions{
		Name:        namesgenerator.GetRandomName(1),
		ProxyURL:    proxyURL,
		AppHostname: "*.sub.example.com",
	})

	u, err := url.Parse(proxyURL.String())
	require.NoError(t, err)
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = fmt.Sprintf("/api/v2/workspaceagents/%s/pty", agentID.String())

	t.Run("Validate", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     "",
			AgentID: uuid.Nil,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("BadURL", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     ":",
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Response.Message, "Invalid URL")
	})

	t.Run("BadURL", func(t *testing.T) {
		t.Parallel()

		u := *u
		u.Scheme = "ftp"

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Response.Message, "Invalid URL")
		require.Contains(t, sdkErr.Response.Detail, "scheme")
	})

	t.Run("BadURLPath", func(t *testing.T) {
		t.Parallel()

		u := *u
		u.Path = "/hello"

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Response.Message, "Invalid URL")
		require.Contains(t, sdkErr.Response.Detail, "The provided URL is not a valid reconnecting PTY endpoint URL")
	})

	t.Run("BadHostname", func(t *testing.T) {
		t.Parallel()

		u := *u
		u.Host = "badhostname.com"

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Response.Message, "Invalid hostname in URL")
	})

	t.Run("NoToken", func(t *testing.T) {
		t.Parallel()

		unauthedClient := codersdk.New(client.URL)

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := unauthedClient.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
	})

	t.Run("NoPermissions", func(t *testing.T) {
		t.Parallel()

		userClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := userClient.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.Error(t, err)
		require.Empty(t, res)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.IssueReconnectingPTYSignedToken(ctx, codersdk.IssueReconnectingPTYSignedTokenRequest{
			URL:     u.String(),
			AgentID: agentID,
		})
		require.NoError(t, err)
		require.NotEmpty(t, res.SignedToken)

		// The token is validated in the apptest suite, so we don't need to
		// validate it here.
	})
}
