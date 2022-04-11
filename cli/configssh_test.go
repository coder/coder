package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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
	binPath := filepath.Join(t.TempDir(), "coder")
	_, err := exec.Command("go", "build", "-o", binPath, "github.com/coder/coder/cmd/coder").CombinedOutput()
	require.NoError(t, err)

	t.Run("Dial", func(t *testing.T) {
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
		cmd, root := clitest.New(t, "config-ssh", "--binary-file", binPath, "--ssh-config-file", tempFile.Name())
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
		time.Sleep(time.Hour)
		output, err := exec.Command("ssh", "-F", tempFile.Name(), "coder."+workspace.Name, "echo", "test").Output()
		t.Log(string(output))
		require.NoError(t, err)
		require.Equal(t, "test", strings.TrimSpace(string(output)))
	})
}
