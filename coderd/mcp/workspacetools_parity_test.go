package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

// TestWorkspaceToolsMatchCoderAgents pins the MCP workspace tools to the
// Coder Agents tools they mirror. The MCP versions differ only by the
// coder_workspace_ name prefix and an extra workspace argument; Coder
// Agents additionally exposes model_intent for its UI and plan-turn
// guidance on write_file.
func TestWorkspaceToolsMatchCoderAgents(t *testing.T) {
	t.Parallel()

	mcpTools := make(map[string]toolsdk.GenericTool, len(toolsdk.All))
	for _, tool := range toolsdk.All {
		mcpTools[tool.Name] = tool
	}

	// Replacing MCP names with Coder Agents names lets descriptions that
	// reference follow-up tools compare equal.
	renamer := strings.NewReplacer(
		toolsdk.ToolNameWorkspaceExecute, chattool.ExecuteToolName,
		toolsdk.ToolNameWorkspaceProcessOutput, "process_output",
		toolsdk.ToolNameWorkspaceProcessList, "process_list",
		toolsdk.ToolNameWorkspaceProcessSignal, "process_signal",
		toolsdk.ToolNameWorkspaceReadFile, "read_file",
		toolsdk.ToolNameWorkspaceWriteFile, chattool.WriteFileToolName,
		toolsdk.ToolNameWorkspaceEditFiles, chattool.EditFilesToolName,
	)

	tests := []struct {
		mcpName string
		agents  fantasy.AgentTool
		// agentsOnly lists Coder Agents arguments the MCP tool omits.
		agentsOnly []string
		// ignoreArgDescriptions lists arguments whose descriptions
		// legitimately differ between surfaces.
		ignoreArgDescriptions []string
		// descriptionPrefix compares only the MCP description as a prefix
		// of the Coder Agents description.
		descriptionPrefix bool
	}{
		{mcpName: toolsdk.ToolNameWorkspaceReadFile, agents: chattool.ReadFile(chattool.ReadFileOptions{})},
		{
			mcpName:               toolsdk.ToolNameWorkspaceWriteFile,
			agents:                chattool.WriteFile(chattool.WriteFileOptions{}),
			ignoreArgDescriptions: []string{"path"},
			descriptionPrefix:     true,
		},
		{mcpName: toolsdk.ToolNameWorkspaceEditFiles, agents: chattool.EditFiles(chattool.EditFilesOptions{})},
		{
			mcpName:    toolsdk.ToolNameWorkspaceExecute,
			agents:     chattool.Execute(chattool.ExecuteOptions{}),
			agentsOnly: []string{"model_intent"},
		},
		{
			mcpName:    toolsdk.ToolNameWorkspaceProcessOutput,
			agents:     chattool.ProcessOutput(chattool.ProcessToolOptions{}),
			agentsOnly: []string{"model_intent"},
		},
		{mcpName: toolsdk.ToolNameWorkspaceProcessList, agents: chattool.ProcessList(chattool.ProcessToolOptions{})},
		{mcpName: toolsdk.ToolNameWorkspaceProcessSignal, agents: chattool.ProcessSignal(chattool.ProcessToolOptions{})},
	}

	for _, tt := range tests {
		t.Run(tt.mcpName, func(t *testing.T) {
			t.Parallel()

			mcpTool, ok := mcpTools[tt.mcpName]
			require.True(t, ok, "tool %q is not registered in toolsdk.All", tt.mcpName)
			info := tt.agents.Info()

			mcpDescription := renamer.Replace(mcpTool.Description)
			if tt.descriptionPrefix {
				require.True(t, strings.HasPrefix(info.Description, strings.TrimSuffix(mcpDescription, ".")),
					"MCP description %q is not a prefix of %q", mcpDescription, info.Description)
			} else {
				require.Equal(t, info.Description, mcpDescription)
			}

			mcpArgs := normalizeSchema(t, renamer, mcpTool.Schema.Properties)
			agentsArgs := normalizeSchema(t, renamer, info.Parameters)
			require.Contains(t, mcpArgs, "workspace", "MCP workspace tools must select a workspace")
			delete(mcpArgs, "workspace")
			for _, name := range tt.agentsOnly {
				delete(agentsArgs, name)
			}
			for _, name := range tt.ignoreArgDescriptions {
				delete(mcpArgs[name].(map[string]any), "description")
				delete(agentsArgs[name].(map[string]any), "description")
			}
			require.Equal(t, agentsArgs, mcpArgs)

			mcpRequired := make([]string, 0, len(mcpTool.Schema.Required))
			for _, name := range mcpTool.Schema.Required {
				if name != "workspace" {
					mcpRequired = append(mcpRequired, name)
				}
			}
			require.ElementsMatch(t, info.Required, mcpRequired)
		})
	}
}

// normalizeSchema round-trips a schema through JSON so typed slices and
// maps compare equal, and maps tool names to their Coder Agents form.
func normalizeSchema(t *testing.T, renamer *strings.Replacer, schema map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(schema)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(renamer.Replace(string(data))), &out))
	return out
}
