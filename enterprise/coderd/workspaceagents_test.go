package coderd_test

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/serpent"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
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

	if runtime.GOOS == "windows" {
		t.Skip("test startup script is not supported on windows")
	}

	// Ensure that workspace agents can reinitialize against claimed prebuilds in non-default organizations:
	for _, useDefaultOrg := range []bool{true, false} {
		t.Run(fmt.Sprintf("useDefaultOrg=%t", useDefaultOrg), func(t *testing.T) {
			t.Parallel()

			tempAgentLog := testutil.CreateTemp(t, "", "testReinitializeAgent")

			startupScript := fmt.Sprintf("printenv >> %s; echo '---\n' >> %s", tempAgentLog.Name(), tempAgentLog.Name())

			db, ps := dbtestutil.NewDB(t)
			// GIVEN a live enterprise API with the prebuilds feature enabled
			client, user := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Database: db,
					Pubsub:   ps,
					DeploymentValues: coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
						dv.Prebuilds.ReconciliationInterval = serpent.Duration(time.Second)
					}),
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureWorkspacePrebuilds:         1,
						codersdk.FeatureExternalProvisionerDaemons: 1,
					},
				},
			})

			orgID := user.OrganizationID
			if !useDefaultOrg {
				secondOrg := dbgen.Organization(t, db, database.Organization{})
				orgID = secondOrg.ID
			}
			provisionerCloser := coderdenttest.NewExternalProvisionerDaemon(t, client, orgID, map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			})
			defer provisionerCloser.Close()

			// GIVEN a template, template version, preset and a prebuilt workspace that uses them all
			agentToken := uuid.UUID{3}
			version := coderdtest.CreateTemplateVersion(t, client, orgID, &echo.Responses{
				Parse: echo.ParseComplete,
				ProvisionPlan: []*proto.Response{
					{
						Type: &proto.Response_Plan{
							Plan: &proto.PlanComplete{
								Presets: []*proto.Preset{
									{
										Name: "test-preset",
										Prebuild: &proto.Prebuild{
											Instances: 1,
										},
									},
								},
								Resources: []*proto.Resource{
									{
										Agents: []*proto.Agent{
											{
												Name:            "smith",
												OperatingSystem: "linux",
												Architecture:    "i386",
											},
										},
									},
								},
							},
						},
					},
				},
				ProvisionApply: []*proto.Response{
					{
						Type: &proto.Response_Apply{
							Apply: &proto.ApplyComplete{
								Resources: []*proto.Resource{
									{
										Type: "compute",
										Name: "main",
										Agents: []*proto.Agent{
											{
												Name:            "smith",
												OperatingSystem: "linux",
												Architecture:    "i386",
												Scripts: []*proto.Script{
													{
														RunOnStart: true,
														Script:     startupScript,
													},
												},
												Auth: &proto.Agent_Token{
													Token: agentToken.String(),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			})
			coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

			coderdtest.CreateTemplate(t, client, orgID, version.ID)

			// Wait for prebuilds to create a prebuilt workspace
			ctx := testutil.Context(t, testutil.WaitLong)
			var prebuildID uuid.UUID
			require.Eventually(t, func() bool {
				agentAndBuild, err := db.GetWorkspaceAgentAndLatestBuildByAuthToken(ctx, agentToken)
				if err != nil {
					return false
				}
				prebuildID = agentAndBuild.WorkspaceBuild.ID
				return true
			}, testutil.WaitLong, testutil.IntervalFast)

			prebuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, prebuildID)

			preset, err := db.GetPresetByWorkspaceBuildID(ctx, prebuildID)
			require.NoError(t, err)

			// GIVEN a running agent
			logDir := t.TempDir()
			inv, _ := clitest.New(t,
				"agent",
				"--auth", "token",
				"--agent-token", agentToken.String(),
				"--agent-url", client.URL.String(),
				"--log-dir", logDir,
			)
			clitest.Start(t, inv)

			// GIVEN the agent is in a happy steady state
			waiter := coderdtest.NewWorkspaceAgentWaiter(t, client, prebuild.WorkspaceID)
			waiter.WaitFor(coderdtest.AgentsReady)

			// WHEN a workspace is created that can benefit from prebuilds
			anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, orgID)
			workspace, err := anotherClient.CreateUserWorkspace(ctx, anotherUser.ID.String(), codersdk.CreateWorkspaceRequest{
				TemplateVersionID:       version.ID,
				TemplateVersionPresetID: preset.ID,
				Name:                    "claimed-workspace",
			})
			require.NoError(t, err)

			coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

			// THEN reinitialization completes
			waiter.WaitFor(coderdtest.AgentsReady)

			var matches [][]byte
			require.Eventually(t, func() bool {
				// THEN the agent script ran again and reused the same agent token
				contents, err := os.ReadFile(tempAgentLog.Name())
				if err != nil {
					return false
				}
				// UUID regex pattern (matches UUID v4-like strings)
				uuidRegex := regexp.MustCompile(`\bCODER_AGENT_TOKEN=(.+)\b`)

				matches = uuidRegex.FindAll(contents, -1)
				// When an agent reinitializes, we expect it to run startup scripts again.
				// As such, we expect to have written the agent environment to the temp file twice.
				// Once on initial startup and then once on reinitialization.
				return len(matches) == 2
			}, testutil.WaitLong, testutil.IntervalMedium)
			require.Equal(t, matches[0], matches[1])
		})
	}
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
	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(authToken))
	agentClient.SDK.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				//nolint:gosec
				InsecureSkipVerify: true,
			},
		},
	}
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

func TestWorkspaceExternalAgentCredentials(t *testing.T) {
	t.Parallel()

	client, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspaceExternalAgent: 1,
			},
		},
	})

	t.Run("Success - linux", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).Seed(database.WorkspaceBuild{
			HasExternalAgent: sql.NullBool{
				Bool:  true,
				Valid: true,
			},
		}).Resource(&proto.Resource{
			Name: "test-agent",
			Type: "coder_external_agent",
		}).WithAgent(func(a []*proto.Agent) []*proto.Agent {
			a[0].Name = "test-agent"
			a[0].OperatingSystem = "linux"
			a[0].Architecture = "amd64"
			return a
		}).Do()

		credentials, err := client.WorkspaceExternalAgentCredentials(
			ctx, r.Workspace.ID, "test-agent")
		require.NoError(t, err)

		require.Equal(t, r.AgentToken, credentials.AgentToken)
		expectedCommand := fmt.Sprintf("curl -fsSL \"%s/api/v2/init-script/linux/amd64\" | CODER_AGENT_TOKEN=%q sh", client.URL, r.AgentToken)
		require.Equal(t, expectedCommand, credentials.Command)
	})

	t.Run("Success - windows", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).Resource(&proto.Resource{
			Name: "test-agent",
			Type: "coder_external_agent",
		}).Seed(database.WorkspaceBuild{
			HasExternalAgent: sql.NullBool{
				Bool:  true,
				Valid: true,
			},
		}).WithAgent(func(a []*proto.Agent) []*proto.Agent {
			a[0].Name = "test-agent"
			a[0].OperatingSystem = "windows"
			a[0].Architecture = "amd64"
			return a
		}).Do()

		credentials, err := client.WorkspaceExternalAgentCredentials(
			ctx, r.Workspace.ID, "test-agent")
		require.NoError(t, err)

		require.Equal(t, r.AgentToken, credentials.AgentToken)
		expectedCommand := fmt.Sprintf("$env:CODER_AGENT_TOKEN=%q; iwr -useb \"%s/api/v2/init-script/windows/amd64\" | iex", r.AgentToken, client.URL)
		require.Equal(t, expectedCommand, credentials.Command)
	})

	t.Run("WithInstanceID - should return 404", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).Seed(database.WorkspaceBuild{
			HasExternalAgent: sql.NullBool{
				Bool:  true,
				Valid: true,
			},
		}).Resource(&proto.Resource{
			Name: "test-agent",
			Type: "coder_external_agent",
		}).WithAgent(func(a []*proto.Agent) []*proto.Agent {
			a[0].Name = "test-agent"
			a[0].Auth = &proto.Agent_InstanceId{
				InstanceId: uuid.New().String(),
			}
			return a
		}).Do()

		_, err := client.WorkspaceExternalAgentCredentials(ctx, r.Workspace.ID, "test-agent")
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, "External agent is authenticated with an instance ID.", apiErr.Message)
	})

	t.Run("No external agent - should return 404", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).Do()

		_, err := client.WorkspaceExternalAgentCredentials(ctx, r.Workspace.ID, "test-agent")
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, "Workspace does not have an external agent.", apiErr.Message)
	})
}
