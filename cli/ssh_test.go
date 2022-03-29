package cli_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
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

func TestSSH(t *testing.T) {
	t.Parallel()
	t.Run("ImmediateExit", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		agentToken := uuid.NewString()
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:           echo.ParseComplete,
			ProvisionDryRun: echo.ProvisionComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "dev",
							Type: "google_compute_instance",
							Agent: &proto.Agent{
								Id: uuid.NewString(),
								Auth: &proto.Agent_Token{
									Token: agentToken,
								},
							},
						}},
					},
				},
			}},
		})
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "", project.ID)
		go func() {
			// Run this async so the SSH command has to wait for
			// the build and agent to connect!
			coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
			agentClient := codersdk.New(client.URL)
			agentClient.SessionToken = agentToken
			agentCloser := agent.New(agentClient.ListenWorkspaceAgent, &peer.ConnOptions{
				Logger: slogtest.Make(t, nil).Leveled(slog.LevelDebug),
			})
			t.Cleanup(func() {
				_ = agentCloser.Close()
			})
		}()

		cmd, root := clitest.New(t, "ssh", workspace.Name)
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
		// Shells on Mac, Windows, and Linux all exit shells with the "exit" command.
		pty.WriteLine("exit")
		<-doneChan
	})
}
