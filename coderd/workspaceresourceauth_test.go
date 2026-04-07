package coderd_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestPostWorkspaceAuthAzureInstanceIdentity(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		certificates, metadataClient := coderdtest.NewAzureInstanceIdentity(t, instanceID)
		client, _ := setupInstanceIDWorkspace(t, &coderdtest.Options{
			AzureCertificates: certificates,
		}, workspaceAgentsForInstanceID(instanceID, "dev"))

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithAzureInstanceIdentity())
		agentClient.SDK.HTTPClient = metadataClient

		err := agentClient.RefreshToken(ctx)
		require.NoError(t, err)
	})

	t.Run("Ambiguous/AzureWithSelector", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		certificates, metadataClient := coderdtest.NewAzureInstanceIdentity(t, instanceID)
		client, store := setupInstanceIDWorkspace(t, &coderdtest.Options{
			AzureCertificates: certificates,
		}, workspaceAgentsForInstanceID(instanceID, "alpha", "beta"))

		expectedAgent := requireWorkspaceAgentByInstanceIDAndName(t, store, instanceID, "alpha")
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithAzureInstanceIdentity(
			agentsdk.WithInstanceIdentityAgentName("alpha"),
		))
		agentClient.SDK.HTTPClient = metadataClient

		err := agentClient.RefreshToken(ctx)
		require.NoError(t, err)
		require.Equal(t, expectedAgent.AuthToken.String(), agentClient.SDK.SessionToken())
	})
}

func TestPostWorkspaceAuthAWSInstanceIdentity(t *testing.T) {
	t.Parallel()

	t.Run("Ambiguous/SingleAgent", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		client, _ := setupInstanceIDWorkspace(t, &coderdtest.Options{
			AWSCertificates: certificates,
		}, workspaceAgentsForInstanceID(instanceID, "dev"))

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithAWSInstanceIdentity())
		agentClient.SDK.HTTPClient = metadataClient

		err := agentClient.RefreshToken(ctx)
		require.NoError(t, err)
	})

	t.Run("Ambiguous/MultipleAgentsNoSelector", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		client, _ := setupInstanceIDWorkspace(t, &coderdtest.Options{
			AWSCertificates: certificates,
		}, workspaceAgentsForInstanceID(instanceID, "alpha", "beta"))

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithAWSInstanceIdentity())
		agentClient.SDK.HTTPClient = metadataClient

		err := agentClient.RefreshToken(ctx)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "CODER_AGENT_NAME")
		require.Contains(t, apiErr.Message, "alpha, beta")
	})

	t.Run("Ambiguous/MultipleAgentsWithSelector", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		client, store := setupInstanceIDWorkspace(t, &coderdtest.Options{
			AWSCertificates: certificates,
		}, workspaceAgentsForInstanceID(instanceID, "alpha", "beta"))

		expectedAgent := requireWorkspaceAgentByInstanceIDAndName(t, store, instanceID, "alpha")
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithAWSInstanceIdentity(
			agentsdk.WithInstanceIdentityAgentName("alpha"),
		))
		agentClient.SDK.HTTPClient = metadataClient

		err := agentClient.RefreshToken(ctx)
		require.NoError(t, err)
		require.Equal(t, expectedAgent.AuthToken.String(), agentClient.SDK.SessionToken())
	})

	t.Run("Ambiguous/MultipleAgentsUnknownSelector", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		client, _ := setupInstanceIDWorkspace(t, &coderdtest.Options{
			AWSCertificates: certificates,
		}, workspaceAgentsForInstanceID(instanceID, "alpha", "beta"))

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithAWSInstanceIdentity(
			agentsdk.WithInstanceIdentityAgentName("nonexistent"),
		))
		agentClient.SDK.HTTPClient = metadataClient

		err := agentClient.RefreshToken(ctx)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Ambiguous/SubAgentExcluded", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		client, store := setupInstanceIDWorkspace(t, &coderdtest.Options{
			AWSCertificates: certificates,
		}, workspaceAgentsForInstanceID(instanceID, "dev"))

		rootAgent := requireWorkspaceAgentByInstanceIDAndName(t, store, instanceID, "dev")
		_ = dbgen.WorkspaceSubAgent(t, store, rootAgent, database.WorkspaceAgent{
			Name: "sub",
			AuthInstanceID: sql.NullString{
				String: instanceID,
				Valid:  true,
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithAWSInstanceIdentity())
		agentClient.SDK.HTTPClient = metadataClient

		err := agentClient.RefreshToken(ctx)
		require.NoError(t, err)
		require.Equal(t, rootAgent.AuthToken.String(), agentClient.SDK.SessionToken())
	})
}

func TestPostWorkspaceAuthGoogleInstanceIdentity(t *testing.T) {
	t.Parallel()

	t.Run("Expired", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, true)
		client := coderdtest.New(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithGoogleInstanceIdentity("", metadata))
		err := agentClient.RefreshToken(ctx)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("InstanceNotFound", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false)
		client := coderdtest.New(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithGoogleInstanceIdentity("", metadata))
		err := agentClient.RefreshToken(ctx)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false)
		client, _ := setupInstanceIDWorkspace(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		}, workspaceAgentsForInstanceID(instanceID, "dev"))

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithGoogleInstanceIdentity("", metadata))
		err := agentClient.RefreshToken(ctx)
		require.NoError(t, err)
	})

	t.Run("Ambiguous/GoogleWithSelector", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false)
		client, store := setupInstanceIDWorkspace(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		}, workspaceAgentsForInstanceID(instanceID, "alpha", "beta"))

		expectedAgent := requireWorkspaceAgentByInstanceIDAndName(t, store, instanceID, "alpha")
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithGoogleInstanceIdentity(
			"",
			metadata,
			agentsdk.WithInstanceIdentityAgentName("alpha"),
		))
		err := agentClient.RefreshToken(ctx)
		require.NoError(t, err)
		require.Equal(t, expectedAgent.AuthToken.String(), agentClient.SDK.SessionToken())
	})
}

func setupInstanceIDWorkspace(t *testing.T, opts *coderdtest.Options, agents []*proto.Agent) (*codersdk.Client, database.Store) {
	t.Helper()

	actualOpts := &coderdtest.Options{}
	if opts != nil {
		*actualOpts = *opts
	}
	actualOpts.IncludeProvisionerDaemon = true

	client, store := coderdtest.NewWithDatabase(t, actualOpts)
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionGraph: []*proto.Response{{
			Type: &proto.Response_Graph{
				Graph: &proto.GraphComplete{
					Resources: []*proto.Resource{{
						Name:   "resource",
						Type:   "instance",
						Agents: agents,
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	return client, store
}

func workspaceAgentsForInstanceID(instanceID string, names ...string) []*proto.Agent {
	agents := make([]*proto.Agent, 0, len(names))
	for _, name := range names {
		agents = append(agents, &proto.Agent{
			Name: name,
			Auth: &proto.Agent_InstanceId{InstanceId: instanceID},
		})
	}
	return agents
}

func requireWorkspaceAgentByInstanceIDAndName(t testing.TB, store database.Store, instanceID string, name string) database.WorkspaceAgent {
	t.Helper()

	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong))
	agent, err := store.GetWorkspaceAgentByInstanceIDAndName(ctx, database.GetWorkspaceAgentByInstanceIDAndNameParams{
		AuthInstanceID: instanceID,
		Name:           name,
	})
	require.NoError(t, err)
	return agent
}

func newTestInstanceID(t testing.TB) string {
	t.Helper()
	return fmt.Sprintf("instance-%d", time.Now().UnixNano())
}
