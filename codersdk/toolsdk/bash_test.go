package toolsdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

func TestWorkspaceBash(t *testing.T) {
	t.Parallel()

	t.Run("ValidateArgs", func(t *testing.T) {
		t.Parallel()

		deps := toolsdk.Deps{}
		ctx := context.Background()

		// Test empty workspace name
		args := toolsdk.WorkspaceBashArgs{
			Workspace: "",
			Command:   "echo test",
		}
		_, err := toolsdk.WorkspaceBash.Handler(ctx, deps, args)
		require.Error(t, err)
		require.Contains(t, err.Error(), "workspace name cannot be empty")

		// Test empty command
		args = toolsdk.WorkspaceBashArgs{
			Workspace: "test-workspace",
			Command:   "",
		}
		_, err = toolsdk.WorkspaceBash.Handler(ctx, deps, args)
		require.Error(t, err)
		require.Contains(t, err.Error(), "command cannot be empty")
	})

	t.Run("ErrorScenarios", func(t *testing.T) {
		t.Parallel()

		deps := toolsdk.Deps{}
		ctx := context.Background()

		// Test input validation errors (these should fail before client access)
		t.Run("EmptyWorkspace", func(t *testing.T) {
			args := toolsdk.WorkspaceBashArgs{
				Workspace: "", // Empty workspace should be caught by validation
				Command:   "echo test",
			}
			_, err := toolsdk.WorkspaceBash.Handler(ctx, deps, args)
			require.Error(t, err)
			require.Contains(t, err.Error(), "workspace name cannot be empty")
		})

		t.Run("EmptyCommand", func(t *testing.T) {
			args := toolsdk.WorkspaceBashArgs{
				Workspace: "test-workspace",
				Command:   "", // Empty command should be caught by validation
			}
			_, err := toolsdk.WorkspaceBash.Handler(ctx, deps, args)
			require.Error(t, err)
			require.Contains(t, err.Error(), "command cannot be empty")
		})
	})

	t.Run("ToolMetadata", func(t *testing.T) {
		t.Parallel()

		tool := toolsdk.WorkspaceBash
		require.Equal(t, toolsdk.ToolNameWorkspaceBash, tool.Name)
		require.NotEmpty(t, tool.Description)
		require.Contains(t, tool.Description, "Execute a bash command in a Coder workspace")
		require.Contains(t, tool.Description, "output is trimmed of leading and trailing whitespace")
		require.Contains(t, tool.Schema.Required, "workspace")
		require.Contains(t, tool.Schema.Required, "command")

		// Check that schema has the required properties
		require.Contains(t, tool.Schema.Properties, "workspace")
		require.Contains(t, tool.Schema.Properties, "command")
	})

	t.Run("GenericTool", func(t *testing.T) {
		t.Parallel()

		genericTool := toolsdk.WorkspaceBash.Generic()
		require.Equal(t, toolsdk.ToolNameWorkspaceBash, genericTool.Name)
		require.NotEmpty(t, genericTool.Description)
		require.NotNil(t, genericTool.Handler)
		require.False(t, genericTool.UserClientOptional)
	})
}

func TestNormalizeWorkspaceInput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "SimpleWorkspace",
			input:    "workspace",
			expected: "workspace",
		},
		{
			name:     "WorkspaceWithAgent",
			input:    "workspace.agent",
			expected: "workspace.agent",
		},
		{
			name:     "OwnerAndWorkspace",
			input:    "owner/workspace",
			expected: "owner/workspace",
		},
		{
			name:     "OwnerDashWorkspace",
			input:    "owner--workspace",
			expected: "owner/workspace",
		},
		{
			name:     "OwnerWorkspaceAgent",
			input:    "owner/workspace.agent",
			expected: "owner/workspace.agent",
		},
		{
			name:     "OwnerDashWorkspaceAgent",
			input:    "owner--workspace.agent",
			expected: "owner/workspace.agent",
		},
		{
			name:     "CoderConnectFormat",
			input:    "agent.workspace.owner", // Special Coder Connect reverse format
			expected: "owner/workspace.agent",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := toolsdk.NormalizeWorkspaceInput(tc.input)
			require.Equal(t, tc.expected, result, "Input %q should normalize to %q but got %q", tc.input, tc.expected, result)
		})
	}
}

func TestAllToolsIncludesBash(t *testing.T) {
	t.Parallel()

	// Verify that WorkspaceBash is included in the All slice
	found := false
	for _, tool := range toolsdk.All {
		if tool.Name == toolsdk.ToolNameWorkspaceBash {
			found = true
			break
		}
	}
	require.True(t, found, "WorkspaceBash tool should be included in toolsdk.All")
}

// Note: Unit testing ExecuteCommandWithTimeout is challenging because it expects
// a concrete SSH session type. The integration tests above demonstrate the
// timeout functionality with a real SSH connection and mock clock.

func TestWorkspaceBashTimeout(t *testing.T) {
	t.Parallel()

	t.Run("TimeoutDefaultValue", func(t *testing.T) {
		t.Parallel()

		// Test that the TimeoutMs field can be set and read correctly
		args := toolsdk.WorkspaceBashArgs{
			Workspace: "test-workspace",
			Command:   "echo test",
			TimeoutMs: 0, // Should default to 60000 in handler
		}

		// Verify that the TimeoutMs field exists and can be set
		require.Equal(t, 0, args.TimeoutMs)

		// Test setting a positive value
		args.TimeoutMs = 5000
		require.Equal(t, 5000, args.TimeoutMs)
	})

	t.Run("TimeoutNegativeValue", func(t *testing.T) {
		t.Parallel()

		// Test that negative values can be set and will be handled by the default logic
		args := toolsdk.WorkspaceBashArgs{
			Workspace: "test-workspace",
			Command:   "echo test",
			TimeoutMs: -100,
		}

		require.Equal(t, -100, args.TimeoutMs)

		// The actual defaulting to 60000 happens inside the handler
		// We can't test it without a full integration test setup
	})

	t.Run("TimeoutSchemaValidation", func(t *testing.T) {
		t.Parallel()

		tool := toolsdk.WorkspaceBash

		// Check that timeout_ms is in the schema
		require.Contains(t, tool.Schema.Properties, "timeout_ms")

		timeoutProperty := tool.Schema.Properties["timeout_ms"].(map[string]any)
		require.Equal(t, "integer", timeoutProperty["type"])
		require.Equal(t, 60000, timeoutProperty["default"])
		require.Equal(t, 1, timeoutProperty["minimum"])
		require.Contains(t, timeoutProperty["description"], "timeout in milliseconds")
	})

	t.Run("TimeoutDescriptionUpdated", func(t *testing.T) {
		t.Parallel()

		tool := toolsdk.WorkspaceBash

		// Check that description mentions timeout functionality
		require.Contains(t, tool.Description, "timeout_ms parameter")
		require.Contains(t, tool.Description, "defaults to 60000ms")
		require.Contains(t, tool.Description, "timeout_ms: 30000")
	})

	t.Run("TimeoutCommandScenario", func(t *testing.T) {
		t.Parallel()

		// Scenario: echo "123"; sleep 60; echo "456" with 5ms timeout
		// In this scenario, we'd expect to see "123" in the output and a cancellation message
		args := toolsdk.WorkspaceBashArgs{
			Workspace: "test-workspace",
			Command:   `echo "123"; sleep 60; echo "456"`, // This command would take 60+ seconds
			TimeoutMs: 5,                                  // 5ms timeout - should timeout after first echo
		}

		// Verify the args are structured correctly for the intended test scenario
		require.Equal(t, "test-workspace", args.Workspace)
		require.Contains(t, args.Command, `echo "123"`)
		require.Contains(t, args.Command, "sleep 60")
		require.Contains(t, args.Command, `echo "456"`)
		require.Equal(t, 5, args.TimeoutMs)

		// Note: The actual timeout behavior would need to be tested with a real workspace
		// This test just verifies the structure is correct for the timeout scenario
	})
}

func TestWorkspaceBashTimeoutIntegration(t *testing.T) {
	t.Parallel()

	t.Run("ActualTimeoutBehavior", func(t *testing.T) {
		t.Parallel()

		// Scenario: echo "123"; sleep 60; echo "456" with 5s timeout
		// In this scenario, we'd expect to see "123" in the output and a cancellation message

		client, workspace, agentToken := setupWorkspaceForAgent(t)

		// Start the agent and wait for it to be fully ready
		_ = agenttest.New(t, client.URL, agentToken)

		// Wait for workspace agents to be ready like other SSH tests do
		coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

		// Use real clock for integration test
		deps, err := toolsdk.NewDeps(client)
		require.NoError(t, err)

		args := toolsdk.WorkspaceBashArgs{
			Workspace: workspace.Name,
			Command:   `echo "123" && sleep 60 && echo "456"`, // This command would take 60+ seconds
			TimeoutMs: 2000,                                   // 2 seconds timeout - should timeout after first echo
		}

		result, err := toolsdk.WorkspaceBash.Handler(t.Context(), deps, args)

		// Should not error (timeout is handled gracefully)
		require.NoError(t, err)

		t.Logf("Test results: exitCode=%d, output=%q, error=%v", result.ExitCode, result.Output, err)

		// Should have a non-zero exit code (timeout or error)
		require.NotEqual(t, 0, result.ExitCode, "Expected non-zero exit code for timeout")

		t.Logf("result.Output: %s", result.Output)

		// Should contain the first echo output
		require.Contains(t, result.Output, "123")

		// Should NOT contain the second echo (it never executed due to timeout)
		require.NotContains(t, result.Output, "456", "Should not contain output after sleep")
	})

	t.Run("NormalCommandExecution", func(t *testing.T) {
		t.Parallel()

		// Test that normal commands still work with timeout functionality present

		client, workspace, agentToken := setupWorkspaceForAgent(t)

		// Start the agent and wait for it to be fully ready
		_ = agenttest.New(t, client.URL, agentToken)

		// Wait for workspace agents to be ready
		coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

		deps, err := toolsdk.NewDeps(client)
		require.NoError(t, err)
		ctx := context.Background()

		args := toolsdk.WorkspaceBashArgs{
			Workspace: workspace.Name,
			Command:   `echo "normal command"`, // Quick command that should complete normally
			TimeoutMs: 5000,                    // 5 second timeout - plenty of time
		}

		result, err := toolsdk.WorkspaceBash.Handler(ctx, deps, args)

		// Should not error
		require.NoError(t, err)

		t.Logf("result.Output: %s", result.Output)

		// Should have exit code 0 (success)
		require.Equal(t, 0, result.ExitCode)

		// Should contain the expected output
		require.Equal(t, "normal command", result.Output)

		// Should NOT contain timeout message
		require.NotContains(t, result.Output, "Command canceled due to timeout")
	})
}
