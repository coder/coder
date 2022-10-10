package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
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

	proxyTestSubdomain = "test.coder.com"
)

func TestGetAppHost(t *testing.T) {
	t.Parallel()

	cases := []string{"", "test.coder.com"}
	for _, c := range cases {
		c := c
		name := c
		if name == "" {
			name = "Empty"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, &coderdtest.Options{
				AppHostname: c,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// Should not leak to unauthenticated users.
			host, err := client.GetAppHost(ctx)
			require.Error(t, err)
			require.Equal(t, "", host.Host)

			_ = coderdtest.CreateFirstUser(t, client)
			host, err = client.GetAppHost(ctx)
			require.NoError(t, err)
			require.Equal(t, c, host.Host)
		})
	}
}

// setupProxyTest creates a workspace with an agent and some apps. It returns a
// codersdk client, the first user, the workspace, and the port number the test
// listener is running on.
func setupProxyTest(t *testing.T, workspaceMutators ...func(*codersdk.CreateWorkspaceRequest)) (*codersdk.Client, codersdk.CreateFirstUserResponse, codersdk.Workspace, uint16) {
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
		AppHostname:                 proxyTestSubdomain,
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
		FetchMetadata:     agentClient.WorkspaceAgentMetadata,
		CoordinatorDialer: agentClient.ListenWorkspaceAgentTailnet,
		Logger:            slogtest.Make(t, nil).Named("agent"),
		StatsReporter:     agentClient.AgentReportStats,
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

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

	return client, user, workspace, uint16(tcpAddr.Port)
}

func TestWorkspaceAppsProxyPath(t *testing.T) {
	t.Parallel()
	client, firstUser, workspace, _ := setupProxyTest(t)

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

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		loc, err := resp.Location()
		require.NoError(t, err)
		require.True(t, loc.Query().Has("message"))
		require.True(t, loc.Query().Has("redirect"))
	})

	t.Run("NoAccessShould404", func(t *testing.T) {
		t.Parallel()

		userClient := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID, rbac.RoleMember())
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
		require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	})
}

func TestWorkspaceApplicationAuth(t *testing.T) {
	t.Parallel()

	// The OK test checks the entire end-to-end flow of authentication.
	t.Run("End-to-End", func(t *testing.T) {
		t.Parallel()

		client, firstUser, workspace, _ := setupProxyTest(t)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Get the current user and API key.
		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		currentAPIKey, err := client.GetAPIKey(ctx, firstUser.UserID.String(), strings.Split(client.SessionToken, "-")[0])
		require.NoError(t, err)

		// Try to load the application without authentication.
		subdomain := fmt.Sprintf("%s--%s--%s--%s", proxyTestAppName, proxyTestAgentName, workspace.Name, user.Username)
		u, err := url.Parse(fmt.Sprintf("http://%s.%s/test", subdomain, proxyTestSubdomain))
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		require.NoError(t, err)
		resp, err := client.HTTPClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		// Check that the Location is correct.
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		gotLocation, err := resp.Location()
		require.NoError(t, err)
		require.Equal(t, client.URL.Host, gotLocation.Host)
		require.Equal(t, "/api/v2/applications/auth-redirect", gotLocation.Path)
		require.Equal(t, u.String(), gotLocation.Query().Get("redirect_uri"))

		// Load the application auth-redirect endpoint.
		resp, err = client.Request(ctx, http.MethodGet, "/api/v2/applications/auth-redirect", nil, codersdk.WithQueryParam(
			"redirect_uri", u.String(),
		))
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		gotLocation, err = resp.Location()
		require.NoError(t, err)

		// Copy the query parameters and then check equality.
		u.RawQuery = gotLocation.RawQuery
		require.Equal(t, u, gotLocation)

		// Verify the API key is set.
		var encryptedAPIKey string
		for k, v := range gotLocation.Query() {
			// The query parameter may change dynamically in the future and is
			// not exported, so we just use a fuzzy check instead.
			if strings.Contains(k, "api_key") {
				encryptedAPIKey = v[0]
			}
		}
		require.NotEmpty(t, encryptedAPIKey, "no API key was set in the query parameters")

		// Decrypt the API key by following the request.
		t.Log("navigating to: ", gotLocation.String())
		req, err = http.NewRequestWithContext(ctx, "GET", gotLocation.String(), nil)
		require.NoError(t, err)
		resp, err = client.HTTPClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		cookies := resp.Cookies()
		require.Len(t, cookies, 1)
		apiKey := cookies[0].Value

		// Fetch the API key.
		apiKeyInfo, err := client.GetAPIKey(ctx, firstUser.UserID.String(), strings.Split(apiKey, "-")[0])
		require.NoError(t, err)
		require.Equal(t, user.ID, apiKeyInfo.UserID)
		require.Equal(t, codersdk.LoginTypePassword, apiKeyInfo.LoginType)
		require.WithinDuration(t, currentAPIKey.ExpiresAt, apiKeyInfo.ExpiresAt, 5*time.Second)

		// Verify the API key permissions
		appClient := codersdk.New(client.URL)
		appClient.SessionToken = apiKey
		appClient.HTTPClient.CheckRedirect = client.HTTPClient.CheckRedirect
		appClient.HTTPClient.Transport = client.HTTPClient.Transport

		var (
			canCreateApplicationConnect = "can-create-application_connect"
			canReadUserMe               = "can-read-user-me"
		)
		authRes, err := appClient.CheckAuthorization(ctx, codersdk.AuthorizationRequest{
			Checks: map[string]codersdk.AuthorizationCheck{
				canCreateApplicationConnect: {
					Object: codersdk.AuthorizationObject{
						ResourceType:   "application_connect",
						OwnerID:        "me",
						OrganizationID: firstUser.OrganizationID.String(),
					},
					Action: "create",
				},
				canReadUserMe: {
					Object: codersdk.AuthorizationObject{
						ResourceType: "user",
						OwnerID:      "me",
						ResourceID:   firstUser.UserID.String(),
					},
					Action: "read",
				},
			},
		})
		require.NoError(t, err)

		require.True(t, authRes[canCreateApplicationConnect])
		require.False(t, authRes[canReadUserMe])

		// Load the application page with the API key set.
		gotLocation, err = resp.Location()
		require.NoError(t, err)
		t.Log("navigating to: ", gotLocation.String())
		req, err = http.NewRequestWithContext(ctx, "GET", gotLocation.String(), nil)
		require.NoError(t, err)
		req.Header.Set(codersdk.SessionCustomHeader, apiKey)
		resp, err = client.HTTPClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("VerifyRedirectURI", func(t *testing.T) {
		t.Parallel()

		client, _, _, _ := setupProxyTest(t)

		cases := []struct {
			name            string
			redirectURI     string
			status          int
			messageContains string
		}{
			{
				name:            "NoRedirectURI",
				redirectURI:     "",
				status:          http.StatusBadRequest,
				messageContains: "Missing redirect_uri query parameter",
			},
			{
				name:            "InvalidURI",
				redirectURI:     "not a url",
				status:          http.StatusBadRequest,
				messageContains: "Invalid redirect_uri query parameter",
			},
			{
				name:            "NotMatchAppHostname",
				redirectURI:     "https://app--agent--workspace--user.not-a-match.com",
				status:          http.StatusBadRequest,
				messageContains: "The redirect_uri query parameter must be a valid app subdomain",
			},
			{
				name:            "InvalidAppURL",
				redirectURI:     "https://not-an-app." + proxyTestSubdomain,
				status:          http.StatusBadRequest,
				messageContains: "The redirect_uri query parameter must be a valid app subdomain",
			},
		}

		for _, c := range cases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
				defer cancel()

				resp, err := client.Request(ctx, http.MethodGet, "/api/v2/applications/auth-redirect", nil,
					codersdk.WithQueryParam("redirect_uri", c.redirectURI),
				)
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, http.StatusBadRequest, resp.StatusCode)
			})
		}
	})
}

// This test ensures that the subdomain handler does nothing if --app-hostname
// is not set by the admin.
func TestWorkspaceAppsProxySubdomainPassthrough(t *testing.T) {
	t.Parallel()

	// No AppHostname set.
	client := coderdtest.New(t, &coderdtest.Options{
		AppHostname: "",
	})
	firstUser := coderdtest.CreateFirstUser(t, client)

	// Configure the HTTP client to always route all requests to the coder test
	// server.
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	transport := defaultTransport.Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, client.URL.Host)
	}
	client.HTTPClient.Transport = transport

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	uri := fmt.Sprintf("http://app--agent--workspace--username.%s/api/v2/users/me", proxyTestSubdomain)
	resp, err := client.Request(ctx, http.MethodGet, uri, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should look like a codersdk.User response.
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var user codersdk.User
	err = json.NewDecoder(resp.Body).Decode(&user)
	require.NoError(t, err)
	require.Equal(t, firstUser.UserID, user.ID)
}

// This test ensures that the subdomain handler blocks the request if it looks
// like a workspace app request but the configured app hostname differs from the
// request, or the request is not a valid app subdomain but the hostname
// matches.
func TestWorkspaceAppsProxySubdomainBlocked(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T, appHostname string) *codersdk.Client {
		client := coderdtest.New(t, &coderdtest.Options{
			AppHostname: appHostname,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		// Configure the HTTP client to always route all requests to the coder test
		// server.
		defaultTransport, ok := http.DefaultTransport.(*http.Transport)
		require.True(t, ok)
		transport := defaultTransport.Clone()
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, client.URL.Host)
		}
		client.HTTPClient.Transport = transport

		return client
	}

	t.Run("NotMatchingHostname", func(t *testing.T) {
		t.Parallel()
		client := setup(t, "test."+proxyTestSubdomain)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		uri := fmt.Sprintf("http://app--agent--workspace--username.%s/api/v2/users/me", proxyTestSubdomain)
		resp, err := client.Request(ctx, http.MethodGet, uri, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should have an error response.
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), "does not accept application requests on this hostname")
	})

	t.Run("InvalidSubdomain", func(t *testing.T) {
		t.Parallel()
		client := setup(t, proxyTestSubdomain)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		uri := fmt.Sprintf("http://not-an-app-subdomain.%s/api/v2/users/me", proxyTestSubdomain)
		resp, err := client.Request(ctx, http.MethodGet, uri, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should have a HTML error response.
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), "Could not parse subdomain application URL")
	})
}

func TestWorkspaceAppsProxySubdomain(t *testing.T) {
	t.Parallel()
	client, firstUser, workspace, port := setupProxyTest(t)

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
		}.String() + "." + proxyTestSubdomain

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

	t.Run("NoAccessShould401", func(t *testing.T) {
		t.Parallel()

		userClient := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID, rbac.RoleMember())
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

	t.Run("ProxyPortMinimumError", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		port := uint16(codersdk.MinimumListeningPort - 1)
		resp, err := client.Request(ctx, http.MethodGet, proxyURL(t, port, "/", proxyTestAppQuery), nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should have an error response.
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var resBody codersdk.Response
		err = json.NewDecoder(resp.Body).Decode(&resBody)
		require.NoError(t, err)
		require.Contains(t, resBody.Message, "Coder reserves ports less than")
	})
}
