package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

const (
	proxyTestAgentName   = "agent-name"
	proxyTestAppName     = "example"
	proxyTestAppQuery    = "query=true"
	proxyTestAppBody     = "hello world"
	proxyTestFakeAppName = "fake"
)

// setupProxyTest creates a workspace with an agent and some apps. It returns a
// codersdk client, the workspace, and the port number the test listener is
// running on.
func setupProxyTest(t *testing.T, workspaceMutators ...func(*codersdk.CreateWorkspaceRequest)) (*codersdk.Client, uuid.UUID, codersdk.Workspace, uint16) {
	// #nosec
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	server := http.Server{
		ReadHeaderTimeout: time.Minute,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := r.Cookie(codersdk.SessionTokenKey)
			assert.ErrorIs(t, err, http.ErrNoCookie)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(proxyTestAppBody))
		}),
	}
	t.Cleanup(func() {
		_ = server.Close()
		_ = ln.Close()
	})
	go server.Serve(ln)
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon:    true,
		AgentStatsRefreshInterval:   time.Millisecond * 100,
		MetricsCacheRefreshInterval: time.Millisecond * 100,
	})
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:           echo.ParseComplete,
		ProvisionDryRun: echo.ProvisionComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: proxyTestAgentName,
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
							Apps: []*proto.App{
								{
									Name: proxyTestAppName,
									Url:  fmt.Sprintf("http://127.0.0.1:%d?%s", tcpAddr.Port, proxyTestAppQuery),
								}, {
									Name: proxyTestFakeAppName,
									// Hopefully this IP and port doesn't exist.
									Url: "http://127.1.0.1:65535",
								},
							},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, workspaceMutators...)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	agentClient := codersdk.New(client.URL)
	agentClient.SessionToken = authToken
	agentCloser := agent.New(agent.Options{
		FetchMetadata:              agentClient.WorkspaceAgentMetadata,
		CoordinatorDialer:          agentClient.ListenWorkspaceAgentTailnet,
		Logger:                     slogtest.Make(t, nil).Named("agent"),
		StatsReporter:              agentClient.AgentReportStats,
		WorkspaceAppHealthReporter: func(context.Context) {},
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)

	// Configure the HTTP client to not follow redirects and to route all
	// requests regardless of hostname to the coderd test server.
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	transport := defaultTransport.Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, client.URL.Host)
	}
	client.HTTPClient.Transport = transport

	return client, user.OrganizationID, workspace, uint16(tcpAddr.Port)
}

func TestWorkspaceAppsProxyPath(t *testing.T) {
	t.Parallel()
	client, orgID, workspace, _ := setupProxyTest(t)

	t.Run("LoginWithoutAuth", func(t *testing.T) {
		t.Parallel()
		client := codersdk.New(client.URL)
		client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodGet, "/@me/"+workspace.Name+"/apps/example", nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		loc, err := resp.Location()
		require.NoError(t, err)
		require.True(t, loc.Query().Has("message"))
		require.True(t, loc.Query().Has("redirect"))
	})

	t.Run("NoAccessShould404", func(t *testing.T) {
		t.Parallel()

		userClient := coderdtest.CreateAnotherUser(t, client, orgID, rbac.RoleMember())
		userClient.HTTPClient.CheckRedirect = client.HTTPClient.CheckRedirect
		userClient.HTTPClient.Transport = client.HTTPClient.Transport

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := userClient.Request(ctx, http.MethodGet, "/@me/"+workspace.Name+"/apps/example", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("RedirectsWithSlash", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodGet, "/@me/"+workspace.Name+"/apps/example", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	})

	t.Run("RedirectsWithQuery", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodGet, "/@me/"+workspace.Name+"/apps/example/", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		loc, err := resp.Location()
		require.NoError(t, err)
		require.Equal(t, proxyTestAppQuery, loc.RawQuery)
	})

	t.Run("Proxies", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodGet, "/@me/"+workspace.Name+"/apps/example/?"+proxyTestAppQuery, nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, proxyTestAppBody, string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("ProxyError", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodGet, "/@me/"+workspace.Name+"/apps/fake/", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		// this is 200 OK because it returns a dashboard page
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestWorkspaceAppsProxySubdomain(t *testing.T) {
	t.Parallel()
	client, orgID, workspace, port := setupProxyTest(t)

	// proxyURL generates a URL for the proxy subdomain. The default path is a
	// slash.
	proxyURL := func(t *testing.T, appNameOrPort interface{}, pathAndQuery ...string) string {
		t.Helper()

		var (
			appName string
			port    uint16
		)
		if val, ok := appNameOrPort.(string); ok {
			appName = val
		} else {
			port, ok = appNameOrPort.(uint16)
			require.True(t, ok)
		}

		me, err := client.User(context.Background(), codersdk.Me)
		require.NoError(t, err, "get current user details")

		hostname := httpapi.ApplicationURL{
			AppName:       appName,
			Port:          port,
			AgentName:     proxyTestAgentName,
			WorkspaceName: workspace.Name,
			Username:      me.Username,
			BaseHostname:  "test.coder.com",
		}.String()

		actualPath := "/"
		query := ""
		if len(pathAndQuery) > 0 {
			actualPath = pathAndQuery[0]
		}
		if len(pathAndQuery) > 1 {
			query = pathAndQuery[1]
		}

		return (&url.URL{
			Scheme:   "http",
			Host:     hostname,
			Path:     actualPath,
			RawQuery: query,
		}).String()
	}

	t.Run("LoginWithoutAuth", func(t *testing.T) {
		t.Parallel()
		unauthedClient := codersdk.New(client.URL)
		unauthedClient.HTTPClient.CheckRedirect = client.HTTPClient.CheckRedirect
		unauthedClient.HTTPClient.Transport = client.HTTPClient.Transport

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := unauthedClient.Request(ctx, http.MethodGet, proxyURL(t, proxyTestAppName), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

		loc, err := resp.Location()
		require.NoError(t, err)
		require.True(t, loc.Query().Has("message"))
		require.False(t, loc.Query().Has("redirect"))

		expectedURL := *client.URL
		expectedURL.Path = "/login"
		loc.RawQuery = ""
		require.Equal(t, &expectedURL, loc)
	})

	t.Run("NoAccessShould401", func(t *testing.T) {
		t.Parallel()

		userClient := coderdtest.CreateAnotherUser(t, client, orgID, rbac.RoleMember())
		userClient.HTTPClient.CheckRedirect = client.HTTPClient.CheckRedirect
		userClient.HTTPClient.Transport = client.HTTPClient.Transport

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := userClient.Request(ctx, http.MethodGet, proxyURL(t, proxyTestAppName), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("RedirectsWithSlash", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		slashlessURL := proxyURL(t, proxyTestAppName, "")
		resp, err := client.Request(ctx, http.MethodGet, slashlessURL, nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

		loc, err := resp.Location()
		require.NoError(t, err)
		require.Equal(t, slashlessURL+"/?"+proxyTestAppQuery, loc.String())
	})

	t.Run("RedirectsWithQuery", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		querylessURL := proxyURL(t, proxyTestAppName, "/", "")
		resp, err := client.Request(ctx, http.MethodGet, querylessURL, nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

		loc, err := resp.Location()
		require.NoError(t, err)
		require.Equal(t, proxyTestAppQuery, loc.RawQuery)
	})

	t.Run("Proxies", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodGet, proxyURL(t, proxyTestAppName, "/", proxyTestAppQuery), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, proxyTestAppBody, string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("ProxiesPort", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodGet, proxyURL(t, port, "/", proxyTestAppQuery), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, proxyTestAppBody, string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("ProxyError", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodGet, proxyURL(t, proxyTestFakeAppName, "/", ""), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	})
}
