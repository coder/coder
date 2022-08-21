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
		IncludeProvisionerD: true,
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
		FetchMetadata: agentClient.WorkspaceAgentMetadata,
		WebRTCDialer:  agentClient.ListenWorkspaceAgent,
		Logger:        slogtest.Make(t, nil).Named("agent"),
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
