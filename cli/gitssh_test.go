package cli_test

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"

	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestGitSSH(t *testing.T) {
	t.Parallel()
	t.Run("Dial", func(t *testing.T) {
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)

		// get user public key
		keypair, err := client.GitSSHKey(context.Background(), codersdk.Me)
		require.NoError(t, err)
		publicKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(keypair.PublicKey))
		require.NoError(t, err)

		// setup template
		agentToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:           echo.ParseComplete,
			ProvisionDryRun: echo.ProvisionComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "somename",
							Type: "someinstance",
							Agents: []*proto.Agent{{
								Auth: &proto.Agent_Token{
									Token: agentToken,
								},
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

		// start workspace agent
		cmd, root := clitest.New(t, "agent", "--agent-token", agentToken, "--agent-url", client.URL.String())
		agentClient := client
		clitest.SetupConfig(t, agentClient, root)
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		agentErrC := make(chan error)
		go func() {
			agentErrC <- cmd.ExecuteContext(ctx)
		}()

		coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
		resources, err := client.WorkspaceResourcesByBuild(context.Background(), workspace.LatestBuild.ID)
		require.NoError(t, err)
		dialer, err := client.DialWorkspaceAgent(context.Background(), resources[0].Agents[0].ID, nil)
		require.NoError(t, err)
		defer dialer.Close()
		_, err = dialer.Ping()
		require.NoError(t, err)

		// start ssh server
		l, err := net.Listen("tcp", "localhost:0")
		require.NoError(t, err)
		defer l.Close()
		publicKeyOption := ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			return ssh.KeysEqual(publicKey, key)
		})
		var inc int64
		sshErrC := make(chan error)
		go func() {
			// as long as we get a successful session we don't care if the server errors
			_ = ssh.Serve(l, func(s ssh.Session) {
				atomic.AddInt64(&inc, 1)
				t.Log("got authenticated session")
				sshErrC <- s.Exit(0)
			}, publicKeyOption)
		}()

		// start ssh session
		addr, ok := l.Addr().(*net.TCPAddr)
		require.True(t, ok)
		// set to agent config dir
		gitsshCmd, _ := clitest.New(t, "gitssh", "--agent-url", agentClient.URL.String(), "--agent-token", agentToken, "--", fmt.Sprintf("-p%d", addr.Port), "-o", "StrictHostKeyChecking=no", "-o", "IdentitiesOnly=yes", "127.0.0.1")
		err = gitsshCmd.ExecuteContext(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, 1, inc)

		err = <-sshErrC
		require.NoError(t, err, "error in ssh session exit")

		cancelFunc()
		err = <-agentErrC
		require.NoError(t, err, "error in agent execute")
	})
}
