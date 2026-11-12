package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/agentsandbox/agentsandboxtest"
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

	t.Run("RecycledInstanceID", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		certificates, metadataClient := coderdtest.NewAWSInstanceIdentity(t, instanceID)
		setup := setupInstanceIDWorkspaceWithResources(t, &coderdtest.Options{
			AWSCertificates: certificates,
		}, workspaceAgentsForInstanceID(instanceID, "dev"))

		successorVersion := coderdtest.CreateTemplateVersion(t, setup.client, setup.user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionGraph: []*proto.Response{{
				Type: &proto.Response_Graph{
					Graph: &proto.GraphComplete{
						Resources: []*proto.Resource{{
							Name:   "resource",
							Type:   "instance",
							Agents: workspaceAgentsForInstanceID(newTestInstanceID(t), "dev"),
						}},
					},
				},
			}},
		}, func(req *codersdk.CreateTemplateVersionRequest) {
			req.TemplateID = setup.template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, setup.client, successorVersion.ID)
		build := coderdtest.CreateWorkspaceBuild(t, setup.client, setup.workspace, database.WorkspaceTransitionStart, func(req *codersdk.CreateWorkspaceBuildRequest) {
			req.TemplateVersionID = successorVersion.ID
		})
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, setup.client, build.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(setup.client.URL, agentsdk.WithAWSInstanceIdentity())
		agentClient.SDK.HTTPClient = metadataClient

		err := agentClient.RefreshToken(ctx)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		// The prior build's agent is soft-deleted when the successor
		// build completes (SoftDeletePriorWorkspaceAgents), so the
		// auth query finds no candidates at all and returns 404.
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
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

		res := postAWSInstanceIdentity(ctx, t, client, metadataClient, "")
		defer res.Body.Close()

		require.Equal(t, http.StatusConflict, res.StatusCode)
		err := codersdk.ReadBodyAsError(res)
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

		res := postAWSInstanceIdentity(ctx, t, client, metadataClient, "   ")
		defer res.Body.Close()

		require.Equal(t, http.StatusConflict, res.StatusCode)
		err := codersdk.ReadBodyAsError(res)
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
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, true, "coder")
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
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false, "coder")
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
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false, "coder")
		client, store := setupInstanceIDWorkspace(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		}, workspaceAgentsForInstanceID(instanceID, "dev"))
		expectedAgent := requireWorkspaceAgentByInstanceIDAndName(t, store, instanceID, "dev")

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithGoogleInstanceIdentity("", metadata))
		err := agentClient.RefreshToken(ctx)
		require.NoError(t, err)
		require.Equal(t, expectedAgent.AuthToken.String(), agentClient.SDK.SessionToken())
	})

	t.Run("ForeignAudience", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false, "other-service")
		client, _ := setupInstanceIDWorkspace(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		}, workspaceAgentsForInstanceID(instanceID, "dev"))

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		agentClient := agentsdk.New(client.URL, agentsdk.WithGoogleInstanceIdentity("", metadata))
		err := agentClient.RefreshToken(ctx)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("Ambiguous/GoogleWithSelector", func(t *testing.T) {
		t.Parallel()

		instanceID := newTestInstanceID(t)
		validator, metadata := coderdtest.NewGoogleInstanceIdentity(t, instanceID, false, "coder")
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

type instanceIDWorkspaceSetup struct {
	client    *codersdk.Client
	store     database.Store
	user      codersdk.CreateFirstUserResponse
	template  codersdk.Template
	workspace codersdk.Workspace
}

func setupInstanceIDWorkspace(t *testing.T, opts *coderdtest.Options, agents []*proto.Agent) (*codersdk.Client, database.Store) {
	t.Helper()

	setup := setupInstanceIDWorkspaceWithResources(t, opts, agents)
	return setup.client, setup.store
}

func setupInstanceIDWorkspaceWithResources(
	t *testing.T,
	opts *coderdtest.Options,
	agents []*proto.Agent,
) instanceIDWorkspaceSetup {
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

	return instanceIDWorkspaceSetup{
		client:    client,
		store:     store,
		user:      user,
		template:  template,
		workspace: workspace,
	}
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

const awsInstanceIdentityMetadataURL = "http://169.254.169.254/latest/dynamic/instance-identity"

func postAWSInstanceIdentity(
	ctx context.Context,
	t testing.TB,
	client *codersdk.Client,
	metadataClient *http.Client,
	agentName string,
) *http.Response {
	t.Helper()

	signature := readAWSInstanceMetadata(ctx, t, metadataClient, "signature")
	document := readAWSInstanceMetadata(ctx, t, metadataClient, "document")
	reqBody, err := json.Marshal(map[string]string{
		"signature":  signature,
		"document":   document,
		"agent_name": agentName,
	})
	require.NoError(t, err)

	res, err := client.RequestWithoutSessionToken(
		ctx,
		http.MethodPost,
		"/api/v2/workspaceagents/aws-instance-identity",
		reqBody,
	)
	require.NoError(t, err)
	return res
}

func readAWSInstanceMetadata(
	ctx context.Context,
	t testing.TB,
	metadataClient *http.Client,
	path string,
) string {
	t.Helper()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		awsInstanceIdentityMetadataURL+"/"+path,
		nil,
	)
	require.NoError(t, err)
	res, err := metadataClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return string(body)
}

func newTestInstanceID(t testing.TB) string {
	t.Helper()
	return fmt.Sprintf("instance-%d", time.Now().UnixNano())
}

func TestPostWorkspaceAuthKubernetesPodIdentity(t *testing.T) {
	t.Parallel()

	// tokenFile is a pod's token as the kubelet mounts it.
	tokenFile := func(t *testing.T, token string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(path, []byte(token+"\n"), 0o600))
		return path
	}

	t.Run("NotConfigured", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		ctx := testutil.Context(t, testutil.WaitShort)
		agentClient := agentsdk.New(client.URL, agentsdk.WithKubernetesPodIdentity(tokenFile(t, "anything")))
		err := agentClient.RefreshToken(ctx)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotImplemented, apiErr.StatusCode())
	})

	t.Run("Refused", func(t *testing.T) {
		t.Parallel()
		cluster := agentsandboxtest.NewCluster(t)
		client := coderdtest.New(t, &coderdtest.Options{SandboxCluster: cluster.AsCluster()})
		// Whatever the cluster does not confirm as a pod's token is refused,
		// and refused at once: there is no pod to wait for.
		for name, token := range map[string]string{
			"forged":           "made-up",
			"not bound to pod": cluster.NewServiceAccountToken(),
		} {
			ctx := testutil.Context(t, testutil.WaitShort)
			agentClient := agentsdk.New(client.URL, agentsdk.WithKubernetesPodIdentity(tokenFile(t, token)))
			err := agentClient.RefreshToken(ctx)
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr, name)
			require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode(), name)
		}
	})

	t.Run("WaitsToBeClaimed", func(t *testing.T) {
		t.Parallel()
		cluster := agentsandboxtest.NewCluster(t)
		// The pod runs its daemon before any workspace has claimed it, as
		// a pod of a warm pool does.
		pod := cluster.NewPod("warm-1")
		stranger := cluster.NewPod("warm-2")

		client, store := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			SandboxCluster:           cluster.AsCluster(),
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		agentClient := agentsdk.New(client.URL, agentsdk.WithKubernetesPodIdentity(tokenFile(t, pod.Token)))
		authenticated := make(chan error, 1)
		go func() { authenticated <- agentClient.RefreshToken(ctx) }()
		// Nobody has claimed the pod: its daemon is nobody, and waits.
		select {
		case err := <-authenticated:
			t.Fatalf("a pod that no workspace has claimed was answered: %v", err)
		case <-time.After(testutil.IntervalSlow):
		}

		// A workspace build registers the pod, as the sandbox provisioner
		// does once it has claimed it.
		instanceID := "k8s-pod:" + pod.UID
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			ProvisionGraph: []*proto.Response{{
				Type: &proto.Response_Graph{
					Graph: &proto.GraphComplete{
						Resources: []*proto.Resource{{
							Name:   "pod",
							Type:   "agent_sandbox",
							Agents: workspaceAgentsForInstanceID(instanceID, "main"),
						}},
					},
				},
			}},
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		// The waiting daemon is told, and is the daemon of that workspace.
		require.NoError(t, testutil.TryReceive(ctx, t, authenticated))
		agent := requireWorkspaceAgentByInstanceIDAndName(t, store, instanceID, "main")
		require.Equal(t, agent.AuthToken.String(), agentClient.GetSessionToken())

		// Another pod of the same pool, with a token just as genuine, is
		// still nobody.
		res, err := client.RequestWithoutSessionToken(ctx, http.MethodPost, "/api/v2/workspaceagents/kubernetes-pod-identity",
			agentsdk.KubernetesPodIdentityToken{Token: stranger.Token})
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})
}
