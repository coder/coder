package agentcontainers_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agenttest"
	agentproto "github.com/coder/coder/v2/agent/proto"
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

				agentClient, _, err := agentAPI.ConnectRPC26(ctx)
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
}
