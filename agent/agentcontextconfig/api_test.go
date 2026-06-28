package agentcontextconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontextconfig"
)

func TestNewAPI_LazyDirectory(t *testing.T) {
	t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
	t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
	t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

	dir := ""
	api := agentcontextconfig.NewAPI(func() string { return dir }, agentcontextconfig.ReadEnvConfig())

	// Before directory is set, MCP paths resolve to nothing.
	mcpFiles := api.MCPConfigFiles()
	require.Empty(t, mcpFiles)

	// After setting the directory, MCPConfigFiles() picks it up.
	dir = platformAbsPath("work")
	mcpFiles = api.MCPConfigFiles()
	require.NotEmpty(t, mcpFiles)
	require.Equal(t, []string{filepath.Join(dir, ".mcp.json")}, mcpFiles)
}

// TestClearEnvVars verifies that ClearEnvVars removes every
// CODER_AGENT_EXP_* env var from the process, including the
// ignored instruction/skill filename overrides.
//
//nolint:paralleltest // Mutates process-wide environment.
func TestClearEnvVars(t *testing.T) {
	keys := []string{
		agentcontextconfig.EnvInstructionsDirs,
		agentcontextconfig.EnvInstructionsFile,
		agentcontextconfig.EnvSkillsDirs,
		agentcontextconfig.EnvSkillMetaFile,
		agentcontextconfig.EnvMCPConfigFiles,
	}
	for _, key := range keys {
		t.Setenv(key, "some-value")
	}

	agentcontextconfig.ClearEnvVars()

	for _, key := range keys {
		_, ok := os.LookupEnv(key)
		require.False(t, ok, "env var %s should be cleared", key)
	}
}
