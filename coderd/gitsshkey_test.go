package coderd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/gitsshkey"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestGitSSHKey(t *testing.T) {
	t.Parallel()
	t.Run("None", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		res := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		key, err := client.GitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("Ed25519", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmEd25519,
		})
		res := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		key, err := client.GitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("ECDSA", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmECDSA,
		})
		res := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		key, err := client.GitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("RSA4096", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmRSA4096,
		})
		res := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		key, err := client.GitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("Regenerate", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmEd25519,
			Auditor:            auditor,
		})
		res := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		key1, err := client.GitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.NotEmpty(t, key1.PublicKey)
		key2, err := client.RegenerateGitSSHKey(ctx, res.UserID.String())
		require.NoError(t, err)
		require.GreaterOrEqual(t, key2.UpdatedAt, key1.UpdatedAt)
		require.NotEmpty(t, key2.PublicKey)

		require.Len(t, auditor.AuditLogs(), 2)
		assert.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[1].Action)
	})
}

func TestAgentGitSSHKey(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	project := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, project.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	agentKey, err := agentClient.GitSSHKey(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, agentKey.PrivateKey)
}

func TestAgentGitSSHKey_APIKeyScopes(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		apiKeyScope string
		expectError bool
	}{
		{apiKeyScope: "default", expectError: false},
		{apiKeyScope: "no_user_data", expectError: true},
	} {
		t.Run(tt.apiKeyScope, func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			})
			user := coderdtest.CreateFirstUser(t, client)
			authToken := uuid.NewString()
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
				Parse:          echo.ParseComplete,
				ProvisionPlan:  echo.PlanComplete,
				ProvisionApply: echo.ProvisionApplyWithAgentAndAPIKeyScope(authToken, tt.apiKeyScope),
			})
			project := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			workspace := coderdtest.CreateWorkspace(t, client, project.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			agentClient := agentsdk.New(client.URL)
			agentClient.SetSessionToken(authToken)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			_, err := agentClient.GitSSHKey(ctx)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
