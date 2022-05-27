package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestWorkspaceAppsProxyPath(t *testing.T) {
	t.Parallel()
	t.Run("Proxies", func(t *testing.T) {
		t.Parallel()
		// #nosec
		ln, err := net.Listen("tcp", ":0")
		require.NoError(t, err)
		server := http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		}
		t.Cleanup(func() {
			_ = server.Close()
			_ = ln.Close()
		})
		go server.Serve(ln)
		tcpAddr, _ := ln.Addr().(*net.TCPAddr)

		client, coderAPI := coderdtest.NewWithAPI(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		daemonCloser := coderdtest.NewProvisionerDaemon(t, coderAPI)
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
									Url:  fmt.Sprintf("http://127.0.0.1:%d", tcpAddr.Port),
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
		daemonCloser.Close()

		agentClient := codersdk.New(client.URL)
		agentClient.SessionToken = authToken
		agentCloser := agent.New(agentClient.ListenWorkspaceAgent, &agent.Options{
			Logger: slogtest.Make(t, nil),
		})
		t.Cleanup(func() {
			_ = agentCloser.Close()
		})
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)

		resp, err := client.Request(context.Background(), http.MethodGet, "/me/"+workspace.Name+"/example", nil)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
