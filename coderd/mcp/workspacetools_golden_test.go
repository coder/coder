package mcp_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

var updateGoldenFiles = flag.Bool("update", false, "update golden files")

// workspaceToolContract is the part of a workspace tool's model-facing
// definition that Coder Agents and MCP share.
type workspaceToolContract struct {
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
	Required    []string       `json:"required"`
}

// TestWorkspaceToolsGolden pins the MCP workspace tools and the Coder
// Agents tools they mirror to one shared contract per tool, stored in
// testdata/workspace_tools so contract changes show up in review. Run
// with -update to regenerate the files from the Coder Agents tools.
//
// The MCP versions differ only by the coder_workspace_ name prefix and
// an extra workspace argument. Coder Agents additionally exposes
// plan-turn guidance on write_file.
func TestWorkspaceToolsGolden(t *testing.T) {
	t.Parallel()

	mcpTools := make(map[string]toolsdk.GenericTool, len(toolsdk.All))
	for _, tool := range toolsdk.All {
		mcpTools[tool.Name] = tool
	}

	// MCP text must reference MCP tool names. A bare Coder Agents name
	// would point the model at a tool that does not exist on MCP. The
	// word boundaries skip the coder_workspace_ prefixed names.
	bareAgentsName := regexp.MustCompile(`\b(process_output|process_list|process_signal|read_file|write_file|edit_files)\b`)

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
		// ignoreArgDescriptions lists arguments whose descriptions
		// legitimately differ between surfaces.
		ignoreArgDescriptions []string
		// descriptionPrefix leaves the tool description out of the
		// contract and instead requires the MCP description to be a
		// prefix of the Coder Agents description.
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
		{mcpName: toolsdk.ToolNameWorkspaceExecute, agents: chattool.Execute(chattool.ExecuteOptions{})},
		{mcpName: toolsdk.ToolNameWorkspaceProcessOutput, agents: chattool.ProcessOutput(chattool.ProcessToolOptions{})},
		{mcpName: toolsdk.ToolNameWorkspaceProcessList, agents: chattool.ProcessList(chattool.ProcessToolOptions{})},
		{mcpName: toolsdk.ToolNameWorkspaceProcessSignal, agents: chattool.ProcessSignal(chattool.ProcessToolOptions{})},
	}

	for _, tt := range tests {
		t.Run(tt.mcpName, func(t *testing.T) {
			t.Parallel()

			mcpTool, ok := mcpTools[tt.mcpName]
			require.True(t, ok, "tool %q is not registered in toolsdk.All", tt.mcpName)
			info := tt.agents.Info()

			mcpSchema, err := json.Marshal(mcpTool.Schema.Properties)
			require.NoError(t, err)
			require.Empty(t, bareAgentsName.FindAllString(mcpTool.Description+string(mcpSchema), -1),
				"MCP tool text references Coder Agents tool names")
			require.Contains(t, mcpTool.Schema.Properties, "workspace", "MCP workspace tools must select a workspace")
			require.Contains(t, mcpTool.Schema.Required, "workspace")

			mcpDescription := renamer.Replace(mcpTool.Description)
			agentsDescription := info.Description
			if tt.descriptionPrefix {
				require.True(t, strings.HasPrefix(agentsDescription, strings.TrimSuffix(mcpDescription, ".")),
					"MCP description %q is not a prefix of %q", mcpDescription, agentsDescription)
				mcpDescription, agentsDescription = "", ""
			}

			omit := func(names ...string) func(string) bool {
				return func(name string) bool { return slices.Contains(names, name) }
			}
			agents := newWorkspaceToolContract(t, renamer, agentsDescription,
				info.Parameters, info.Required, omit(), tt.ignoreArgDescriptions)
			mcp := newWorkspaceToolContract(t, renamer, mcpDescription,
				mcpTool.Schema.Properties, mcpTool.Schema.Required, omit("workspace"), tt.ignoreArgDescriptions)

			goldenPath := filepath.Join("testdata", "workspace_tools", info.Name+".json.golden")
			if *updateGoldenFiles {
				require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
				require.NoError(t, os.WriteFile(goldenPath, agents, 0o600))
			}
			golden, err := os.ReadFile(goldenPath)
			require.NoError(t, err, "golden file %s not found; run with -update to create it", goldenPath)

			require.Empty(t, cmp.Diff(string(golden), string(agents)),
				"Coder Agents %s does not match %s (-golden +actual); run with -update if the change is intended", info.Name, goldenPath)
			require.Empty(t, cmp.Diff(string(golden), string(mcp)),
				"MCP %s does not match %s (-golden +actual); update the MCP tool to match", tt.mcpName, goldenPath)
		})
	}
}

// newWorkspaceToolContract renders a tool's shared contract as indented
// JSON. It maps tool names to their Coder Agents form, drops omitted
// arguments, and clears descriptions that differ between surfaces.
func newWorkspaceToolContract(
	t *testing.T,
	renamer *strings.Replacer,
	description string,
	parameters map[string]any,
	required []string,
	omit func(string) bool,
	ignoreArgDescriptions []string,
) []byte {
	t.Helper()

	// Round-trip through JSON so typed slices and maps compare equal.
	data, err := json.Marshal(parameters)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(renamer.Replace(string(data))), &params))

	for name := range params {
		if omit(name) {
			delete(params, name)
		}
	}
	for _, name := range ignoreArgDescriptions {
		arg, ok := params[name].(map[string]any)
		require.True(t, ok, "argument %q is missing", name)
		delete(arg, "description")
	}

	req := slices.DeleteFunc(append([]string{}, required...), omit)
	slices.Sort(req)

	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(workspaceToolContract{
		Description: description,
		Parameters:  params,
		Required:    req,
	}))
	return out.Bytes()
}
