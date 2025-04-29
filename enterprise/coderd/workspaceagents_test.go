package coderd_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
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
		r := setupWorkspaceAgent(t, client, user, 0)
		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // Testing that even the owner gets blocked.
		_, err := workspacesdk.New(client).DialAgent(ctx, r.sdkAgent.ID, nil)
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
		r := setupWorkspaceAgent(t, client, user, 0)
		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // Testing RBAC is not the point of this test.
		conn, err := workspacesdk.New(client).DialAgent(ctx, r.sdkAgent.ID, nil)
		require.NoError(t, err)
		_ = conn.Close()
	})
}

func TestReinitializeAgent(t *testing.T) {
	t.Parallel()

	// GIVEN a live enterprise API with the prebuilds feature enabled
	client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				// TODO: enable the prebuilds feature and experiment
			},
		},
	})

	// GIVEN a template, template version, preset and a prebuilt workspace that uses them all
	presetID := uuid.New()
	tv := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
		OrganizationID: user.OrganizationID,
		CreatedBy:      user.UserID,
	}).Preset(database.TemplateVersionPreset{
		ID: presetID,
	}).Do()

	r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:    prebuilds.SystemUserID,
		TemplateID: tv.Template.ID,
	}).Seed(database.WorkspaceBuild{
		TemplateVersionID: tv.TemplateVersion.ID,
		TemplateVersionPresetID: uuid.NullUUID{
			UUID:  presetID,
			Valid: true,
		},
	}).WithAgent(func(a []*proto.Agent) []*proto.Agent {
		a[0].Scripts = []*proto.Script{
			{
				DisplayName: "Prebuild Test Script",
				Script:      "sleep 5", // Make reinitialization take long enough to assert that it happened
				RunOnStart:  true,
			},
		}
		return a
	}).Do()

	// GIVEN a running agent
	logDir := t.TempDir()
	inv, _ := clitest.New(t,
		"agent",
		"--auth", "token",
		"--agent-token", r.AgentToken,
		"--agent-url", client.URL.String(),
		"--log-dir", logDir,
	)
	clitest.Start(t, inv)

	// GIVEN the agent is in a happy steady state
	waiter := coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID)
	waiter.WaitFor(coderdtest.AgentsReady)

	// WHEN a workspace is created that can benefit from prebuilds
	ctx := testutil.Context(t, testutil.WaitShort)
	_, err := client.CreateUserWorkspace(ctx, user.UserID.String(), codersdk.CreateWorkspaceRequest{
		TemplateVersionID:       tv.TemplateVersion.ID,
		TemplateVersionPresetID: presetID,
		Name:                    "claimed-workspace",
	})
	require.NoError(t, err)

	// THEN the now claimed workspace agent reinitializes
	waiter.WaitFor(coderdtest.AgentsNotReady)

	// THEN reinitialization completes
	waiter.WaitFor(coderdtest.AgentsReady)
}

type setupResp struct {
	workspace codersdk.Workspace
	sdkAgent  codersdk.WorkspaceAgent
	agent     agent.Agent
}

func setupWorkspaceAgent(t *testing.T, client *codersdk.Client, user codersdk.CreateFirstUserResponse, appPort uint16) setupResp {
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
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
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
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
	agnt := agent.New(agent.Options{
		Client: agentClient,
		Logger: testutil.Logger(t).Named("agent"),
	})
	t.Cleanup(func() {
		_ = agnt.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	sdkAgent, err := client.WorkspaceAgent(ctx, resources[0].Agents[0].ID)
	require.NoError(t, err)

	return setupResp{workspace, sdkAgent, agnt}
}
