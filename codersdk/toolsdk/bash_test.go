package toolsdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

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

		deps := toolsdk.Deps{} // Empty deps will cause client access to fail
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
