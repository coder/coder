package coderd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestGitSSHKey(t *testing.T) {
	t.Parallel()
	t.Run("None", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, nil)
		res := coderdtest.CreateFirstUser(t, client)
		key, err := client.GitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("Ed25519", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmEd25519,
		})
		res := coderdtest.CreateFirstUser(t, client)
		key, err := client.GitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("ECDSA", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmECDSA,
		})
		res := coderdtest.CreateFirstUser(t, client)
		key, err := client.GitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("RSA4096", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmRSA4096,
		})
		res := coderdtest.CreateFirstUser(t, client)
		key, err := client.GitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("Regenerate", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmEd25519,
		})
		res := coderdtest.CreateFirstUser(t, client)
		key1, err := client.GitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.NotEmpty(t, key1.PublicKey)
		key2, err := client.RegenerateGitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.GreaterOrEqual(t, key2.UpdatedAt, key1.UpdatedAt)
		require.NotEmpty(t, key2.PublicKey)
		require.NotEqual(t, key2.PublicKey, key1.PublicKey)
	})
}

func TestAgentGitSSHKey(t *testing.T) {
	t.Parallel()

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
						}},
					}},
				},
			},
		}},
	})
	project := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	daemonCloser.Close()

	agentClient := codersdk.New(client.URL)
	agentClient.SessionToken = authToken

	agentKey, err := agentClient.AgentGitSSHKey(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, agentKey.PrivateKey)
}
