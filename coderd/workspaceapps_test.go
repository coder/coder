package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func TestWorkspaceAppsProxyPath(t *testing.T) {
	t.Parallel()
	// #nosec
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	server := http.Server{
		ReadHeaderTimeout: time.Minute,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := r.Cookie(codersdk.SessionTokenKey)
			assert.ErrorIs(t, err, http.ErrNoCookie)
			w.WriteHeader(http.StatusOK)
		}),
	}
	t.Cleanup(func() {
		_ = server.Close()
		_ = ln.Close()
	})
	go server.Serve(ln)
	tcpAddr, _ := ln.Addr().(*net.TCPAddr)

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
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
							Id: uuid.NewString(),
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
							Apps: []*proto.App{{
								Name: "example",
								Url:  fmt.Sprintf("http://127.0.0.1:%d?query=true", tcpAddr.Port),
							}, {
								Name: "fake",
								Url:  "http://127.0.0.2",
							}},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	agentClient := codersdk.New(client.URL)
	agentClient.SessionToken = authToken
	agentCloser := agent.New(agent.Options{
		FetchMetadata:     agentClient.WorkspaceAgentMetadata,
		CoordinatorDialer: agentClient.ListenWorkspaceAgentTailnet,
		WebRTCDialer:      agentClient.ListenWorkspaceAgent,
		Logger:            slogtest.Make(t, nil).Named("agent"),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	t.Run("RedirectsWithoutAuth", func(t *testing.T) {
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
		location, err := resp.Location()
		require.NoError(t, err)
		require.Equal(t, "/login", location.Path)
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
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
		require.Equal(t, "query=true", loc.RawQuery)
	})

	t.Run("Proxies", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodGet, "/@me/"+workspace.Name+"/apps/example/?query=true", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("ProxyError", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.Request(ctx, http.MethodGet, "/@me/"+workspace.Name+"/apps/fake/", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestParseSubdomainAppURL(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name          string
		Host          string
		Expected      coderd.ApplicationURL
		ExpectedError string
	}{
		{
			Name:          "Invalid_Empty",
			Host:          "example.com",
			Expected:      coderd.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_Workspace.Agent--App",
			Host:          "workspace.agent--app.coder.com",
			Expected:      coderd.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_Workspace--App",
			Host:          "workspace--app.coder.com",
			Expected:      coderd.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_App--Workspace--User",
			Host:          "app--workspace--user.coder.com",
			Expected:      coderd.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_TooManyComponents",
			Host:          "1--2--3--4--5.coder.com",
			Expected:      coderd.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		// Correct
		{
			Name: "AppName--Agent--Workspace--User",
			Host: "app--agent--workspace--user.coder.com",
			Expected: coderd.ApplicationURL{
				AppName:       "app",
				Port:          0,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
				BaseHostname:  "coder.com",
			},
		},
		{
			Name: "Port--Agent--Workspace--User",
			Host: "8080--agent--workspace--user.coder.com",
			Expected: coderd.ApplicationURL{
				AppName:       "",
				Port:          8080,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
				BaseHostname:  "coder.com",
			},
		},
		{
			Name: "DeepSubdomain",
			Host: "app--agent--workspace--user.dev.dean-was-here.coder.com",
			Expected: coderd.ApplicationURL{
				AppName:       "app",
				Port:          0,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
				BaseHostname:  "dev.dean-was-here.coder.com",
			},
		},
		{
			Name: "HyphenatedNames",
			Host: "app-name--agent-name--workspace-name--user-name.coder.com",
			Expected: coderd.ApplicationURL{
				AppName:       "app-name",
				Port:          0,
				AgentName:     "agent-name",
				WorkspaceName: "workspace-name",
				Username:      "user-name",
				BaseHostname:  "coder.com",
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			app, err := coderd.ParseSubdomainAppURL(c.Host)
			if c.ExpectedError == "" {
				require.NoError(t, err)
				require.Equal(t, c.Expected, app, "expected app")
			} else {
				require.ErrorContains(t, err, c.ExpectedError, "expected error")
			}
		})
	}
}
