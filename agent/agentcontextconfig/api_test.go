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
		t.Setenv("USERPROFILE", fakeHome)

		// Clear all env vars so defaults are used.
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := platformAbsPath("work")
		cfg := agentcontextconfig.Config(workDir)

		require.Equal(t, workspacesdk.DefaultInstructionsFile, cfg.InstructionsFile)
		require.Equal(t, workspacesdk.DefaultSkillMetaFile, cfg.SkillMetaFile)
		// Default instructions dir is "~/.coder" which resolves
		// to the home directory.
		require.Equal(t, []string{filepath.Join(fakeHome, ".coder")}, cfg.InstructionsDirs)
		// Default skills dir is ".agents/skills" (relative),
		// resolved against the working directory.
		require.Equal(t, []string{filepath.Join(workDir, ".agents", "skills")}, cfg.SkillsDirs)
		// Default MCP config file is ".mcp.json" (relative),
		// resolved against the working directory.
		require.Equal(t, []string{filepath.Join(workDir, ".mcp.json")}, cfg.MCPConfigFiles)
	})

	t.Run("CustomEnvVars", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)

		optInstructions := platformAbsPath("opt", "instructions")
		optSkills := platformAbsPath("opt", "skills")
		optMCP := platformAbsPath("opt", "mcp.json")

		t.Setenv(agentcontextconfig.EnvInstructionsDirs, optInstructions)
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "CUSTOM.md")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, optSkills)
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "META.yaml")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, optMCP)

		workDir := platformAbsPath("work")
		cfg := agentcontextconfig.Config(workDir)

		require.Equal(t, "CUSTOM.md", cfg.InstructionsFile)
		require.Equal(t, "META.yaml", cfg.SkillMetaFile)
		require.Equal(t, []string{optInstructions}, cfg.InstructionsDirs)
		require.Equal(t, []string{optSkills}, cfg.SkillsDirs)
		require.Equal(t, []string{optMCP}, cfg.MCPConfigFiles)
	})

	t.Run("WhitespaceInFileNames", func(t *testing.T) {
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "  CLAUDE.md  ")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := platformAbsPath("work")
		cfg := agentcontextconfig.Config(workDir)

		require.Equal(t, "CLAUDE.md", cfg.InstructionsFile)
	})

	t.Run("CommaSeparatedDirs", func(t *testing.T) {
		a := platformAbsPath("opt", "a")
		b := platformAbsPath("opt", "b")

		t.Setenv(agentcontextconfig.EnvInstructionsDirs, a+","+b)
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := platformAbsPath("work")
		cfg := agentcontextconfig.Config(workDir)

		require.Equal(t, []string{a, b}, cfg.InstructionsDirs)
	})
}

func TestNewAPI_LazyDirectory(t *testing.T) {
	t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
	t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
	t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
	t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
	t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

	dir := ""
	api := agentcontextconfig.NewAPI(func() string { return dir })

	// Before directory is set, relative paths resolve to nothing.
	cfg := api.Config()
	require.Empty(t, cfg.SkillsDirs)
	require.Empty(t, cfg.MCPConfigFiles)

	// After setting the directory, Config() picks it up lazily.
	dir = platformAbsPath("work")
	cfg = api.Config()
	require.NotEmpty(t, cfg.SkillsDirs)
	require.Equal(t, []string{filepath.Join(dir, ".agents", "skills")}, cfg.SkillsDirs)
}
