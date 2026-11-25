package toolsdk

import (
	"context"
	"io"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsrelated"
)

// findWorkspaceAndAgent finds workspace and agent by name with auto-start support
func findWorkspaceAndAgent(ctx context.Context, client *codersdk.Client, workspaceName string) (codersdk.Workspace, codersdk.WorkspaceAgent, error) {
	// Parse workspace name to extract workspace and agent parts
	parts := strings.Split(workspaceName, ".")
	var agentName string
	if len(parts) >= 2 {
		agentName = parts[1]
		workspaceName = parts[0]
	}

	// Get workspace
	workspace, err := client.ResolveWorkspace(ctx, workspaceName)
	if err != nil {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
	}

	// Auto-start workspace if needed
	if workspace.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
		if workspace.LatestBuild.Transition == codersdk.WorkspaceTransitionDelete {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace %q is deleted", workspace.Name)
		}
		if workspace.LatestBuild.Job.Status == codersdk.ProvisionerJobFailed {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace %q is in failed state", workspace.Name)
		}
		if workspace.LatestBuild.Status != codersdk.WorkspaceStatusStopped {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace must be started; was unable to autostart as the last build job is %q, expected %q",
				workspace.LatestBuild.Status, codersdk.WorkspaceStatusStopped)
		}

		// Start workspace
		build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionStart,
		})
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("failed to start workspace: %w", err)
		}

		// Wait for build to complete
		if build.Job.CompletedAt == nil {
			err := cliui.WorkspaceBuild(ctx, io.Discard, client, build.ID)
			if err != nil {
				return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("failed to wait for build completion: %w", err)
			}
		}

		// Refresh workspace after build completes. Only the agent is read below.
		workspace, err = client.Workspace(ctx, workspace.ID, codersdk.WorkspaceOptions{
			IncludeRelated: &wsrelated.Config{
				LatestBuild: &wsrelated.LatestBuild{
					Resources: &wsrelated.Resources{
						Agents: &wsrelated.Agents{},
					},
				},
			},
		})
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
	}

	// Find agent
	workspaceAgent, err := getWorkspaceAgent(workspace, agentName)
	if err != nil {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
	}

	return workspace, workspaceAgent, nil
}

// getWorkspaceAgent finds the specified agent in the workspace
func getWorkspaceAgent(workspace codersdk.Workspace, agentName string) (codersdk.WorkspaceAgent, error) {
	resources := workspace.LatestBuild.Resources

	var agents []codersdk.WorkspaceAgent
	var availableNames []string

	for _, resource := range resources {
		for _, agent := range resource.Agents {
			availableNames = append(availableNames, agent.Name)
			agents = append(agents, agent)
		}
	}

	if len(agents) == 0 {
		return codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace %q has no agents", workspace.Name)
	}

	if agentName != "" {
		for _, agent := range agents {
			if agent.Name == agentName || agent.ID.String() == agentName {
				return agent, nil
			}
		}
		return codersdk.WorkspaceAgent{}, xerrors.Errorf("agent not found by name %q, available agents: %v", agentName, availableNames)
	}

	if len(agents) == 1 {
		return agents[0], nil
	}

	return codersdk.WorkspaceAgent{}, xerrors.Errorf("multiple agents found, please specify the agent name, available agents: %v", availableNames)
}
