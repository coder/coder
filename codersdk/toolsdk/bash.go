package toolsdk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

type WorkspaceBashArgs struct {
	Workspace  string `json:"workspace"`
	Command    string `json:"command"`
	TimeoutMs  int    `json:"timeout_ms,omitempty"`
	Background bool   `json:"background,omitempty"`
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

The timeout_ms parameter specifies the command timeout in milliseconds (defaults to 60000ms, maximum of 300000ms).
If the command times out, all output captured up to that point is returned with a cancellation message.

For background commands (background: true), output is captured until the timeout is reached, then the command
continues running in the background. The captured output is returned as the result.

For file operations (list, write, edit), always prefer the dedicated file tools.
Do not use bash commands (ls, cat, echo, heredoc, etc.) to list, write, or read
files when the file tools are available. The bash tool should be used for:

	- Running commands and scripts
	- Installing packages
	- Starting services
	- Executing programs

Examples:
- workspace: "john/dev-env", command: "git status", timeout_ms: 30000
- workspace: "my-workspace", command: "npm run dev", background: true, timeout_ms: 10000
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
				"timeout_ms": map[string]any{
					"type":        "integer",
					"description": "Command timeout in milliseconds. Defaults to 60000ms (60 seconds) if not specified.",
					"default":     60000,
					"minimum":     1,
				},
				"background": map[string]any{
					"type":        "boolean",
					"description": "Whether to run the command in the background. Output is captured until timeout, then the command continues running in the background.",
				},
			},
			Required: []string{"workspace", "command"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args WorkspaceBashArgs) (res WorkspaceBashResult, err error) {
		if args.Workspace == "" {
			return WorkspaceBashResult{}, xerrors.New("workspace name cannot be empty")
		}
		if args.Command == "" {
			return WorkspaceBashResult{}, xerrors.New("command cannot be empty")
		}

		ctx, cancel := context.WithTimeoutCause(ctx, 5*time.Minute, xerrors.New("MCP handler timeout after 5 min"))
		defer cancel()

		conn, err := newAgentConn(ctx, deps.coderClient, args.Workspace)
		if err != nil {
			return WorkspaceBashResult{}, err
		}
		defer conn.Close()

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

		// Set default timeout if not specified (60 seconds)
		timeoutMs := args.TimeoutMs
		defaultTimeoutMs := 60000
		if timeoutMs <= 0 {
			timeoutMs = defaultTimeoutMs
		}
		command := args.Command
		if args.Background {
			// For background commands, use nohup directly to ensure they survive SSH session
			// termination. This captures output normally but allows the process to continue
			// running even after the SSH connection closes.
			command = fmt.Sprintf("nohup %s </dev/null 2>&1", args.Command)
		}

		// Create context with command timeout (replace the broader MCP timeout)
		commandCtx, commandCancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer commandCancel()

		// Execute command with timeout handling
		output, err := executeCommandWithTimeout(commandCtx, session, command)
		outputStr := strings.TrimSpace(string(output))

		// Handle command execution results
		if err != nil {
			// Check if the command timed out
			if errors.Is(context.Cause(commandCtx), context.DeadlineExceeded) {
				if args.Background {
					outputStr += "\nCommand continues running in background"
				} else {
					outputStr += "\nCommand canceled due to timeout"
				}
				return WorkspaceBashResult{
					Output:   outputStr,
					ExitCode: 124,
				}, nil
			}

			// Extract exit code from SSH error if available
			exitCode := 1
			var exitErr *gossh.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitStatus()
			}

			// For other errors, use standard timeout or generic error code
			return WorkspaceBashResult{
				Output:   outputStr,
				ExitCode: exitCode,
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

func splitNameAndOwner(identifier string) (name string, owner string) {
	// Parse owner and name (workspace, task).
	parts := strings.SplitN(identifier, "/", 2)

	if len(parts) == 2 {
		owner = parts[0]
		name = parts[1]
	} else {
		owner = "me"
		name = identifier
	}

	return name, owner
}

// namedWorkspace gets a workspace by owner/name or just name
func namedWorkspace(ctx context.Context, client *codersdk.Client, identifier string) (codersdk.Workspace, error) {
	workspaceName, owner := splitNameAndOwner(identifier)

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

// executeCommandWithTimeout executes a command with timeout support
func executeCommandWithTimeout(ctx context.Context, session *gossh.Session, command string) ([]byte, error) {
	// Set up pipes to capture output
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return nil, xerrors.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := session.StderrPipe()
	if err != nil {
		return nil, xerrors.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := session.Start(command); err != nil {
		return nil, xerrors.Errorf("failed to start command: %w", err)
	}

	// Create a thread-safe buffer for combined output
	var output bytes.Buffer
	var mu sync.Mutex
	safeWriter := &syncWriter{w: &output, mu: &mu}

	// Use io.MultiWriter to combine stdout and stderr
	multiWriter := io.MultiWriter(safeWriter)

	// Channel to signal when command completes
	done := make(chan error, 1)

	// Start goroutine to copy output and wait for completion
	go func() {
		// Copy stdout and stderr concurrently
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			_, _ = io.Copy(multiWriter, stdoutPipe)
		}()

		go func() {
			defer wg.Done()
			_, _ = io.Copy(multiWriter, stderrPipe)
		}()

		// Wait for all output to be copied
		wg.Wait()

		// Wait for the command to complete
		done <- session.Wait()
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-done:
		// Command completed normally
		return safeWriter.Bytes(), err
	case <-ctx.Done():
		// Context was canceled (timeout or other cancellation)
		// Close the session to stop the command, but handle errors gracefully
		closeErr := session.Close()

		// Give a brief moment to collect any remaining output and for goroutines to finish
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()

		select {
		case <-timer.C:
			// Timer expired, return what we have
			break
		case err := <-done:
			// Command finished during grace period
			if closeErr == nil {
				return safeWriter.Bytes(), err
			}
			// If session close failed, prioritize the context error
			break
		}

		// Return the collected output with the context error
		return safeWriter.Bytes(), context.Cause(ctx)
	}
}

// syncWriter is a thread-safe writer
type syncWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (sw *syncWriter) Write(p []byte) (n int, err error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

func (sw *syncWriter) Bytes() []byte {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	// Return a copy to prevent race conditions with the underlying buffer
	b := sw.w.Bytes()
	result := make([]byte, len(b))
	copy(result, b)
	return result
}
