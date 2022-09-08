package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
		URL           string
		Expected      coderd.ApplicationURL
		ExpectedError string
	}{
		{
			Name:          "Empty",
			URL:           "https://example.com",
			Expected:      coderd.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Workspace.Agent+App",
			URL:           "https://workspace.agent--app.coder.com",
			Expected:      coderd.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Workspace+App",
			URL:           "https://workspace--app.coder.com",
			Expected:      coderd.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		// Correct
		{
			Name: "User+Workspace+App",
			URL:  "https://app--workspace--user.coder.com",
			Expected: coderd.ApplicationURL{
				AppName:       "app",
				WorkspaceName: "workspace",
				Agent:         "",
				Username:      "user",
				Path:          "",
				Domain:        "coder.com",
			},
		},
		{
			Name: "User+Workspace+Port",
			URL:  "https://8080--workspace--user.coder.com",
			Expected: coderd.ApplicationURL{
				AppName:       "8080",
				WorkspaceName: "workspace",
				Agent:         "",
				Username:      "user",
				Path:          "",
				Domain:        "coder.com",
			},
		},
		{
			Name: "User+Workspace.Agent+App",
			URL:  "https://app--workspace--agent--user.coder.com",
			Expected: coderd.ApplicationURL{
				AppName:       "app",
				WorkspaceName: "workspace",
				Agent:         "agent",
				Username:      "user",
				Path:          "",
				Domain:        "coder.com",
			},
		},
		{
			Name: "User+Workspace.Agent+Port",
			URL:  "https://8080--workspace--agent--user.coder.com",
			Expected: coderd.ApplicationURL{
				AppName:       "8080",
				WorkspaceName: "workspace",
				Agent:         "agent",
				Username:      "user",
				Path:          "",
				Domain:        "coder.com",
			},
		},
		{
			Name: "HyphenatedNames",
			URL:  "https://app-name--workspace-thing--agent-thing--admin-user.coder.com",
			Expected: coderd.ApplicationURL{
				AppName:       "app-name",
				WorkspaceName: "workspace-thing",
				Agent:         "agent-thing",
				Username:      "admin-user",
				Path:          "",
				Domain:        "coder.com",
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest("GET", c.URL, nil)

			app, err := coderd.ParseSubdomainAppURL(r)
			if c.ExpectedError == "" {
				require.NoError(t, err)
				require.Equal(t, c.Expected, app, "expected app")
			} else {
				require.ErrorContains(t, err, c.ExpectedError, "expected error")
			}
		})
	}
}
