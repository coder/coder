package cli_test

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
)

func TestConfigSSH(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	coderdtest.NewProvisionerDaemon(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionDryRun: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: "example",
						}},
					}},
				},
			},
		}},
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: "example",
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
						}},
					}},
				},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, codersdk.Me, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	agentClient := codersdk.New(client.URL)
	agentClient.SessionToken = authToken
	agentCloser := agent.New(agentClient.ListenWorkspaceAgent, &peer.ConnOptions{
		Logger: slogtest.Make(t, nil),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	tempFile, err := os.CreateTemp(t.TempDir(), "")
	require.NoError(t, err)
	_ = tempFile.Close()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
	agentConn, err := client.DialWorkspaceAgent(context.Background(), resources[0].Agents[0].ID, nil, nil)
	require.NoError(t, err)
	defer agentConn.Close()

	// Using socat we can force SSH to use a UNIX socket
	// created in this test. That way we still validate
	// our configuration, but use the native SSH command
	// line to interface.
	socket := filepath.Join(t.TempDir(), "ssh")
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			ssh, err := agentConn.SSH()
			require.NoError(t, err)
			go io.Copy(conn, ssh)
			go io.Copy(ssh, conn)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
	})

	cmd, root := clitest.New(t, "config-ssh", "--ssh-option", "ProxyCommand socat - UNIX-CLIENT:"+socket, "--ssh-config-file", tempFile.Name())
	clitest.SetupConfig(t, client, root)
	doneChan := make(chan struct{})
	pty := ptytest.New(t)
	cmd.SetIn(pty.Input())
	cmd.SetOut(pty.Output())
	go func() {
		defer close(doneChan)
		err := cmd.Execute()
		require.NoError(t, err)
	}()
	<-doneChan

	t.Log(tempFile.Name())
	// #nosec
	data, err := exec.Command("ssh", "-F", tempFile.Name(), "coder."+workspace.Name, "echo", "test").Output()
	require.NoError(t, err)
	require.Equal(t, "test", strings.TrimSpace(string(data)))
}
