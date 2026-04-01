package agentcontextconfig_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestConfig(t *testing.T) {
	t.Run("Defaults", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)

		// Clear all env vars so defaults are used.
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		cfg := agentcontextconfig.Config("/work")

		require.Equal(t, workspacesdk.DefaultInstructionsFile, cfg.InstructionsFile)
		require.Equal(t, workspacesdk.DefaultSkillMetaFile, cfg.SkillMetaFile)
		// Default instructions dir is "~/.coder" which resolves
		// to the home directory.
		require.Equal(t, []string{filepath.Join(fakeHome, ".coder")}, cfg.InstructionsDirs)
		// Default skills dir is ".agents/skills" (relative),
		// resolved against the working directory.
		require.Equal(t, []string{"/work/.agents/skills"}, cfg.SkillsDirs)
		// Default MCP config file is ".mcp.json" (relative),
		// resolved against the working directory.
		require.Equal(t, []string{"/work/.mcp.json"}, cfg.MCPConfigFiles)
	})

	t.Run("CustomEnvVars", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)

		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "/opt/instructions")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "CUSTOM.md")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "/opt/skills")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "META.yaml")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "/opt/mcp.json")

		cfg := agentcontextconfig.Config("/work")

		require.Equal(t, "CUSTOM.md", cfg.InstructionsFile)
		require.Equal(t, "META.yaml", cfg.SkillMetaFile)
		require.Equal(t, []string{"/opt/instructions"}, cfg.InstructionsDirs)
		require.Equal(t, []string{"/opt/skills"}, cfg.SkillsDirs)
		require.Equal(t, []string{"/opt/mcp.json"}, cfg.MCPConfigFiles)
	})

	t.Run("WhitespaceInFileNames", func(t *testing.T) {
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "  CLAUDE.md  ")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		cfg := agentcontextconfig.Config("/work")

		require.Equal(t, "CLAUDE.md", cfg.InstructionsFile)
	})

	t.Run("CommaSeparatedDirs", func(t *testing.T) {
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "/opt/a,/opt/b")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		cfg := agentcontextconfig.Config("/work")

		require.Equal(t, []string{"/opt/a", "/opt/b"}, cfg.InstructionsDirs)
	})
}
