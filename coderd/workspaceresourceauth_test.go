package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
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

	t.Run("Ambiguous/EmptyAgentNameTreatedAsUnset", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		client, _ := setupInstanceIDWorkspace(t, &coderdtest.Options{
			AWSCertificates: certificates,
		}, workspaceAgentsForInstanceID(instanceID, "alpha", "beta"))

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		signatureReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/signature", nil)
		require.NoError(t, err)
		signatureRes, err := metadataClient.Do(signatureReq)
		require.NoError(t, err)
		defer signatureRes.Body.Close()
		signature, err := io.ReadAll(signatureRes.Body)
		require.NoError(t, err)

		documentReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/document", nil)
		require.NoError(t, err)
		documentRes, err := metadataClient.Do(documentReq)
		require.NoError(t, err)
		defer documentRes.Body.Close()
		document, err := io.ReadAll(documentRes.Body)
		require.NoError(t, err)

		reqBody, err := json.Marshal(map[string]string{
			"signature":  string(signature),
			"document":   string(document),
			"agent_name": "",
		})
		require.NoError(t, err)

		res, err := client.RequestWithoutSessionToken(ctx, http.MethodPost, "/api/v2/workspaceagents/aws-instance-identity", reqBody)
		require.NoError(t, err)
		defer res.Body.Close()

		require.Equal(t, http.StatusConflict, res.StatusCode)
		err = codersdk.ReadBodyAsError(res)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "CODER_AGENT_NAME")
		require.Contains(t, apiErr.Message, "alpha, beta")
	})

	t.Run("Ambiguous/WhitespaceAgentNameTreatedAsUnset", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		client, _ := setupInstanceIDWorkspace(t, &coderdtest.Options{
			AWSCertificates: certificates,
		}, workspaceAgentsForInstanceID(instanceID, "alpha", "beta"))

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		signatureReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/signature", nil)
		require.NoError(t, err)
		signatureRes, err := metadataClient.Do(signatureReq)
		require.NoError(t, err)
		defer signatureRes.Body.Close()
		signature, err := io.ReadAll(signatureRes.Body)
		require.NoError(t, err)

		documentReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/document", nil)
		require.NoError(t, err)
		documentRes, err := metadataClient.Do(documentReq)
		require.NoError(t, err)
		defer documentRes.Body.Close()
		document, err := io.ReadAll(documentRes.Body)
		require.NoError(t, err)

		reqBody, err := json.Marshal(map[string]string{
			"signature":  string(signature),
			"document":   string(document),
			"agent_name": "   ",
		})
		require.NoError(t, err)

		res, err := client.RequestWithoutSessionToken(ctx, http.MethodPost, "/api/v2/workspaceagents/aws-instance-identity", reqBody)
		require.NoError(t, err)
		defer res.Body.Close()

		require.Equal(t, http.StatusConflict, res.StatusCode)
		err = codersdk.ReadBodyAsError(res)
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

	// Rebuilding a workspace inserts a new workspace_agents row that
	// keeps the original auth_instance_id, so historical agents
	// accumulate on long-lived instances. The latest build's agent
	// should authenticate; agents bound to prior builds must be
	// excluded from the candidate set so a single live agent is not
	// misreported as multi-agent ambiguity.
	t.Run("Ambiguous/PriorBuildAgentsExcluded", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		client, store := setupInstanceIDWorkspace(t, &coderdtest.Options{
			AWSCertificates: certificates,
		}, workspaceAgentsForInstanceID(instanceID, "dev"))

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		sysCtx := dbauthz.AsSystemRestricted(ctx)

		// Walk from the first agent back to its workspace.
		staleAgent := requireWorkspaceAgentByInstanceIDAndName(t, store, instanceID, "dev")
		staleResource, err := store.GetWorkspaceResourceByID(sysCtx, staleAgent.ResourceID)
		require.NoError(t, err)
		firstBuild, err := store.GetWorkspaceBuildByJobID(sysCtx, staleResource.JobID)
		require.NoError(t, err)
		workspace, err := client.Workspace(ctx, firstBuild.WorkspaceID)
		require.NoError(t, err)

		// Stop then restart so the workspace ends on a fresh Start build
		// with a brand-new agent reusing the same instance ID.
		stopBuild := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStop)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, stopBuild.ID)
		startBuild := coderdtest.CreateWorkspaceBuild(t, client, workspace, database.WorkspaceTransitionStart)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, startBuild.ID)

		// More than one agent now shares the instance ID: every workspace
		// build of this template echoes the same agent definition, so the
		// original Start build's agent plus every subsequent build's agent
		// all carry the same auth_instance_id. Without the latest-build
		// filter the handler would treat this as multi-agent ambiguity and
		// 409.
		allAgents, err := store.GetWorkspaceAgentsByInstanceID(sysCtx, instanceID)
		require.NoError(t, err)
		require.Greater(t, len(allAgents), 1, "expected stale and fresh agents to share the instance ID")

		latestBuild, err := store.GetLatestWorkspaceBuildByWorkspaceID(sysCtx, workspace.ID)
		require.NoError(t, err)
		latestResources, err := store.GetWorkspaceResourcesByJobID(sysCtx, latestBuild.JobID)
		require.NoError(t, err)
		require.NotEmpty(t, latestResources)
		latestAgents, err := store.GetWorkspaceAgentsByResourceIDs(sysCtx, []uuid.UUID{latestResources[0].ID})
		require.NoError(t, err)
		require.Len(t, latestAgents, 1)
		freshAgent := latestAgents[0]
		require.NotEqual(t, staleAgent.ID, freshAgent.ID)

		agentClient := agentsdk.New(client.URL, agentsdk.WithAWSInstanceIdentity())
		agentClient.SDK.HTTPClient = metadataClient

		err = agentClient.RefreshToken(ctx)
		require.NoError(t, err)
		require.Equal(t, freshAgent.AuthToken.String(), agentClient.SDK.SessionToken())
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
	agents, err := store.GetWorkspaceAgentsByInstanceID(ctx, instanceID)
	require.NoError(t, err)
	for _, agent := range agents {
		if agent.Name == name {
			return agent
		}
	}
	require.FailNow(t, "workspace agent not found", "instance ID %q, name %q", instanceID, name)
	return database.WorkspaceAgent{}
}

func newTestInstanceID(t testing.TB) string {
	t.Helper()
	return fmt.Sprintf("instance-%d", time.Now().UnixNano())
}
