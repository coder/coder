package agentcontainers_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agenttest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func TestSubAgentClient_CreateWithDisplayApps(t *testing.T) {
	t.Parallel()

	t.Run("CreateWithDisplayApps", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			displayApps  []codersdk.DisplayApp
			expectedApps []agentproto.CreateSubAgentRequest_DisplayApp
		}{
			{
				name:        "single display app",
				displayApps: []codersdk.DisplayApp{codersdk.DisplayAppVSCodeDesktop},
				expectedApps: []agentproto.CreateSubAgentRequest_DisplayApp{
					agentproto.CreateSubAgentRequest_VSCODE,
				},
			},
			{
				name: "multiple display apps",
				displayApps: []codersdk.DisplayApp{
					codersdk.DisplayAppVSCodeDesktop,
					codersdk.DisplayAppSSH,
					codersdk.DisplayAppPortForward,
				},
				expectedApps: []agentproto.CreateSubAgentRequest_DisplayApp{
					agentproto.CreateSubAgentRequest_VSCODE,
					agentproto.CreateSubAgentRequest_SSH_HELPER,
					agentproto.CreateSubAgentRequest_PORT_FORWARDING_HELPER,
				},
			},
			{
				name: "all display apps",
				displayApps: []codersdk.DisplayApp{
					codersdk.DisplayAppPortForward,
					codersdk.DisplayAppSSH,
					codersdk.DisplayAppVSCodeDesktop,
					codersdk.DisplayAppVSCodeInsiders,
					codersdk.DisplayAppWebTerminal,
				},
				expectedApps: []agentproto.CreateSubAgentRequest_DisplayApp{
					agentproto.CreateSubAgentRequest_PORT_FORWARDING_HELPER,
					agentproto.CreateSubAgentRequest_SSH_HELPER,
					agentproto.CreateSubAgentRequest_VSCODE,
					agentproto.CreateSubAgentRequest_VSCODE_INSIDERS,
					agentproto.CreateSubAgentRequest_WEB_TERMINAL,
				},
			},
			{
				name:        "no display apps",
				displayApps: []codersdk.DisplayApp{},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitShort)
				logger := testutil.Logger(t)
				statsCh := make(chan *agentproto.Stats)

				agentAPI := agenttest.NewClient(t, logger, uuid.New(), agentsdk.Manifest{}, statsCh, tailnet.NewCoordinator(logger))

				agentClient, _, err := agentAPI.ConnectRPC28(ctx)
				require.NoError(t, err)

				subAgentClient := agentcontainers.NewSubAgentClientFromAPI(logger, agentClient)

				// When: We create a sub agent with display apps.
				subAgent, err := subAgentClient.Create(ctx, agentcontainers.SubAgent{
					Name:            "sub-agent-" + tt.name,
					Directory:       "/workspaces/coder",
					Architecture:    "amd64",
					OperatingSystem: "linux",
					DisplayApps:     tt.displayApps,
				})
				require.NoError(t, err)

				displayApps, err := agentAPI.GetSubAgentDisplayApps(subAgent.ID)
				require.NoError(t, err)

				// Then: We expect the apps to be created.
				require.Equal(t, tt.expectedApps, displayApps)
			})
		}
	})

	t.Run("CreateWithApps", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			apps         []agentcontainers.SubAgentApp
			expectedApps []*agentproto.CreateSubAgentRequest_App
		}{
			{
				name: "SlugOnly",
				apps: []agentcontainers.SubAgentApp{
					{
						Slug: "code-server",
					},
				},
				expectedApps: []*agentproto.CreateSubAgentRequest_App{
					{
						Slug: "code-server",
					},
				},
			},
			{
				name: "AllFields",
				apps: []agentcontainers.SubAgentApp{
					{
						Slug:        "jupyter",
						Command:     "jupyter lab --port=8888",
						DisplayName: "Jupyter Lab",
						External:    false,
						Group:       "Development",
						HealthCheck: agentcontainers.SubAgentHealthCheck{
							Interval:  30,
							Threshold: 3,
							URL:       "http://localhost:8888/api",
						},
						Hidden:    false,
						Icon:      "/icon/jupyter.svg",
						OpenIn:    codersdk.WorkspaceAppOpenInTab,
						Order:     int32(1),
						Share:     codersdk.WorkspaceAppSharingLevelAuthenticated,
						Subdomain: true,
						URL:       "http://localhost:8888",
					},
				},
				expectedApps: []*agentproto.CreateSubAgentRequest_App{
					{
						Slug:        "jupyter",
						Command:     ptr.Ref("jupyter lab --port=8888"),
						DisplayName: ptr.Ref("Jupyter Lab"),
						External:    ptr.Ref(false),
						Group:       ptr.Ref("Development"),
						Healthcheck: &agentproto.CreateSubAgentRequest_App_Healthcheck{
							Interval:  30,
							Threshold: 3,
							Url:       "http://localhost:8888/api",
						},
						Hidden:    ptr.Ref(false),
						Icon:      ptr.Ref("/icon/jupyter.svg"),
						OpenIn:    agentproto.CreateSubAgentRequest_App_TAB.Enum(),
						Order:     ptr.Ref(int32(1)),
						Share:     agentproto.CreateSubAgentRequest_App_AUTHENTICATED.Enum(),
						Subdomain: ptr.Ref(true),
						Url:       ptr.Ref("http://localhost:8888"),
					},
				},
			},
			{
				name: "AllSharingLevels",
				apps: []agentcontainers.SubAgentApp{
					{
						Slug:  "owner-app",
						Share: codersdk.WorkspaceAppSharingLevelOwner,
					},
					{
						Slug:  "authenticated-app",
						Share: codersdk.WorkspaceAppSharingLevelAuthenticated,
					},
					{
						Slug:  "public-app",
						Share: codersdk.WorkspaceAppSharingLevelPublic,
					},
					{
						Slug:  "organization-app",
						Share: codersdk.WorkspaceAppSharingLevelOrganization,
					},
				},
				expectedApps: []*agentproto.CreateSubAgentRequest_App{
					{
						Slug:  "owner-app",
						Share: agentproto.CreateSubAgentRequest_App_OWNER.Enum(),
					},
					{
						Slug:  "authenticated-app",
						Share: agentproto.CreateSubAgentRequest_App_AUTHENTICATED.Enum(),
					},
					{
						Slug:  "public-app",
						Share: agentproto.CreateSubAgentRequest_App_PUBLIC.Enum(),
					},
					{
						Slug:  "organization-app",
						Share: agentproto.CreateSubAgentRequest_App_ORGANIZATION.Enum(),
					},
				},
			},
			{
				name: "WithHealthCheck",
				apps: []agentcontainers.SubAgentApp{
					{
						Slug: "health-app",
						HealthCheck: agentcontainers.SubAgentHealthCheck{
							Interval:  60,
							Threshold: 5,
							URL:       "http://localhost:3000/health",
						},
					},
				},
				expectedApps: []*agentproto.CreateSubAgentRequest_App{
					{
						Slug: "health-app",
						Healthcheck: &agentproto.CreateSubAgentRequest_App_Healthcheck{
							Interval:  60,
							Threshold: 5,
							Url:       "http://localhost:3000/health",
						},
					},
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitShort)
				logger := testutil.Logger(t)
				statsCh := make(chan *agentproto.Stats)

				agentAPI := agenttest.NewClient(t, logger, uuid.New(), agentsdk.Manifest{}, statsCh, tailnet.NewCoordinator(logger))

				agentClient, _, err := agentAPI.ConnectRPC28(ctx)
				require.NoError(t, err)

				subAgentClient := agentcontainers.NewSubAgentClientFromAPI(logger, agentClient)

				// When: We create a sub agent with display apps.
				subAgent, err := subAgentClient.Create(ctx, agentcontainers.SubAgent{
					Name:            "sub-agent-" + tt.name,
					Directory:       "/workspaces/coder",
					Architecture:    "amd64",
					OperatingSystem: "linux",
					Apps:            tt.apps,
				})
				require.NoError(t, err)

				apps, err := agentAPI.GetSubAgentApps(subAgent.ID)
				require.NoError(t, err)

				// Then: We expect the apps to be created.
				require.Len(t, apps, len(tt.expectedApps))
				for i, expectedApp := range tt.expectedApps {
					actualApp := apps[i]

					assert.Equal(t, expectedApp.Slug, actualApp.Slug)
					assert.Equal(t, expectedApp.Command, actualApp.Command)
					assert.Equal(t, expectedApp.DisplayName, actualApp.DisplayName)
					assert.Equal(t, ptr.NilToEmpty(expectedApp.External), ptr.NilToEmpty(actualApp.External))
					assert.Equal(t, expectedApp.Group, actualApp.Group)
					assert.Equal(t, ptr.NilToEmpty(expectedApp.Hidden), ptr.NilToEmpty(actualApp.Hidden))
					assert.Equal(t, expectedApp.Icon, actualApp.Icon)
					assert.Equal(t, ptr.NilToEmpty(expectedApp.Order), ptr.NilToEmpty(actualApp.Order))
					assert.Equal(t, ptr.NilToEmpty(expectedApp.Subdomain), ptr.NilToEmpty(actualApp.Subdomain))
					assert.Equal(t, expectedApp.Url, actualApp.Url)

					if expectedApp.OpenIn != nil {
						require.NotNil(t, actualApp.OpenIn)
						assert.Equal(t, *expectedApp.OpenIn, *actualApp.OpenIn)
					} else {
						assert.Equal(t, expectedApp.OpenIn, actualApp.OpenIn)
					}

					if expectedApp.Share != nil {
						require.NotNil(t, actualApp.Share)
						assert.Equal(t, *expectedApp.Share, *actualApp.Share)
					} else {
						assert.Equal(t, expectedApp.Share, actualApp.Share)
					}

					if expectedApp.Healthcheck != nil {
						require.NotNil(t, expectedApp.Healthcheck)
						assert.Equal(t, expectedApp.Healthcheck.Interval, actualApp.Healthcheck.Interval)
						assert.Equal(t, expectedApp.Healthcheck.Threshold, actualApp.Healthcheck.Threshold)
						assert.Equal(t, expectedApp.Healthcheck.Url, actualApp.Healthcheck.Url)
					} else {
						assert.Equal(t, expectedApp.Healthcheck, actualApp.Healthcheck)
					}
				}
			})
		}
	})
}

func TestSubAgent_CloneConfig(t *testing.T) {
	t.Parallel()

	t.Run("CopiesIDFromDevcontainer", func(t *testing.T) {
		t.Parallel()

		subAgent := agentcontainers.SubAgent{
			ID:              uuid.New(),
			Name:            "original-name",
			Directory:       "/workspace",
			Architecture:    "amd64",
			OperatingSystem: "linux",
			DisplayApps:     []codersdk.DisplayApp{codersdk.DisplayAppVSCodeDesktop},
			Apps:            []agentcontainers.SubAgentApp{{Slug: "app1"}},
		}
		expectedID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
		dc := codersdk.WorkspaceAgentDevcontainer{
			Name:       "devcontainer-name",
			SubagentID: uuid.NullUUID{UUID: expectedID, Valid: true},
		}

		cloned := subAgent.CloneConfig(dc)

		assert.Equal(t, expectedID, cloned.ID)
		assert.Equal(t, dc.Name, cloned.Name)
		assert.Equal(t, subAgent.Directory, cloned.Directory)
		assert.Equal(t, uuid.Nil, cloned.AuthToken, "AuthToken should not be copied")
	})

	t.Run("HandlesNilSubagentID", func(t *testing.T) {
		t.Parallel()

		subAgent := agentcontainers.SubAgent{
			ID:              uuid.New(),
			Name:            "original-name",
			Directory:       "/workspace",
			Architecture:    "amd64",
			OperatingSystem: "linux",
		}
		dc := codersdk.WorkspaceAgentDevcontainer{
			Name:       "devcontainer-name",
			SubagentID: uuid.NullUUID{Valid: false},
		}

		cloned := subAgent.CloneConfig(dc)

		assert.Equal(t, uuid.Nil, cloned.ID)
	})
}

func TestSubAgent_EqualConfig(t *testing.T) {
	t.Parallel()

	t.Run("TrueWhenFieldsMatch", func(t *testing.T) {
		t.Parallel()

		a := agentcontainers.SubAgent{
			ID:              uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			Name:            "test-agent",
			Directory:       "/workspace",
			Architecture:    "amd64",
			OperatingSystem: "linux",
			DisplayApps:     []codersdk.DisplayApp{codersdk.DisplayAppVSCodeDesktop},
			Apps:            []agentcontainers.SubAgentApp{{Slug: "app1"}},
		}
		// Different ID but same config fields.
		b := agentcontainers.SubAgent{
			ID:              uuid.MustParse("660e8400-e29b-41d4-a716-446655440000"),
			Name:            "test-agent",
			Directory:       "/workspace",
			Architecture:    "amd64",
			OperatingSystem: "linux",
			DisplayApps:     []codersdk.DisplayApp{codersdk.DisplayAppVSCodeDesktop},
			Apps:            []agentcontainers.SubAgentApp{{Slug: "app1"}},
		}

		assert.True(t, a.EqualConfig(b), "EqualConfig compares config fields, not ID")
	})

	t.Run("FalseWhenFieldsDiffer", func(t *testing.T) {
		t.Parallel()

		a := agentcontainers.SubAgent{
			Name:            "test-agent",
			Directory:       "/workspace",
			Architecture:    "amd64",
			OperatingSystem: "linux",
		}
		b := agentcontainers.SubAgent{
			Name:            "different-name",
			Directory:       "/workspace",
			Architecture:    "amd64",
			OperatingSystem: "linux",
		}

		assert.False(t, a.EqualConfig(b))
	})
}
