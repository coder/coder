package apptest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
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

// DeploymentOptions are the options for creating a *Deployment with a
// DeploymentFactory.
type DeploymentOptions struct {
	AppHost                              string
	DisablePathApps                      bool
	DisableSubdomainApps                 bool
	DangerousAllowPathAppSharing         bool
	DangerousAllowPathAppSiteOwnerAccess bool

	// The following fields are only used by setupProxyTestWithFactory.
	noWorkspace bool
	port        uint16
}

// Deployment is a license-agnostic deployment with all the fields that apps
// tests need.
type Deployment struct {
	Options *DeploymentOptions

	// Client should be logged in as the admin user.
	Client         *codersdk.Client
	FirstUser      codersdk.CreateFirstUserResponse
	PathAppBaseURL *url.URL
}

// DeploymentFactory generates a deployment with an API client, a path base URL,
// and a subdomain app host URL.
type DeploymentFactory func(t *testing.T, opts *DeploymentOptions) *Deployment

// App is similar to httpapi.ApplicationURL but with a Query field.
type App struct {
	Username      string
	WorkspaceName string
	// AgentName is optional, except for when proxying to a port. AgentName is
	// always ignored when making a path app URL.
	//
	// Set WorkspaceName to `workspace.agent` if you want to generate a path app
	// URL with an agent name.
	AgentName     string
	AppSlugOrPort string

	Query string
}

// AppDetails are the full test details returned from setupProxyTestWithFactory.
type AppDetails struct {
	*Deployment

	Me codersdk.User

	// The following fields are not set if setupProxyTest was called with
	// `withWorkspace` set to `false`.

	Workspace *codersdk.Workspace
	Agent     *codersdk.WorkspaceAgent
	AppPort   uint16

	FakeApp          App
	OwnerApp         App
	AuthenticatedApp App
	PublicApp        App
	PortApp          App
}

// PathAppURL returns the URL for the given path app.
func (d *AppDetails) PathAppURL(app App) *url.URL {
	appPath := fmt.Sprintf("/@%s/%s/apps/%s", app.Username, app.WorkspaceName, app.AppSlugOrPort)

	u := *d.PathAppBaseURL
	u.Path = path.Join(u.Path, appPath)
	u.Path += "/"
	u.RawQuery = app.Query
	return &u
}

// SubdomainAppURL returns the URL for the given subdomain app.
func (d *AppDetails) SubdomainAppURL(app App) *url.URL {
	if d.Options.DisableSubdomainApps || d.Options.AppHost == "" {
		panic("subdomain apps are disabled")
	}

	host := fmt.Sprintf("%s--%s--%s--%s", app.AppSlugOrPort, app.AgentName, app.WorkspaceName, app.Username)

	u := *d.PathAppBaseURL
	u.Host = strings.Replace(d.Options.AppHost, "*", host, 1)
	u.Path = "/"
	u.RawQuery = app.Query
	return &u
}

// setupProxyTestWithFactory does the following:
// 1. Create a deployment with the factory.
// 2. Start a test app server.
// 3. Create a template version, template and workspace with many apps.
// 4. Start a workspace agent.
// 5. Returns details about the deployment and its apps.
func setupProxyTestWithFactory(t *testing.T, factory DeploymentFactory, opts *DeploymentOptions) *AppDetails {
	if opts == nil {
		opts = &DeploymentOptions{}
	}
	if opts.AppHost == "" {
		opts.AppHost = proxyTestSubdomainRaw
	}
	if opts.DisableSubdomainApps {
		opts.AppHost = ""
	}

	deployment := factory(t, opts)

	// Configure the HTTP client to not follow redirects and to route all
	// requests regardless of hostname to the coderd test server.
	deployment.Client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	forceURLTransport(t, deployment.Client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	me, err := deployment.Client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	if opts.noWorkspace {
		return &AppDetails{
			Deployment: deployment,
			Me:         me,
		}
	}

	if opts.port == 0 {
		opts.port = appServer(t)
	}
	workspace, agnt := createWorkspaceWithApps(t, deployment.Client, deployment.FirstUser.OrganizationID, me, opts.AppHost, opts.port)

	return &AppDetails{
		Deployment: deployment,
		Me:         me,
		Workspace:  &workspace,
		Agent:      &agnt,
		AppPort:    opts.port,

		FakeApp: App{
			Username:      me.Username,
			WorkspaceName: workspace.Name,
			AgentName:     agnt.Name,
			AppSlugOrPort: proxyTestAppNameFake,
		},
		OwnerApp: App{
			Username:      me.Username,
			WorkspaceName: workspace.Name,
			AgentName:     agnt.Name,
			AppSlugOrPort: proxyTestAppNameOwner,
			Query:         proxyTestAppQuery,
		},
		AuthenticatedApp: App{
			Username:      me.Username,
			WorkspaceName: workspace.Name,
			AgentName:     agnt.Name,
			AppSlugOrPort: proxyTestAppNameAuthenticated,
			Query:         proxyTestAppQuery,
		},
		PublicApp: App{
			Username:      me.Username,
			WorkspaceName: workspace.Name,
			AgentName:     agnt.Name,
			AppSlugOrPort: proxyTestAppNamePublic,
			Query:         proxyTestAppQuery,
		},
		PortApp: App{
			Username:      me.Username,
			WorkspaceName: workspace.Name,
			AgentName:     agnt.Name,
			AppSlugOrPort: strconv.Itoa(int(opts.port)),
		},
	}
}

func appServer(t *testing.T) uint16 {
	// Start a listener on a random port greater than the minimum app port.
	var (
		ln      net.Listener
		tcpAddr *net.TCPAddr
	)
	for i := 0; i < 10; i++ {
		var err error
		// #nosec
		ln, err = net.Listen("tcp", ":0")
		require.NoError(t, err)

		var ok bool
		tcpAddr, ok = ln.Addr().(*net.TCPAddr)
		require.True(t, ok)
		if tcpAddr.Port < codersdk.WorkspaceAgentMinimumListeningPort {
			_ = ln.Close()
			time.Sleep(20 * time.Millisecond)
			continue
		}
	}

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
	go func() {
		_ = server.Serve(ln)
	}()

	return uint16(tcpAddr.Port)
}

func createWorkspaceWithApps(t *testing.T, client *codersdk.Client, orgID uuid.UUID, me codersdk.User, appHost string, port uint16, workspaceMutators ...func(*codersdk.CreateWorkspaceRequest)) (codersdk.Workspace, codersdk.WorkspaceAgent) {
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

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)
	if appHost != "" {
		manifest, err := agentClient.Manifest(context.Background())
		require.NoError(t, err)
		proxyURL := fmt.Sprintf(
			"http://{{port}}--%s--%s--%s%s",
			proxyTestAgentName,
			workspace.Name,
			me.Username,
			strings.ReplaceAll(appHost, "*", ""),
		)
		if client.URL.Port() != "" {
			proxyURL += fmt.Sprintf(":%s", client.URL.Port())
		}
		require.Equal(t, proxyURL, manifest.VSCodePortProxyURI)
	}
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: slogtest.Make(t, nil).Named("agent"),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})

	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	agents := make([]codersdk.WorkspaceAgent, 0, 1)
	for _, resource := range resources {
		agents = append(agents, resource.Agents...)
	}
	require.Len(t, agents, 1)

	return workspace, agents[0]
}

func doWithRetries(t require.TestingT, client *codersdk.Client, req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	require.Eventually(t, func() bool {
		// nolint // only requests which are not passed upstream have a body closed
		resp, err = client.HTTPClient.Do(req)
		if resp != nil && resp.StatusCode == http.StatusBadGateway {
			if resp.Body != nil {
				resp.Body.Close()
			}
			return false
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast)
	return resp, err
}

func requestWithRetries(ctx context.Context, t require.TestingT, client *codersdk.Client, method, urlOrPath string, body interface{}, opts ...codersdk.RequestOption) (*http.Response, error) {
	var resp *http.Response
	var err error
	require.Eventually(t, func() bool {
		// nolint // only requests which are not passed upstream have a body closed
		resp, err = client.Request(ctx, method, urlOrPath, body, opts...)
		if resp != nil && resp.StatusCode == http.StatusBadGateway {
			if resp.Body != nil {
				resp.Body.Close()
			}
			return false
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast)
	return resp, err
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
