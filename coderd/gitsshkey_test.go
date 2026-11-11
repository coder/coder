package coderd_test

import (
	"context"
	"crypto/sha512"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/gitsshkey"
	"github.com/coder/coder/v2/codersdk"
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
		ProvisionGraph: echo.ProvisionGraphWithAgent(authToken),
	})
	project := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, project.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(authToken))

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
		{apiKeyScope: "all", expectError: false},
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
				ProvisionGraph: echo.ProvisionGraphWithAgentAndAPIKeyScope(authToken, tt.apiKeyScope),
			})
			project := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			workspace := coderdtest.CreateWorkspace(t, client, project.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(authToken))

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			_, err := agentClient.GitSSHKey(ctx)

			if tt.expectError {
				require.Error(t, err)
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAgentGitSign(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T, experiments ...codersdk.Experiment) (*codersdk.Client, *agentsdk.Client) {
		t.Helper()
		dv := coderdtest.DeploymentValues(t)
		for _, e := range experiments {
			dv.Experiments = append(dv.Experiments, string(e))
		}
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv})
		user := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()
		return client, agentsdk.New(client.URL, agentsdk.WithFixedToken(r.AgentToken))
	}
	digest := sha512.Sum512([]byte("commit payload"))
	req := agentsdk.GitSignRequest{Namespace: "git", HashAlgorithm: "sha512", Hash: digest[:]}

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()
		_, agentClient := setup(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := agentClient.GitSign(ctx, req)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, string(codersdk.ExperimentGitCommitSigning))
	})

	t.Run("Signs", func(t *testing.T) {
		t.Parallel()
		_, agentClient := setup(t, codersdk.ExperimentGitCommitSigning)
		ctx := testutil.Context(t, testutil.WaitLong)

		resp, err := agentClient.GitSign(ctx, req)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(resp.Signature, "-----BEGIN SSH SIGNATURE-----\n"))

		// Only the git namespace may be signed.
		_, err = agentClient.GitSign(ctx, agentsdk.GitSignRequest{Namespace: "file", HashAlgorithm: "sha512", Hash: digest[:]})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())

		// The digest must match the declared algorithm.
		_, err = agentClient.GitSign(ctx, agentsdk.GitSignRequest{Namespace: "git", HashAlgorithm: "sha256", Hash: digest[:]})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("UserSessionRejected", func(t *testing.T) {
		t.Parallel()
		client, _ := setup(t, codersdk.ExperimentGitCommitSigning)
		ctx := testutil.Context(t, testutil.WaitLong)

		// A user session token is not a workspace agent token.
		userAsAgent := agentsdk.New(client.URL, agentsdk.WithFixedToken(client.SessionToken()))
		_, err := userAsAgent.GitSign(ctx, req)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
	})
}

func TestAgentGitSign_APIKeyScopes(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		apiKeyScope string
		expectError bool
	}{
		{apiKeyScope: "all", expectError: false},
		{apiKeyScope: "no_user_data", expectError: true},
	} {
		t.Run(tt.apiKeyScope, func(t *testing.T) {
			t.Parallel()

			dv := coderdtest.DeploymentValues(t)
			dv.Experiments = []string{string(codersdk.ExperimentGitCommitSigning)}
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				DeploymentValues:         dv,
			})
			user := coderdtest.CreateFirstUser(t, client)
			authToken := uuid.NewString()
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
				Parse:          echo.ParseComplete,
				ProvisionPlan:  echo.PlanComplete,
				ProvisionGraph: echo.ProvisionGraphWithAgentAndAPIKeyScope(authToken, tt.apiKeyScope),
			})
			project := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			workspace := coderdtest.CreateWorkspace(t, client, project.ID)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(authToken))
			ctx := testutil.Context(t, testutil.WaitLong)

			digest := sha512.Sum512([]byte("commit payload"))
			_, err := agentClient.GitSign(ctx, agentsdk.GitSignRequest{Namespace: "git", HashAlgorithm: "sha512", Hash: digest[:]})
			if tt.expectError {
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
