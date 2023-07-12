package coderd_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

// App names for each app sharing level.
const (
	testAppNameOwner         = "test-app-owner"
	testAppNameAuthenticated = "test-app-authenticated"
	testAppNamePublic        = "test-app-public"
)

func TestBlockNonBrowser(t *testing.T) {
	t.Parallel()
	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			BrowserOnly: true,
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureBrowserOnly: 1,
				},
			},
		})
		_, agent := setupWorkspaceAgent(t, client, user, 0)
		_, err := client.DialWorkspaceAgent(context.Background(), agent.ID, nil)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})
	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureBrowserOnly: 0,
				},
			},
		})
		_, agent := setupWorkspaceAgent(t, client, user, 0)
		conn, err := client.DialWorkspaceAgent(context.Background(), agent.ID, nil)
		require.NoError(t, err)
		_ = conn.Close()
	})
}

func setupWorkspaceAgent(t *testing.T, client *codersdk.Client, user codersdk.CreateFirstUserResponse, appPort uint16) (codersdk.Workspace, codersdk.WorkspaceAgent) {
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionApply: []*proto.Provision_Response{{
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
							Apps: []*proto.App{
								{
									Slug:         testAppNameOwner,
									DisplayName:  testAppNameOwner,
									SharingLevel: proto.AppSharingLevel_OWNER,
									Url:          fmt.Sprintf("http://localhost:%d", appPort),
								},
								{
									Slug:         testAppNameAuthenticated,
									DisplayName:  testAppNameAuthenticated,
									SharingLevel: proto.AppSharingLevel_AUTHENTICATED,
									Url:          fmt.Sprintf("http://localhost:%d", appPort),
								},
								{
									Slug:         testAppNamePublic,
									DisplayName:  testAppNamePublic,
									SharingLevel: proto.AppSharingLevel_PUBLIC,
									Url:          fmt.Sprintf("http://localhost:%d", appPort),
								},
							},
						}},
					}},
				},
			},
		}},
	})
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	agentClient := agentsdk.New(client.URL)
	agentClient.SDK.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				//nolint:gosec
				InsecureSkipVerify: true,
			},
		},
	}
	agentClient.SetSessionToken(authToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: slogtest.Make(t, nil).Named("agent"),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	agnt, err := client.WorkspaceAgent(ctx, resources[0].Agents[0].ID)
	require.NoError(t, err)

	return workspace, agnt
}
