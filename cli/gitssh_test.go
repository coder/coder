package cli_test

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestGitSSH(t *testing.T) {
	t.Parallel()
	t.Run("Dial", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		instanceID := "instanceidentifier"
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		client := coderdtest.New(t, &coderdtest.Options{
			AWSInstanceIdentity: certificates,
		})
		user := coderdtest.CreateFirstUser(t, client)

		// get user public key
		keypair, err := client.GitSSHKey(ctx, codersdk.Me)
		require.NoError(t, err)
		publicKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(keypair.PublicKey))
		require.NoError(t, err)

		// setup provisioner
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "somename",
							Type: "someinstance",
							Agents: []*proto.Agent{{
								Auth: &proto.Agent_InstanceId{
									InstanceId: instanceID,
								},
							}},
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, codersdk.Me, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		// start workspace agent
		cmd, root := clitest.New(t, "workspaces", "agent", "--auth", "aws-instance-identity", "--url", client.URL.String())
		agentClient := &*client
		clitest.SetupConfig(t, agentClient, root)
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		go func() {
			// A linting error occurs for weakly typing the context value here,
			// but it seems reasonable for a one-off test.
			// nolint
			ctx = context.WithValue(ctx, "aws-client", metadataClient)
			err := cmd.ExecuteContext(ctx)
			require.NoError(t, err)
		}()
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)
		resources, err := client.WorkspaceResourcesByBuild(ctx, workspace.LatestBuild.ID)
		require.NoError(t, err)
		dialer, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil, nil)
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
		go func() {
			// as long as we get a successful session we don't care if the server errors
			_ = ssh.Serve(l, func(s ssh.Session) {
				atomic.AddInt64(&inc, 1)
				t.Log("got authenticated sesion")
				err := s.Exit(0)
				require.NoError(t, err)
			}, publicKeyOption)
		}()

		// start ssh session
		addr, ok := l.Addr().(*net.TCPAddr)
		require.True(t, ok)
		cfgDir := createConfig(cmd)
		// set to agent config dir
		cmd, root = clitest.New(t, "gitssh", "--global-config="+string(cfgDir), "--", fmt.Sprintf("-p%d", addr.Port), "-o", "StrictHostKeyChecking=no", "127.0.0.1")
		clitest.SetupConfig(t, agentClient, root)

		err = cmd.ExecuteContext(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 1, inc)
	})
}

// createConfig consumes the global configuration flag to produce a config root.
func createConfig(cmd *cobra.Command) config.Root {
	globalRoot, err := cmd.Flags().GetString("global-config")
	if err != nil {
		panic(err)
	}
	return config.Root(globalRoot)
}
