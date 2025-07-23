package toolsdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type WorkspaceBashArgs struct {
	Workspace string `json:"workspace"`
	Command   string `json:"command"`
}

type WorkspaceBashResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

var WorkspaceBash = Tool[WorkspaceBashArgs, WorkspaceBashResult]{
	Tool: aisdk.Tool{
		Name: ToolNameWorkspaceBash,
		Description: `Execute a bash command in a Coder workspace.

This tool provides the same functionality as the 'coder ssh <workspace> <command>' CLI command.
It automatically starts the workspace if it's stopped and waits for the agent to be ready.
The output is trimmed of leading and trailing whitespace.

The workspace parameter supports various formats:
- workspace (uses current user)
- owner/workspace
- owner--workspace
- workspace.agent (specific agent)
- owner/workspace.agent

Examples:
- workspace: "my-workspace", command: "ls -la"
- workspace: "john/dev-env", command: "git status"
- workspace: "my-workspace.main", command: "docker ps"`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": map[string]any{
					"type":        "string",
					"description": "The workspace name in format [owner/]workspace[.agent]. If owner is not specified, the authenticated user is used.",
				},
				"command": map[string]any{
					"type":        "string",
					"description": "The bash command to execute in the workspace.",
				},
			},
			Required: []string{"workspace", "command"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args WorkspaceBashArgs) (WorkspaceBashResult, error) {
		if args.Workspace == "" {
			return WorkspaceBashResult{}, xerrors.New("workspace name cannot be empty")
		}
		if args.Command == "" {
			return WorkspaceBashResult{}, xerrors.New("command cannot be empty")
		}

		// Normalize workspace input to handle various formats
		workspaceName := NormalizeWorkspaceInput(args.Workspace)

		// Find workspace and agent
		_, workspaceAgent, err := findWorkspaceAndAgent(ctx, deps.coderClient, workspaceName)
		if err != nil {
			return WorkspaceBashResult{}, xerrors.Errorf("failed to find workspace: %w", err)
		}

		// Wait for agent to be ready
		if err := cliui.Agent(ctx, io.Discard, workspaceAgent.ID, cliui.AgentOptions{
			FetchInterval: 0,
			Fetch:         deps.coderClient.WorkspaceAgent,
			FetchLogs:     deps.coderClient.WorkspaceAgentLogsAfter,
			Wait:          true, // Always wait for startup scripts
		}); err != nil {
			return WorkspaceBashResult{}, xerrors.Errorf("agent not ready: %w", err)
		}

		// Create workspace SDK client for agent connection
		wsClient := workspacesdk.New(deps.coderClient)

		// Dial agent
		conn, err := wsClient.DialAgent(ctx, workspaceAgent.ID, &workspacesdk.DialAgentOptions{
			BlockEndpoints: false,
		})
		if err != nil {
			return WorkspaceBashResult{}, xerrors.Errorf("failed to dial agent: %w", err)
		}
		defer conn.Close()

		// Wait for connection to be reachable
		if !conn.AwaitReachable(ctx) {
			return WorkspaceBashResult{}, xerrors.New("agent connection not reachable")
		}

		// Create SSH client
		sshClient, err := conn.SSHClient(ctx)
		if err != nil {
			return WorkspaceBashResult{}, xerrors.Errorf("failed to create SSH client: %w", err)
		}
		defer sshClient.Close()

		// Create SSH session
		session, err := sshClient.NewSession()
		if err != nil {
			return WorkspaceBashResult{}, xerrors.Errorf("failed to create SSH session: %w", err)
		}
		defer session.Close()

		// Execute command and capture output
		output, err := session.CombinedOutput(args.Command)
		outputStr := strings.TrimSpace(string(output))

		if err != nil {
			// Check if it's an SSH exit error to get the exit code
			var exitErr *gossh.ExitError
			if errors.As(err, &exitErr) {
				return WorkspaceBashResult{
					Output:   outputStr,
					ExitCode: exitErr.ExitStatus(),
				}, nil
			}
			// For other errors, return exit code 1
			return WorkspaceBashResult{
				Output:   outputStr,
				ExitCode: 1,
			}, nil
		}

		return WorkspaceBashResult{
			Output:   outputStr,
			ExitCode: 0,
		}, nil
	},
}

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
	workspace, err := namedWorkspace(ctx, client, workspaceName)
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

		// Refresh workspace after build completes
		workspace, err = client.Workspace(ctx, workspace.ID)
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

// namedWorkspace gets a workspace by owner/name or just name
func namedWorkspace(ctx context.Context, client *codersdk.Client, identifier string) (codersdk.Workspace, error) {
	// Parse owner and workspace name
	parts := strings.SplitN(identifier, "/", 2)
	var owner, workspaceName string

	if len(parts) == 2 {
		owner = parts[0]
		workspaceName = parts[1]
	} else {
		owner = "me"
		workspaceName = identifier
	}

	// Handle -- separator format (convert to / format)
	if strings.Contains(identifier, "--") && !strings.Contains(identifier, "/") {
		dashParts := strings.SplitN(identifier, "--", 2)
		if len(dashParts) == 2 {
			owner = dashParts[0]
			workspaceName = dashParts[1]
		}
	}

	return client.WorkspaceByOwnerAndName(ctx, owner, workspaceName, codersdk.WorkspaceOptions{})
}

// NormalizeWorkspaceInput converts workspace name input to standard format.
// Handles the following input formats:
//   - workspace                    → workspace
//   - workspace.agent              → workspace.agent
//   - owner/workspace              → owner/workspace
//   - owner--workspace             → owner/workspace
//   - owner/workspace.agent        → owner/workspace.agent
//   - owner--workspace.agent       → owner/workspace.agent
//   - agent.workspace.owner        → owner/workspace.agent (Coder Connect format)
func NormalizeWorkspaceInput(input string) string {
	// Handle the special Coder Connect format: agent.workspace.owner
	// This format uses only dots and has exactly 3 parts
	if strings.Count(input, ".") == 2 && !strings.Contains(input, "/") && !strings.Contains(input, "--") {
		parts := strings.Split(input, ".")
		if len(parts) == 3 {
			// Convert agent.workspace.owner → owner/workspace.agent
			return fmt.Sprintf("%s/%s.%s", parts[2], parts[1], parts[0])
		}
	}

	// Convert -- separator to / separator for consistency
	normalized := strings.ReplaceAll(input, "--", "/")

	return normalized
}
