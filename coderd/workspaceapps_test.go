package coderd_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

const (
	proxyTestAgentName            = "agent-name"
	proxyTestAppNameFake          = "test-app-fake"
	proxyTestAppNameOwner         = "test-app-owner"
	proxyTestAppNameAuthenticated = "test-app-authenticated"
	proxyTestAppNamePublic        = "test-app-public"
	proxyTestAppQuery             = "query=true"
	proxyTestAppBody              = "hello world from apps test"

	proxyTestSubdomainRaw = "*.test.coder.com"
	proxyTestSubdomain    = "test.coder.com"
)

func TestGetAppHost(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		accessURL   string
		appHostname string
		expected    string
	}{
		{
			name:        "OK",
			accessURL:   "https://test.coder.com",
			appHostname: "*.test.coder.com",
			expected:    "*.test.coder.com",
		},
		{
			name:        "None",
			accessURL:   "https://test.coder.com",
			appHostname: "",
			expected:    "",
		},
		{
			name:        "OKWithPort",
			accessURL:   "https://test.coder.com:8443",
			appHostname: "*.test.coder.com",
			expected:    "*.test.coder.com:8443",
		},
		{
			name:        "OKWithSuffix",
			accessURL:   "https://test.coder.com:8443",
			appHostname: "*--suffix.test.coder.com",
			expected:    "*--suffix.test.coder.com:8443",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			accessURL, err := url.Parse(c.accessURL)
			require.NoError(t, err)

			client := coderdtest.New(t, &coderdtest.Options{
				AccessURL:   accessURL,
				AppHostname: c.appHostname,
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// Should not leak to unauthenticated users.
			host, err := client.AppHost(ctx)
			require.Error(t, err)
			require.Equal(t, "", host.Host)

			_ = coderdtest.CreateFirstUser(t, client)
			host, err = client.AppHost(ctx)
			require.NoError(t, err)
			require.Equal(t, c.expected, host.Host)
		})
	}
}

type setupProxyTestOpts struct {
	AppHost                              string
	DisablePathApps                      bool
	DangerousAllowPathAppSharing         bool
	DangerousAllowPathAppSiteOwnerAccess bool

	NoWorkspace bool
}

// setupProxyTest creates a workspace with an agent and some apps. It returns a
// codersdk client, the first user, the workspace, and the port number the test
// listener is running on.
func setupProxyTest(t *testing.T, opts *setupProxyTestOpts) (*codersdk.Client, codersdk.CreateFirstUserResponse, *codersdk.Workspace, uint16) {
	if opts == nil {
		opts = &setupProxyTestOpts{}
	}
	if opts.AppHost == "" {
		opts.AppHost = proxyTestSubdomainRaw
	}

	// #nosec
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	server := http.Server{
		ReadHeaderTimeout: time.Minute,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := r.Cookie(codersdk.SessionTokenCookie)
			assert.ErrorIs(t, err, http.ErrNoCookie)
			w.Header().Set("X-Forwarded-For", r.Header.Get("X-Forwarded-For"))
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

	deploymentConfig := coderdtest.DeploymentConfig(t)
	deploymentConfig.DisablePathApps.Value = opts.DisablePathApps
	deploymentConfig.Dangerous.AllowPathAppSharing.Value = opts.DangerousAllowPathAppSharing
	deploymentConfig.Dangerous.AllowPathAppSiteOwnerAccess.Value = opts.DangerousAllowPathAppSiteOwnerAccess

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentConfig:            deploymentConfig,
		AppHostname:                 opts.AppHost,
		IncludeProvisionerDaemon:    true,
		AgentStatsRefreshInterval:   time.Millisecond * 100,
		MetricsCacheRefreshInterval: time.Millisecond * 100,
		RealIPConfig: &httpmw.RealIPConfig{
			TrustedOrigins: []*net.IPNet{{
				IP:   net.ParseIP("127.0.0.1"),
				Mask: net.CIDRMask(8, 32),
			}},
			TrustedHeaders: []string{
				"CF-Connecting-IP",
			},
		},
	})

	user := coderdtest.CreateFirstUser(t, client)

	var workspace *codersdk.Workspace
	if !opts.NoWorkspace {
		ws := createWorkspaceWithApps(t, client, user.OrganizationID, opts.AppHost, uint16(tcpAddr.Port))
		workspace = &ws
	}

	// Configure the HTTP client to not follow redirects and to route all
	// requests regardless of hostname to the coderd test server.
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	forceURLTransport(t, client)

	return client, user, workspace, uint16(tcpAddr.Port)
}

func createWorkspaceWithApps(t *testing.T, client *codersdk.Client, orgID uuid.UUID, appHost string, port uint16, workspaceMutators ...func(*codersdk.CreateWorkspaceRequest)) codersdk.Workspace {
	authToken := uuid.NewString()

	appURL := fmt.Sprintf("http://127.0.0.1:%d?%s", port, proxyTestAppQuery)
	version := coderdtest.CreateTemplateVersion(t, client, orgID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.ProvisionComplete,
		ProvisionApply: []*proto.Provision_Response{{
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
									Slug:         proxyTestAppNameFake,
									DisplayName:  proxyTestAppNameFake,
									SharingLevel: proto.AppSharingLevel_OWNER,
									// Hopefully this IP and port doesn't exist.
									Url: "http://127.1.0.1:65535",
								},
								{
									Slug:         proxyTestAppNameOwner,
									DisplayName:  proxyTestAppNameOwner,
									SharingLevel: proto.AppSharingLevel_OWNER,
									Url:          appURL,
								},
								{
									Slug:         proxyTestAppNameAuthenticated,
									DisplayName:  proxyTestAppNameAuthenticated,
									SharingLevel: proto.AppSharingLevel_AUTHENTICATED,
									Url:          appURL,
								},
								{
									Slug:         proxyTestAppNamePublic,
									DisplayName:  proxyTestAppNamePublic,
									SharingLevel: proto.AppSharingLevel_PUBLIC,
									Url:          appURL,
								},
							},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, orgID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, orgID, template.ID, workspaceMutators...)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	user, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	if appHost != "" {
		metadata, err := agentClient.Metadata(context.Background())
		require.NoError(t, err)
		proxyURL := fmt.Sprintf(
			"http://{{port}}--%s--%s--%s%s",
			proxyTestAgentName,
			workspace.Name,
			user.Username,
			strings.ReplaceAll(appHost, "*", ""),
		)
		if client.URL.Port() != "" {
			proxyURL += fmt.Sprintf(":%s", client.URL.Port())
		}
		require.Equal(t, proxyURL, metadata.VSCodePortProxyURI)
	}
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: slogtest.Make(t, nil).Named("agent"),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	return workspace
}

func TestWorkspaceAppsProxyPath(t *testing.T) {
	t.Parallel()
	client, firstUser, workspace, _ := setupProxyTest(t, nil)

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()

		deploymentConfig := coderdtest.DeploymentConfig(t)
		deploymentConfig.DisablePathApps.Value = true

		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentConfig:            deploymentConfig,
			IncludeProvisionerDaemon:    true,
			AgentStatsRefreshInterval:   time.Millisecond * 100,
			MetricsCacheRefreshInterval: time.Millisecond * 100,
		})
		user := coderdtest.CreateFirstUser(t, client)
		workspace := createWorkspaceWithApps(t, client, user.OrganizationID, "", 0)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, fmt.Sprintf("/@me/%s/apps/%s", workspace.Name, proxyTestAppNameOwner), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), "Path-based applications are disabled")
	})

	t.Run("LoginWithoutAuth", func(t *testing.T) {
		t.Parallel()
		client := codersdk.New(client.URL)
		client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, fmt.Sprintf("/@me/%s/apps/%s", workspace.Name, proxyTestAppNameOwner), nil)
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

		userClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID, rbac.RoleMember())
		userClient.HTTPClient.CheckRedirect = client.HTTPClient.CheckRedirect
		userClient.HTTPClient.Transport = client.HTTPClient.Transport

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, userClient, http.MethodGet, fmt.Sprintf("/@me/%s/apps/%s", workspace.Name, proxyTestAppNameOwner), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("RedirectsWithSlash", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, fmt.Sprintf("/@me/%s/apps/%s", workspace.Name, proxyTestAppNameOwner), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	})

	t.Run("RedirectsWithQuery", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, fmt.Sprintf("/@me/%s/apps/%s/", workspace.Name, proxyTestAppNameOwner), nil)
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

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, fmt.Sprintf("/@me/%s/apps/%s/?%s", workspace.Name, proxyTestAppNameOwner, proxyTestAppQuery), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, proxyTestAppBody, string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("ForwardsIP", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, fmt.Sprintf("/@me/%s/apps/%s/?%s", workspace.Name, proxyTestAppNameOwner, proxyTestAppQuery), nil, func(r *http.Request) {
			r.Header.Set("Cf-Connecting-IP", "1.1.1.1")
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, proxyTestAppBody, string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "1.1.1.1,127.0.0.1", resp.Header.Get("X-Forwarded-For"))
	})

	t.Run("ProxyError", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodGet, fmt.Sprintf("/@me/%s/apps/%s/", workspace.Name, proxyTestAppNameFake), nil)
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

		client, firstUser, workspace, _ := setupProxyTest(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Get the current user and API key.
		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		currentAPIKey, err := client.APIKey(ctx, firstUser.UserID.String(), strings.Split(client.SessionToken(), "-")[0])
		require.NoError(t, err)

		// Try to load the application without authentication.
		subdomain := fmt.Sprintf("%s--%s--%s--%s", proxyTestAppNameOwner, proxyTestAgentName, workspace.Name, user.Username)
		u, err := url.Parse(fmt.Sprintf("http://%s.%s/test", subdomain, proxyTestSubdomain))
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		require.NoError(t, err)

		var resp *http.Response
		resp, err = doWithRetries(t, client, req)
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
		resp, err = requestWithRetries(ctx, t, client, http.MethodGet, "/api/v2/applications/auth-redirect", nil, codersdk.WithQueryParam(
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
		resp, err = doWithRetries(t, client, req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		cookies := resp.Cookies()
		require.Len(t, cookies, 1)
		apiKey := cookies[0].Value

		// Fetch the API key.
		apiKeyInfo, err := client.APIKey(ctx, firstUser.UserID.String(), strings.Split(apiKey, "-")[0])
		require.NoError(t, err)
		require.Equal(t, user.ID, apiKeyInfo.UserID)
		require.Equal(t, codersdk.LoginTypePassword, apiKeyInfo.LoginType)
		require.WithinDuration(t, currentAPIKey.ExpiresAt, apiKeyInfo.ExpiresAt, 5*time.Second)
		require.EqualValues(t, currentAPIKey.LifetimeSeconds, apiKeyInfo.LifetimeSeconds)

		// Verify the API key permissions
		appClient := codersdk.New(client.URL)
		appClient.SetSessionToken(apiKey)
		appClient.HTTPClient.CheckRedirect = client.HTTPClient.CheckRedirect
		appClient.HTTPClient.Transport = client.HTTPClient.Transport

		var (
			canCreateApplicationConnect = "can-create-application_connect"
			canReadUserMe               = "can-read-user-me"
		)
		authRes, err := appClient.AuthCheck(ctx, codersdk.AuthorizationRequest{
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
		req.Header.Set(codersdk.SessionTokenHeader, apiKey)
		resp, err = doWithRetries(t, client, req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("VerifyRedirectURI", func(t *testing.T) {
		t.Parallel()

		client, _, _, _ := setupProxyTest(t, nil)

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

				resp, err := requestWithRetries(ctx, t, client, http.MethodGet, "/api/v2/applications/auth-redirect", nil,
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
	t.Cleanup(func() {
		transport.CloseIdleConnections()
	})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	uri := fmt.Sprintf("http://app--agent--workspace--username.%s/api/v2/users/me", proxyTestSubdomain)
	resp, err := requestWithRetries(ctx, t, client, http.MethodGet, uri, nil)
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
		t.Cleanup(func() {
			transport.CloseIdleConnections()
		})

		return client
	}

	t.Run("InvalidSubdomain", func(t *testing.T) {
		t.Parallel()
		client := setup(t, proxyTestSubdomainRaw)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		uri := fmt.Sprintf("http://not-an-app-subdomain.%s/api/v2/users/me", proxyTestSubdomain)
		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, uri, nil)
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
	client, firstUser, _, port := setupProxyTest(t, nil)

	// proxyURL generates a URL for the proxy subdomain. The default path is a
	// slash.
	proxyURL := func(t *testing.T, client *codersdk.Client, appNameOrPort interface{}, pathAndQuery ...string) string {
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		me, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err, "get current user details")

		res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Owner: codersdk.Me,
		})
		require.NoError(t, err, "get workspaces")
		require.Len(t, res.Workspaces, 1, "expected 1 workspace")

		appHost, err := client.AppHost(ctx)
		require.NoError(t, err, "get app host")

		subdomain := httpapi.ApplicationURL{
			AppSlug:       appName,
			Port:          port,
			AgentName:     proxyTestAgentName,
			WorkspaceName: res.Workspaces[0].Name,
			Username:      me.Username,
		}.String()

		hostname := strings.Replace(appHost.Host, "*", subdomain, 1)

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

		userClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID, rbac.RoleMember())
		userClient.HTTPClient.CheckRedirect = client.HTTPClient.CheckRedirect
		userClient.HTTPClient.Transport = client.HTTPClient.Transport

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, userClient, http.MethodGet, proxyURL(t, client, proxyTestAppNameOwner), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("RedirectsWithSlash", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		slashlessURL := proxyURL(t, client, proxyTestAppNameOwner, "")
		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, slashlessURL, nil)
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

		querylessURL := proxyURL(t, client, proxyTestAppNameOwner, "/", "")
		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, querylessURL, nil)
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

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, proxyURL(t, client, proxyTestAppNameOwner, "/", proxyTestAppQuery), nil)
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

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, proxyURL(t, client, port, "/", proxyTestAppQuery), nil)
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

		resp, err := client.Request(ctx, http.MethodGet, proxyURL(t, client, proxyTestAppNameFake, "/", ""), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	})

	t.Run("ProxyPortMinimumError", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		port := uint16(codersdk.WorkspaceAgentMinimumListeningPort - 1)
		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, proxyURL(t, client, port, "/", proxyTestAppQuery), nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should have an error response.
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var resBody codersdk.Response
		err = json.NewDecoder(resp.Body).Decode(&resBody)
		require.NoError(t, err)
		require.Contains(t, resBody.Message, "Coder reserves ports less than")
	})

	t.Run("SuffixWildcardOK", func(t *testing.T) {
		t.Parallel()

		client, _, _, _ := setupProxyTest(t, &setupProxyTestOpts{
			AppHost: "*-suffix.test.coder.com",
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		u := proxyURL(t, client, proxyTestAppNameOwner, "/", proxyTestAppQuery)
		t.Logf("url: %s", u)

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, u, nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, proxyTestAppBody, string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("SuffixWildcardNotMatch", func(t *testing.T) {
		t.Parallel()

		client, _, _, _ := setupProxyTest(t, &setupProxyTestOpts{
			AppHost: "*-suffix.test.coder.com",
		})

		t.Run("NoSuffix", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			u := proxyURL(t, client, proxyTestAppNameOwner, "/", proxyTestAppQuery)
			// Replace the -suffix with nothing.
			u = strings.Replace(u, "-suffix", "", 1)

			resp, err := requestWithRetries(ctx, t, client, http.MethodGet, u, nil)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// It's probably rendering the dashboard, so only ensure that the body
			// doesn't match.
			require.NotContains(t, string(body), proxyTestAppBody)
		})

		t.Run("DifferentSuffix", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			u := proxyURL(t, client, proxyTestAppNameOwner, "/", proxyTestAppQuery)
			// Replace the -suffix with something else.
			u = strings.Replace(u, "-suffix", "-not-suffix", 1)

			resp, err := requestWithRetries(ctx, t, client, http.MethodGet, u, nil)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// It's probably rendering the dashboard, so only ensure that the body
			// doesn't match.
			require.NotContains(t, string(body), proxyTestAppBody)
		})
	})
}

func TestAppSubdomainLogout(t *testing.T) {
	t.Parallel()

	keyID, keySecret, err := coderd.GenerateAPIKeyIDSecret()
	require.NoError(t, err)
	fakeAPIKey := fmt.Sprintf("%s-%s", keyID, keySecret)

	cases := []struct {
		name string
		// The cookie to send with the request. The regular API key header
		// is also sent to bypass any auth checks on this value, and to
		// ensure that the logout code is safe when multiple keys are
		// passed.
		// Empty value means no cookie is sent, "-" means send a valid
		// API key, and "bad-secret" means send a valid key ID with a bad
		// secret.
		cookie string
		// You can use "access_url" to use the site access URL as the
		// redirect URI, or "app_host" to use a valid app host.
		redirectURI string

		// If expectedStatus is not an error status, we expect the cookie to
		// be deleted if it was set.
		expectedStatus       int
		expectedBodyContains string
		// If empty, the expected location is the redirectURI if the
		// expected status code is http.StatusTemporaryRedirect (using the
		// access URL if not set).
		// You can use "access_url" to force the access URL.
		expectedLocation string
	}{
		{
			name:           "OKAccessURL",
			cookie:         "-",
			redirectURI:    "access_url",
			expectedStatus: http.StatusTemporaryRedirect,
		},
		{
			name:           "OKAppHost",
			cookie:         "-",
			redirectURI:    "app_host",
			expectedStatus: http.StatusTemporaryRedirect,
		},
		{
			name:        "OKNoAPIKey",
			cookie:      "",
			redirectURI: "access_url",
			// Even if the devurl cookie is missing, we still redirect without
			// any complaints.
			expectedStatus: http.StatusTemporaryRedirect,
		},
		{
			name:        "OKBadAPIKey",
			cookie:      "test-api-key",
			redirectURI: "access_url",
			// Even if the devurl cookie is bad, we still delete the cookie and
			// redirect without any complaints.
			expectedStatus: http.StatusTemporaryRedirect,
		},
		{
			name:           "OKUnknownAPIKey",
			cookie:         fakeAPIKey,
			redirectURI:    "access_url",
			expectedStatus: http.StatusTemporaryRedirect,
		},
		{
			name:                 "BadAPIKeySecret",
			cookie:               "bad-secret",
			redirectURI:          "access_url",
			expectedStatus:       http.StatusUnauthorized,
			expectedBodyContains: "API key secret is invalid",
		},
		{
			name:                 "InvalidRedirectURI",
			cookie:               "-",
			redirectURI:          string([]byte{0x00}),
			expectedStatus:       http.StatusBadRequest,
			expectedBodyContains: "Could not parse redirect URI",
		},
		{
			name:        "DisallowedRedirectURI",
			cookie:      "-",
			redirectURI: "https://github.com/coder/coder",
			// We don't allow redirecting to a different host, but we don't
			// show an error page and just redirect to the access URL to avoid
			// breaking the logout flow if the user is accessing from the wrong
			// host.
			expectedStatus:   http.StatusTemporaryRedirect,
			expectedLocation: "access_url",
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			client, _, _, _ := setupProxyTest(t, nil)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			// The token should work.
			_, err := client.User(ctx, codersdk.Me)
			require.NoError(t, err)

			appHost, err := client.AppHost(ctx)
			require.NoError(t, err, "get app host")

			if c.cookie == "-" {
				c.cookie = client.SessionToken()
			} else if c.cookie == "bad-secret" {
				keyID, _, err := httpmw.SplitAPIToken(client.SessionToken())
				require.NoError(t, err)
				c.cookie = fmt.Sprintf("%s-%s", keyID, keySecret)
			}
			if c.redirectURI == "access_url" {
				c.redirectURI = client.URL.String()
			} else if c.redirectURI == "app_host" {
				c.redirectURI = "http://" + strings.Replace(appHost.Host, "*", "something--something--something--something", 1) + "/"
			}
			if c.expectedLocation == "" && c.expectedStatus == http.StatusTemporaryRedirect {
				c.expectedLocation = c.redirectURI
			}
			if c.expectedLocation == "access_url" {
				c.expectedLocation = client.URL.String()
			}

			logoutURL := &url.URL{
				Scheme: "http",
				Host:   strings.Replace(appHost.Host, "*", "coder-logout", 1),
				Path:   "/",
			}
			if c.redirectURI != "" {
				q := logoutURL.Query()
				q.Set("redirect_uri", c.redirectURI)
				logoutURL.RawQuery = q.Encode()
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, logoutURL.String(), nil)
			require.NoError(t, err, "create logout request")
			// The header is prioritized over the devurl cookie if both are
			// set, so this ensures we can trigger the logout code path with
			// bad cookies during tests.
			req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
			if c.cookie != "" {
				req.AddCookie(&http.Cookie{
					Name:  httpmw.DevURLSessionTokenCookie,
					Value: c.cookie,
				})
			}

			client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
			resp, err := client.HTTPClient.Do(req)
			require.NoError(t, err, "do logout request")
			defer resp.Body.Close()

			require.Equal(t, c.expectedStatus, resp.StatusCode, "logout response status code")
			if c.expectedStatus < 400 && c.cookie != "" {
				cookies := resp.Cookies()
				require.Len(t, cookies, 1, "logout response cookies")
				cookie := cookies[0]
				require.Equal(t, httpmw.DevURLSessionTokenCookie, cookie.Name)
				require.Equal(t, "", cookie.Value)
				require.True(t, cookie.Expires.Before(time.Now()), "cookie should be expired")

				// The token shouldn't work anymore if it was the original valid
				// session token.
				if c.cookie == client.SessionToken() {
					_, err = client.User(ctx, codersdk.Me)
					require.Error(t, err)
				}
			}
			if c.expectedBodyContains != "" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Contains(t, string(body), c.expectedBodyContains, "logout response body")
			}
			if c.expectedLocation != "" {
				location := resp.Header.Get("Location")
				require.Equal(t, c.expectedLocation, location, "logout response location")
			}
		})
	}
}

func TestAppSharing(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T, allowPathAppSharing, allowSiteOwnerAccess bool) (workspace codersdk.Workspace, agnt codersdk.WorkspaceAgent, user codersdk.User, ownerClient *codersdk.Client, client *codersdk.Client, clientInOtherOrg *codersdk.Client, clientWithNoAuth *codersdk.Client) {
		//nolint:gosec
		const password = "SomeSecurePassword!"

		var port uint16
		ownerClient, _, _, port = setupProxyTest(t, &setupProxyTestOpts{
			NoWorkspace:                          true,
			DangerousAllowPathAppSharing:         allowPathAppSharing,
			DangerousAllowPathAppSiteOwnerAccess: allowSiteOwnerAccess,
		})
		forceURLTransport(t, ownerClient)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		t.Cleanup(cancel)

		ownerUser, err := ownerClient.User(ctx, codersdk.Me)
		require.NoError(t, err)

		// Create a template-admin user in the same org. We don't use an owner
		// since they have access to everything.
		user, err = ownerClient.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "user@coder.com",
			Username:       "user",
			Password:       password,
			OrganizationID: ownerUser.OrganizationIDs[0],
		})
		require.NoError(t, err)

		_, err = ownerClient.UpdateUserRoles(ctx, user.ID.String(), codersdk.UpdateRoles{
			Roles: []string{"template-admin", "member"},
		})
		require.NoError(t, err)

		client = codersdk.New(ownerClient.URL)
		loginRes, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    user.Email,
			Password: password,
		})
		require.NoError(t, err)
		client.SetSessionToken(loginRes.SessionToken)
		client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		forceURLTransport(t, client)

		// Create workspace.
		workspace = createWorkspaceWithApps(t, client, user.OrganizationIDs[0], proxyTestSubdomainRaw, port)

		// Verify that the apps have the correct sharing levels set.
		workspaceBuild, err := client.WorkspaceBuild(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		agnt = workspaceBuild.Resources[0].Agents[0]
		found := map[string]codersdk.WorkspaceAppSharingLevel{}
		expected := map[string]codersdk.WorkspaceAppSharingLevel{
			proxyTestAppNameFake:          codersdk.WorkspaceAppSharingLevelOwner,
			proxyTestAppNameOwner:         codersdk.WorkspaceAppSharingLevelOwner,
			proxyTestAppNameAuthenticated: codersdk.WorkspaceAppSharingLevelAuthenticated,
			proxyTestAppNamePublic:        codersdk.WorkspaceAppSharingLevelPublic,
		}
		for _, app := range agnt.Apps {
			found[app.DisplayName] = app.SharingLevel
		}
		require.Equal(t, expected, found, "apps have incorrect sharing levels")

		// Create a user in a different org.
		otherOrg, err := ownerClient.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name: "a-different-org",
		})
		require.NoError(t, err)
		userInOtherOrg, err := ownerClient.CreateUser(ctx, codersdk.CreateUserRequest{
			Email:          "no-template-access@coder.com",
			Username:       "no-template-access",
			Password:       password,
			OrganizationID: otherOrg.ID,
		})
		require.NoError(t, err)

		clientInOtherOrg = codersdk.New(client.URL)
		loginRes, err = clientInOtherOrg.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    userInOtherOrg.Email,
			Password: password,
		})
		require.NoError(t, err)
		clientInOtherOrg.SetSessionToken(loginRes.SessionToken)
		clientInOtherOrg.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		forceURLTransport(t, clientInOtherOrg)

		// Create an unauthenticated codersdk client.
		clientWithNoAuth = codersdk.New(client.URL)
		clientWithNoAuth.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		forceURLTransport(t, clientWithNoAuth)

		return workspace, agnt, user, ownerClient, client, clientInOtherOrg, clientWithNoAuth
	}

	verifyAccess := func(t *testing.T, isPathApp bool, username, workspaceName, agentName, appName string, client *codersdk.Client, shouldHaveAccess, shouldRedirectToLogin bool) {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// If the client has a session token, we also want to check that a
		// scoped key works.
		clients := []*codersdk.Client{client}
		if client.SessionToken() != "" {
			token, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
				Scope: codersdk.APIKeyScopeApplicationConnect,
			})
			require.NoError(t, err)

			scopedClient := codersdk.New(client.URL)
			scopedClient.SetSessionToken(token.Key)
			scopedClient.HTTPClient.CheckRedirect = client.HTTPClient.CheckRedirect
			scopedClient.HTTPClient.Transport = client.HTTPClient.Transport

			clients = append(clients, scopedClient)
		}

		for i, client := range clients {
			msg := fmt.Sprintf("client %d", i)

			u := fmt.Sprintf("/@%s/%s.%s/apps/%s/?%s", username, workspaceName, agentName, appName, proxyTestAppQuery)
			if !isPathApp {
				subdomain := httpapi.ApplicationURL{
					AppSlug:       appName,
					AgentName:     agentName,
					WorkspaceName: workspaceName,
					Username:      username,
				}.String()

				hostname := strings.Replace(proxyTestSubdomainRaw, "*", subdomain, 1)
				u = fmt.Sprintf("http://%s/?%s", hostname, proxyTestAppQuery)
			}

			res, err := requestWithRetries(ctx, t, client, http.MethodGet, u, nil)
			require.NoError(t, err, msg)

			dump, err := httputil.DumpResponse(res, true)
			_ = res.Body.Close()
			require.NoError(t, err, msg)
			// t.Logf("response dump: %s", dump)

			if !shouldHaveAccess {
				if shouldRedirectToLogin {
					assert.Equal(t, http.StatusTemporaryRedirect, res.StatusCode, "should not have access, expected temporary redirect. "+msg)
					location, err := res.Location()
					require.NoError(t, err, msg)

					expectedPath := "/login"
					if !isPathApp {
						expectedPath = "/api/v2/applications/auth-redirect"
					}
					assert.Equal(t, expectedPath, location.Path, "should not have access, expected redirect to applicable login endpoint. "+msg)
				} else {
					// If the user doesn't have access we return 404 to avoid
					// leaking information about the existence of the app.
					assert.Equal(t, http.StatusNotFound, res.StatusCode, "should not have access, expected not found. "+msg)
				}
			}

			if shouldHaveAccess {
				assert.Equal(t, http.StatusOK, res.StatusCode, "should have access, expected ok. "+msg)
				assert.Contains(t, string(dump), "hello world", "should have access, expected hello world. "+msg)
			}
		}
	}

	testLevels := func(t *testing.T, isPathApp, pathAppSharingEnabled, siteOwnerPathAppAccessEnabled bool) {
		workspace, agnt, user, ownerClient, client, clientInOtherOrg, clientWithNoAuth := setup(t, pathAppSharingEnabled, siteOwnerPathAppAccessEnabled)

		allowedUnlessSharingDisabled := !isPathApp || pathAppSharingEnabled
		siteOwnerCanAccess := !isPathApp || siteOwnerPathAppAccessEnabled
		siteOwnerCanAccessShared := siteOwnerCanAccess || pathAppSharingEnabled

		deploymentConfig, err := ownerClient.DeploymentConfig(context.Background())
		require.NoError(t, err)

		assert.Equal(t, pathAppSharingEnabled, deploymentConfig.Dangerous.AllowPathAppSharing.Value)
		assert.Equal(t, siteOwnerPathAppAccessEnabled, deploymentConfig.Dangerous.AllowPathAppSiteOwnerAccess.Value)

		t.Run("LevelOwner", func(t *testing.T) {
			t.Parallel()

			// Site owner should be able to access all workspaces if
			// enabled.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNameOwner, ownerClient, siteOwnerCanAccess, false)

			// Owner should be able to access their own workspace.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNameOwner, client, true, false)

			// Authenticated users should not have access to a workspace that
			// they do not own.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNameOwner, clientInOtherOrg, false, false)

			// Unauthenticated user should not have any access.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNameOwner, clientWithNoAuth, false, true)
		})

		t.Run("LevelAuthenticated", func(t *testing.T) {
			t.Parallel()

			// Site owner should be able to access all workspaces if
			// enabled.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNameAuthenticated, ownerClient, siteOwnerCanAccessShared, false)

			// Owner should be able to access their own workspace.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNameAuthenticated, client, true, false)

			// Authenticated users should be able to access the workspace.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNameAuthenticated, clientInOtherOrg, allowedUnlessSharingDisabled, false)

			// Unauthenticated user should not have any access.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNameAuthenticated, clientWithNoAuth, false, true)
		})

		t.Run("LevelPublic", func(t *testing.T) {
			t.Parallel()

			// Site owner should be able to access all workspaces if
			// enabled.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNamePublic, ownerClient, siteOwnerCanAccessShared, false)

			// Owner should be able to access their own workspace.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNamePublic, client, true, false)

			// Authenticated users should be able to access the workspace.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNamePublic, clientInOtherOrg, allowedUnlessSharingDisabled, false)

			// Unauthenticated user should be able to access the workspace.
			verifyAccess(t, isPathApp, user.Username, workspace.Name, agnt.Name, proxyTestAppNamePublic, clientWithNoAuth, allowedUnlessSharingDisabled, !allowedUnlessSharingDisabled)
		})
	}

	t.Run("Path", func(t *testing.T) {
		t.Parallel()

		t.Run("Default", func(t *testing.T) {
			t.Parallel()
			testLevels(t, true, false, false)
		})

		t.Run("AppSharingEnabled", func(t *testing.T) {
			t.Parallel()
			testLevels(t, true, true, false)
		})

		t.Run("SiteOwnerAccessEnabled", func(t *testing.T) {
			t.Parallel()
			testLevels(t, true, false, true)
		})

		t.Run("BothEnabled", func(t *testing.T) {
			t.Parallel()
			testLevels(t, true, false, true)
		})
	})

	t.Run("Subdomain", func(t *testing.T) {
		t.Parallel()
		testLevels(t, false, false, false)
	})
}

func TestWorkspaceAppsNonCanonicalHeaders(t *testing.T) {
	t.Parallel()

	setupNonCanonicalHeadersTest := func(t *testing.T, customAppHost ...string) (*codersdk.Client, codersdk.CreateFirstUserResponse, codersdk.Workspace, uint16) {
		// Start a TCP server that manually parses the request. Golang's HTTP
		// server canonicalizes all HTTP request headers it receives, so we
		// can't use it to test that we forward non-canonical headers.
		// #nosec
		ln, err := net.Listen("tcp", ":0")
		require.NoError(t, err)
		go func() {
			for {
				c, err := ln.Accept()
				if xerrors.Is(err, net.ErrClosed) {
					return
				}
				require.NoError(t, err)

				go func() {
					s := bufio.NewScanner(c)

					// Read request line.
					assert.True(t, s.Scan())
					reqLine := s.Text()
					assert.True(t, strings.HasPrefix(reqLine, fmt.Sprintf("GET /?%s HTTP/1.1", proxyTestAppQuery)))

					// Read headers and discard them. We collect the
					// Sec-WebSocket-Key header (with a capital S) to respond
					// with.
					secWebSocketKey := "(none found)"
					for s.Scan() {
						if s.Text() == "" {
							break
						}

						line := strings.TrimSpace(s.Text())
						if strings.HasPrefix(line, "Sec-WebSocket-Key: ") {
							secWebSocketKey = strings.TrimPrefix(line, "Sec-WebSocket-Key: ")
						}
					}

					// Write response containing text/plain with the
					// Sec-WebSocket-Key header.
					res := fmt.Sprintf("HTTP/1.1 204 No Content\r\nSec-WebSocket-Key: %s\r\nConnection: close\r\n\r\n", secWebSocketKey)
					_, err = c.Write([]byte(res))
					assert.NoError(t, err)
					err = c.Close()
					assert.NoError(t, err)
				}()
			}
		}()
		t.Cleanup(func() {
			_ = ln.Close()
		})
		tcpAddr, ok := ln.Addr().(*net.TCPAddr)
		require.True(t, ok)

		appHost := proxyTestSubdomainRaw
		if len(customAppHost) > 0 {
			appHost = customAppHost[0]
		}

		client := coderdtest.New(t, &coderdtest.Options{
			AppHostname:                 appHost,
			IncludeProvisionerDaemon:    true,
			AgentStatsRefreshInterval:   time.Millisecond * 100,
			MetricsCacheRefreshInterval: time.Millisecond * 100,
			RealIPConfig: &httpmw.RealIPConfig{
				TrustedOrigins: []*net.IPNet{{
					IP:   net.ParseIP("127.0.0.1"),
					Mask: net.CIDRMask(8, 32),
				}},
				TrustedHeaders: []string{
					"CF-Connecting-IP",
				},
			},
		})

		user := coderdtest.CreateFirstUser(t, client)

		workspace := createWorkspaceWithApps(t, client, user.OrganizationID, appHost, uint16(tcpAddr.Port))

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
		t.Cleanup(func() {
			transport.CloseIdleConnections()
		})

		return client, user, workspace, uint16(tcpAddr.Port)
	}

	t.Run("ProxyPath", func(t *testing.T) {
		t.Parallel()

		client, _, workspace, _ := setupNonCanonicalHeadersTest(t)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		u, err := client.URL.Parse(fmt.Sprintf("/@me/%s/apps/%s/?%s", workspace.Name, proxyTestAppNameOwner, proxyTestAppQuery))
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		require.NoError(t, err)

		// Use a non-canonical header name. The S in Sec-WebSocket-Key should be
		// capitalized according to the websocket spec, but Golang will
		// lowercase it to match the HTTP/1 spec.
		//
		// Setting the header on the map directly will force the header to not
		// be canonicalized on the client, but it will be canonicalized on the
		// server.
		secWebSocketKey := "test-dean-was-here"
		req.Header["Sec-WebSocket-Key"] = []string{secWebSocketKey}

		req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		resp, err := doWithRetries(t, client, req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// The response should be a 204 No Content with the Sec-WebSocket-Key
		// header set to the value we sent.
		res, err := httputil.DumpResponse(resp, true)
		require.NoError(t, err)
		t.Log(string(res))
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		require.Equal(t, secWebSocketKey, resp.Header.Get("Sec-WebSocket-Key"))
	})

	t.Run("Subdomain", func(t *testing.T) {
		t.Parallel()

		appHost := proxyTestSubdomainRaw
		client, _, workspace, _ := setupNonCanonicalHeadersTest(t, appHost)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		user, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)

		u := fmt.Sprintf(
			"http://%s--%s--%s--%s%s?%s",
			proxyTestAppNameOwner,
			proxyTestAgentName,
			workspace.Name,
			user.Username,
			strings.ReplaceAll(appHost, "*", ""),
			proxyTestAppQuery,
		)

		// Re-enable the default redirect behavior.
		client.HTTPClient.CheckRedirect = nil

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		require.NoError(t, err)

		// Use a non-canonical header name. The S in Sec-WebSocket-Key should be
		// capitalized according to the websocket spec, but Golang will
		// lowercase it to match the HTTP/1 spec.
		//
		// Setting the header on the map directly will force the header to not
		// be canonicalized on the client, but it will be canonicalized on the
		// server.
		secWebSocketKey := "test-dean-was-here"
		req.Header["Sec-WebSocket-Key"] = []string{secWebSocketKey}

		req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		resp, err := doWithRetries(t, client, req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// The response should be a 204 No Content with the Sec-WebSocket-Key
		// header set to the value we sent.
		res, err := httputil.DumpResponse(resp, true)
		require.NoError(t, err)
		t.Log(string(res))
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		require.Equal(t, secWebSocketKey, resp.Header.Get("Sec-WebSocket-Key"))
	})
}

// forceURLTransport forces the client to route all requests to the client's
// configured URL host regardless of hostname.
func forceURLTransport(t *testing.T, client *codersdk.Client) {
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	transport := defaultTransport.Clone()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, client.URL.Host)
	}
	client.HTTPClient.Transport = transport
	t.Cleanup(func() {
		transport.CloseIdleConnections()
	})
}
