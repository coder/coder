package cli_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestShow(t *testing.T) {
	t.Parallel()
	t.Run("Exists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithAgent())
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, member, template.ID)
		build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		args := []string{
			"show",
			workspace.Name,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		ctx := testutil.Context(t, testutil.WaitShort)
		go func() {
			defer close(doneChan)
			err := inv.WithContext(ctx).Run()
			assert.NoError(t, err)
		}()
		matches := []struct {
			match string
			write string
		}{
			{match: fmt.Sprintf("%s/%s", workspace.OwnerName, workspace.Name)},
			{match: fmt.Sprintf("(%s since ", build.Status)},
			{match: fmt.Sprintf("%s:%s", workspace.TemplateName, workspace.LatestBuild.TemplateVersionName)},
			{match: "compute.main"},
			{match: "smith (linux, i386)"},
			{match: "coder ssh " + workspace.Name},
		}
		for _, m := range matches {
			pty.ExpectMatchContext(ctx, m.match)
			if len(m.write) > 0 {
				pty.WriteLine(m.write)
			}
		}
		_ = testutil.TryReceive(ctx, t, doneChan)
	})
}

func TestShowDevcontainers_Golden(t *testing.T) {
	t.Parallel()

	mainAgentID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	agentID := mainAgentID

	testCases := []struct {
		name           string
		showDetails    bool
		devcontainers  []codersdk.WorkspaceAgentDevcontainer
		listeningPorts map[uuid.UUID]codersdk.WorkspaceAgentListeningPortsResponse
	}{
		{
			name: "running_devcontainer_with_agent",
			devcontainers: []codersdk.WorkspaceAgentDevcontainer{
				{
					ID:              uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					Name:            "web-dev",
					WorkspaceFolder: "/workspaces/web-dev",
					ConfigPath:      "/workspaces/web-dev/.devcontainer/devcontainer.json",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusRunning,
					Dirty:           false,
					Container: &codersdk.WorkspaceAgentContainer{
						ID:           "container-web-dev",
						FriendlyName: "quirky_lovelace",
						Image:        "mcr.microsoft.com/devcontainers/typescript-node:1.0.0",
						Running:      true,
						Status:       "running",
						CreatedAt:    time.Now().Add(-1 * time.Hour),
						Labels: map[string]string{
							agentcontainers.DevcontainerConfigFileLabel:  "/workspaces/web-dev/.devcontainer/devcontainer.json",
							agentcontainers.DevcontainerLocalFolderLabel: "/workspaces/web-dev",
						},
					},
					Agent: &codersdk.WorkspaceAgentDevcontainerAgent{
						ID:        uuid.MustParse("22222222-2222-2222-2222-222222222222"),
						Name:      "web-dev",
						Directory: "/workspaces/web-dev",
					},
				},
			},
			listeningPorts: map[uuid.UUID]codersdk.WorkspaceAgentListeningPortsResponse{
				uuid.MustParse("22222222-2222-2222-2222-222222222222"): {
					Ports: []codersdk.WorkspaceAgentListeningPort{
						{
							ProcessName: "node",
							Network:     "tcp",
							Port:        3000,
						},
						{
							ProcessName: "webpack-dev-server",
							Network:     "tcp",
							Port:        8080,
						},
					},
				},
			},
		},
		{
			name: "running_devcontainer_without_agent",
			devcontainers: []codersdk.WorkspaceAgentDevcontainer{
				{
					ID:              uuid.MustParse("33333333-3333-3333-3333-333333333333"),
					Name:            "web-server",
					WorkspaceFolder: "/workspaces/web-server",
					ConfigPath:      "/workspaces/web-server/.devcontainer/devcontainer.json",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusRunning,
					Dirty:           false,
					Container: &codersdk.WorkspaceAgentContainer{
						ID:           "container-web-server",
						FriendlyName: "amazing_turing",
						Image:        "nginx:latest",
						Running:      true,
						Status:       "running",
						CreatedAt:    time.Now().Add(-30 * time.Minute),
						Labels: map[string]string{
							agentcontainers.DevcontainerConfigFileLabel:  "/workspaces/web-server/.devcontainer/devcontainer.json",
							agentcontainers.DevcontainerLocalFolderLabel: "/workspaces/web-server",
						},
					},
					Agent: nil, // No agent for this running container.
				},
			},
		},
		{
			name: "stopped_devcontainer",
			devcontainers: []codersdk.WorkspaceAgentDevcontainer{
				{
					ID:              uuid.MustParse("44444444-4444-4444-4444-444444444444"),
					Name:            "api-dev",
					WorkspaceFolder: "/workspaces/api-dev",
					ConfigPath:      "/workspaces/api-dev/.devcontainer/devcontainer.json",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusStopped,
					Dirty:           false,
					Container: &codersdk.WorkspaceAgentContainer{
						ID:           "container-api-dev",
						FriendlyName: "clever_darwin",
						Image:        "mcr.microsoft.com/devcontainers/go:1.0.0",
						Running:      false,
						Status:       "exited",
						CreatedAt:    time.Now().Add(-2 * time.Hour),
						Labels: map[string]string{
							agentcontainers.DevcontainerConfigFileLabel:  "/workspaces/api-dev/.devcontainer/devcontainer.json",
							agentcontainers.DevcontainerLocalFolderLabel: "/workspaces/api-dev",
						},
					},
					Agent: nil, // No agent for stopped container.
				},
			},
		},
		{
			name: "starting_devcontainer",
			devcontainers: []codersdk.WorkspaceAgentDevcontainer{
				{
					ID:              uuid.MustParse("55555555-5555-5555-5555-555555555555"),
					Name:            "database-dev",
					WorkspaceFolder: "/workspaces/database-dev",
					ConfigPath:      "/workspaces/database-dev/.devcontainer/devcontainer.json",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusStarting,
					Dirty:           false,
					Container: &codersdk.WorkspaceAgentContainer{
						ID:           "container-database-dev",
						FriendlyName: "nostalgic_hawking",
						Image:        "mcr.microsoft.com/devcontainers/postgres:1.0.0",
						Running:      false,
						Status:       "created",
						CreatedAt:    time.Now().Add(-5 * time.Minute),
						Labels: map[string]string{
							agentcontainers.DevcontainerConfigFileLabel:  "/workspaces/database-dev/.devcontainer/devcontainer.json",
							agentcontainers.DevcontainerLocalFolderLabel: "/workspaces/database-dev",
						},
					},
					Agent: nil, // No agent yet while starting.
				},
			},
		},
		{
			name: "error_devcontainer",
			devcontainers: []codersdk.WorkspaceAgentDevcontainer{
				{
					ID:              uuid.MustParse("66666666-6666-6666-6666-666666666666"),
					Name:            "failed-dev",
					WorkspaceFolder: "/workspaces/failed-dev",
					ConfigPath:      "/workspaces/failed-dev/.devcontainer/devcontainer.json",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusError,
					Dirty:           false,
					Error:           "Failed to pull image mcr.microsoft.com/devcontainers/go:latest: timeout after 5m0s",
					Container:       nil, // No container due to error.
					Agent:           nil, // No agent due to error.
				},
			},
		},

		{
			name: "mixed_devcontainer_states",
			devcontainers: []codersdk.WorkspaceAgentDevcontainer{
				{
					ID:              uuid.MustParse("88888888-8888-8888-8888-888888888888"),
					Name:            "frontend",
					WorkspaceFolder: "/workspaces/frontend",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusRunning,
					Container: &codersdk.WorkspaceAgentContainer{
						ID:           "container-frontend",
						FriendlyName: "vibrant_tesla",
						Image:        "node:18",
						Running:      true,
						Status:       "running",
						CreatedAt:    time.Now().Add(-30 * time.Minute),
					},
					Agent: &codersdk.WorkspaceAgentDevcontainerAgent{
						ID:        uuid.MustParse("99999999-9999-9999-9999-999999999999"),
						Name:      "frontend",
						Directory: "/workspaces/frontend",
					},
				},
				{
					ID:              uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
					Name:            "backend",
					WorkspaceFolder: "/workspaces/backend",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusStopped,
					Container: &codersdk.WorkspaceAgentContainer{
						ID:           "container-backend",
						FriendlyName: "peaceful_curie",
						Image:        "python:3.11",
						Running:      false,
						Status:       "exited",
						CreatedAt:    time.Now().Add(-1 * time.Hour),
					},
					Agent: nil,
				},
				{
					ID:              uuid.MustParse("bbbbbbbb-cccc-dddd-eeee-ffffffffffff"),
					Name:            "error-container",
					WorkspaceFolder: "/workspaces/error-container",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusError,
					Error:           "Container build failed: dockerfile syntax error on line 15",
					Container:       nil,
					Agent:           nil,
				},
			},
			listeningPorts: map[uuid.UUID]codersdk.WorkspaceAgentListeningPortsResponse{
				uuid.MustParse("99999999-9999-9999-9999-999999999999"): {
					Ports: []codersdk.WorkspaceAgentListeningPort{
						{
							ProcessName: "vite",
							Network:     "tcp",
							Port:        5173,
						},
					},
				},
			},
		},
		{
			name: "running_devcontainer_with_agent_and_error",
			devcontainers: []codersdk.WorkspaceAgentDevcontainer{
				{
					ID:              uuid.MustParse("cccccccc-dddd-eeee-ffff-000000000000"),
					Name:            "problematic-dev",
					WorkspaceFolder: "/workspaces/problematic-dev",
					ConfigPath:      "/workspaces/problematic-dev/.devcontainer/devcontainer.json",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusRunning,
					Dirty:           false,
					Error:           "Warning: Container started but healthcheck failed",
					Container: &codersdk.WorkspaceAgentContainer{
						ID:           "container-problematic",
						FriendlyName: "cranky_mendel",
						Image:        "mcr.microsoft.com/devcontainers/python:1.0.0",
						Running:      true,
						Status:       "running",
						CreatedAt:    time.Now().Add(-15 * time.Minute),
						Labels: map[string]string{
							agentcontainers.DevcontainerConfigFileLabel:  "/workspaces/problematic-dev/.devcontainer/devcontainer.json",
							agentcontainers.DevcontainerLocalFolderLabel: "/workspaces/problematic-dev",
						},
					},
					Agent: &codersdk.WorkspaceAgentDevcontainerAgent{
						ID:        uuid.MustParse("dddddddd-eeee-ffff-aaaa-111111111111"),
						Name:      "problematic-dev",
						Directory: "/workspaces/problematic-dev",
					},
				},
			},
			listeningPorts: map[uuid.UUID]codersdk.WorkspaceAgentListeningPortsResponse{
				uuid.MustParse("dddddddd-eeee-ffff-aaaa-111111111111"): {
					Ports: []codersdk.WorkspaceAgentListeningPort{
						{
							ProcessName: "python",
							Network:     "tcp",
							Port:        8000,
						},
					},
				},
			},
		},
		{
			name: "long_error_message",
			devcontainers: []codersdk.WorkspaceAgentDevcontainer{
				{
					ID:              uuid.MustParse("eeeeeeee-ffff-0000-1111-222222222222"),
					Name:            "long-error-dev",
					WorkspaceFolder: "/workspaces/long-error-dev",
					ConfigPath:      "/workspaces/long-error-dev/.devcontainer/devcontainer.json",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusError,
					Dirty:           false,
					Error:           "Failed to build devcontainer: dockerfile parse error at line 25: unknown instruction 'INSTALL', did you mean 'RUN apt-get install'? This is a very long error message that should be truncated when detail flag is not used",
					Container:       nil,
					Agent:           nil,
				},
			},
		},
		{
			name:        "long_error_message_with_detail",
			showDetails: true,
			devcontainers: []codersdk.WorkspaceAgentDevcontainer{
				{
					ID:              uuid.MustParse("eeeeeeee-ffff-0000-1111-222222222222"),
					Name:            "long-error-dev",
					WorkspaceFolder: "/workspaces/long-error-dev",
					ConfigPath:      "/workspaces/long-error-dev/.devcontainer/devcontainer.json",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusError,
					Dirty:           false,
					Error:           "Failed to build devcontainer: dockerfile parse error at line 25: unknown instruction 'INSTALL', did you mean 'RUN apt-get install'? This is a very long error message that should be truncated when detail flag is not used",
					Container:       nil,
					Agent:           nil,
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var allAgents []codersdk.WorkspaceAgent
			mainAgent := codersdk.WorkspaceAgent{
				ID:              mainAgentID,
				Name:            "main",
				OperatingSystem: "linux",
				Architecture:    "amd64",
				Status:          codersdk.WorkspaceAgentConnected,
				Health:          codersdk.WorkspaceAgentHealth{Healthy: true},
				Version:         "v2.15.0",
			}
			allAgents = append(allAgents, mainAgent)

			for _, dc := range tc.devcontainers {
				if dc.Agent != nil {
					devcontainerAgent := codersdk.WorkspaceAgent{
						ID:              dc.Agent.ID,
						ParentID:        uuid.NullUUID{UUID: mainAgentID, Valid: true},
						Name:            dc.Agent.Name,
						OperatingSystem: "linux",
						Architecture:    "amd64",
						Status:          codersdk.WorkspaceAgentConnected,
						Health:          codersdk.WorkspaceAgentHealth{Healthy: true},
						Version:         "v2.15.0",
					}
					allAgents = append(allAgents, devcontainerAgent)
				}
			}

			resources := []codersdk.WorkspaceResource{
				{
					Type:   "compute",
					Name:   "main",
					Agents: allAgents,
				},
			}
			options := cliui.WorkspaceResourcesOptions{
				WorkspaceName: "test-workspace",
				ServerVersion: "v2.15.0",
				ShowDetails:   tc.showDetails,
				Devcontainers: map[uuid.UUID]codersdk.WorkspaceAgentListContainersResponse{
					agentID: {
						Devcontainers: tc.devcontainers,
					},
				},
				ListeningPorts: tc.listeningPorts,
			}

			var buf bytes.Buffer
			err := cliui.WorkspaceResources(&buf, resources, options)
			require.NoError(t, err)

			replacements := map[string]string{}
			clitest.TestGoldenFile(t, "TestShowDevcontainers_Golden/"+tc.name, buf.Bytes(), replacements)
		})
	}
}
