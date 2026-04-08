package agentcontextconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/codersdk"
)

// filterParts returns only the parts matching the given type.
func filterParts(parts []codersdk.ChatMessagePart, t codersdk.ChatMessagePartType) []codersdk.ChatMessagePart {
	var out []codersdk.ChatMessagePart
	for _, p := range parts {
		if p.Type == t {
			out = append(out, p)
		}
	}
	return out
}

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
		cfg, mcpFiles := agentcontextconfig.Config(workDir)

		// Parts is always non-nil.
		require.NotNil(t, cfg.Parts)
		// Default MCP config file is ".mcp.json" (relative),
		// resolved against the working directory.
		require.Equal(t, []string{filepath.Join(workDir, ".mcp.json")}, mcpFiles)
	})

	t.Run("CustomEnvVars", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)

		optInstructions := t.TempDir()
		optSkills := t.TempDir()
		optMCP := platformAbsPath("opt", "mcp.json")

		t.Setenv(agentcontextconfig.EnvInstructionsDirs, optInstructions)
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "CUSTOM.md")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, optSkills)
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "META.yaml")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, optMCP)

		// Create files matching the custom names so we can
		// verify the env vars actually change lookup behavior.
		require.NoError(t, os.WriteFile(filepath.Join(optInstructions, "CUSTOM.md"), []byte("custom instructions"), 0o600))
		skillDir := filepath.Join(optSkills, "my-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(skillDir, "META.yaml"),
			[]byte("---\nname: my-skill\ndescription: custom meta\n---\n"),
			0o600,
		))

		workDir := platformAbsPath("work")
		cfg, mcpFiles := agentcontextconfig.Config(workDir)

		require.Equal(t, []string{optMCP}, mcpFiles)
		ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
		require.Len(t, ctxFiles, 1)
		require.Equal(t, "custom instructions", ctxFiles[0].ContextFileContent)
		skillParts := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeSkill)
		require.Len(t, skillParts, 1)
		require.Equal(t, "my-skill", skillParts[0].SkillName)
		require.Equal(t, "META.yaml", skillParts[0].ContextFileSkillMetaFile)
	})

	t.Run("WhitespaceInFileNames", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "  CLAUDE.md  ")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := t.TempDir()
		// Create a file matching the trimmed name.
		require.NoError(t, os.WriteFile(filepath.Join(fakeHome, "CLAUDE.md"), []byte("hello"), 0o600))

		cfg, _ := agentcontextconfig.Config(workDir)

		ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
		require.Len(t, ctxFiles, 1)
		require.Equal(t, "hello", ctxFiles[0].ContextFileContent)
	})

	t.Run("CommaSeparatedDirs", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)

		a := t.TempDir()
		b := t.TempDir()

		t.Setenv(agentcontextconfig.EnvInstructionsDirs, a+","+b)
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		// Put instruction files in both dirs.
		require.NoError(t, os.WriteFile(filepath.Join(a, "AGENTS.md"), []byte("from a"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(b, "AGENTS.md"), []byte("from b"), 0o600))

		workDir := t.TempDir()
		cfg, _ := agentcontextconfig.Config(workDir)

		ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
		require.Len(t, ctxFiles, 2)
		require.Equal(t, "from a", ctxFiles[0].ContextFileContent)
		require.Equal(t, "from b", ctxFiles[1].ContextFileContent)
	})

	t.Run("ReadsInstructionFiles", func(t *testing.T) {
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := t.TempDir()
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)

		// Create ~/.coder/AGENTS.md
		coderDir := filepath.Join(fakeHome, ".coder")
		require.NoError(t, os.MkdirAll(coderDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(coderDir, "AGENTS.md"),
			[]byte("home instructions"),
			0o600,
		))

		cfg, _ := agentcontextconfig.Config(workDir)

		ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
		require.NotNil(t, cfg.Parts)
		require.Len(t, ctxFiles, 1)
		require.Equal(t, "home instructions", ctxFiles[0].ContextFileContent)
		require.Equal(t, filepath.Join(coderDir, "AGENTS.md"), ctxFiles[0].ContextFilePath)
		require.False(t, ctxFiles[0].ContextFileTruncated)
	})

	t.Run("ReadsWorkingDirInstructionFile", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := t.TempDir()

		// Create AGENTS.md in the working directory.
		require.NoError(t, os.WriteFile(
			filepath.Join(workDir, "AGENTS.md"),
			[]byte("project instructions"),
			0o600,
		))

		cfg, _ := agentcontextconfig.Config(workDir)

		// Should find the working dir file (not in instruction dirs).
		ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
		require.NotNil(t, cfg.Parts)
		require.Len(t, ctxFiles, 1)
		require.Equal(t, "project instructions", ctxFiles[0].ContextFileContent)
		require.Equal(t, filepath.Join(workDir, "AGENTS.md"), ctxFiles[0].ContextFilePath)
	})

	t.Run("TruncatesLargeInstructionFile", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := t.TempDir()
		largeContent := strings.Repeat("a", 64*1024+100)
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte(largeContent), 0o600))

		cfg, _ := agentcontextconfig.Config(workDir)

		ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
		require.Len(t, ctxFiles, 1)
		require.True(t, ctxFiles[0].ContextFileTruncated)
		require.Len(t, ctxFiles[0].ContextFileContent, 64*1024)
	})

	t.Run("SanitizesHTMLComments", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(workDir, "AGENTS.md"),
			[]byte("visible\n<!-- hidden -->content"),
			0o600,
		))

		cfg, _ := agentcontextconfig.Config(workDir)

		ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
		require.Len(t, ctxFiles, 1)
		require.Equal(t, "visible\ncontent", ctxFiles[0].ContextFileContent)
	})

	t.Run("SanitizesInvisibleUnicode", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := t.TempDir()
		// U+200B (zero-width space) should be stripped.
		require.NoError(t, os.WriteFile(
			filepath.Join(workDir, "AGENTS.md"),
			[]byte("before\u200bafter"),
			0o600,
		))

		cfg, _ := agentcontextconfig.Config(workDir)

		ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
		require.Len(t, ctxFiles, 1)
		require.Equal(t, "beforeafter", ctxFiles[0].ContextFileContent)
	})

	t.Run("NormalizesCRLF", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(workDir, "AGENTS.md"),
			[]byte("line1\r\nline2\rline3"),
			0o600,
		))

		cfg, _ := agentcontextconfig.Config(workDir)

		ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
		require.Len(t, ctxFiles, 1)
		require.Equal(t, "line1\nline2\nline3", ctxFiles[0].ContextFileContent)
	})

	t.Run("DiscoversSkills", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := t.TempDir()
		skillsDir := filepath.Join(workDir, ".agents", "skills")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, skillsDir)

		// Create a valid skill.
		skillDir := filepath.Join(skillsDir, "my-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(skillDir, "SKILL.md"),
			[]byte("---\nname: my-skill\ndescription: A test skill\n---\nSkill body"),
			0o600,
		))

		cfg, _ := agentcontextconfig.Config(workDir)

		skillParts := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeSkill)
		require.Len(t, skillParts, 1)
		require.Equal(t, "my-skill", skillParts[0].SkillName)
		require.Equal(t, "A test skill", skillParts[0].SkillDescription)
		require.Equal(t, skillDir, skillParts[0].SkillDir)
		require.Equal(t, "SKILL.md", skillParts[0].ContextFileSkillMetaFile)
	})

	t.Run("SkipsMissingDirs", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)

		nonExistent := filepath.Join(t.TempDir(), "does-not-exist")
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, nonExistent)
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, nonExistent)
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := t.TempDir()
		cfg, _ := agentcontextconfig.Config(workDir)

		// Non-nil empty slice (signals agent supports new format).
		require.NotNil(t, cfg.Parts)
		require.Empty(t, cfg.Parts)
	})

	t.Run("MCPConfigFilesResolvedSeparately", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")

		optMCP := platformAbsPath("opt", "custom.json")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, optMCP)

		workDir := t.TempDir()
		_, mcpFiles := agentcontextconfig.Config(workDir)

		require.Equal(t, []string{optMCP}, mcpFiles)
	})

	t.Run("SkillNameMustMatchDir", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := t.TempDir()
		skillsDir := filepath.Join(workDir, "skills")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, skillsDir)

		// Skill name in frontmatter doesn't match directory name.
		skillDir := filepath.Join(skillsDir, "wrong-dir-name")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(skillDir, "SKILL.md"),
			[]byte("---\nname: actual-name\ndescription: mismatch\n---\n"),
			0o600,
		))

		cfg, _ := agentcontextconfig.Config(workDir)
		skillParts := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeSkill)
		require.Empty(t, skillParts)
	})

	t.Run("DuplicateSkillsFirstWins", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, fakeHome)
		t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
		t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
		t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

		workDir := t.TempDir()
		skillsDir1 := filepath.Join(workDir, "skills1")
		skillsDir2 := filepath.Join(workDir, "skills2")
		t.Setenv(agentcontextconfig.EnvSkillsDirs, skillsDir1+","+skillsDir2)

		// Same skill name in both directories.
		for _, dir := range []string{skillsDir1, skillsDir2} {
			skillDir := filepath.Join(dir, "dup-skill")
			require.NoError(t, os.MkdirAll(skillDir, 0o755))
			require.NoError(t, os.WriteFile(
				filepath.Join(skillDir, "SKILL.md"),
				[]byte("---\nname: dup-skill\ndescription: from "+filepath.Base(dir)+"\n---\n"),
				0o600,
			))
		}

		cfg, _ := agentcontextconfig.Config(workDir)
		skillParts := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeSkill)
		require.Len(t, skillParts, 1)
		require.Equal(t, "from skills1", skillParts[0].SkillDescription)
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

	// Before directory is set, MCP paths resolve to nothing.
	mcpFiles := api.MCPConfigFiles()
	require.Empty(t, mcpFiles)

	// After setting the directory, MCPConfigFiles() picks it up.
	dir = platformAbsPath("work")
	mcpFiles = api.MCPConfigFiles()
	require.NotEmpty(t, mcpFiles)
	require.Equal(t, []string{filepath.Join(dir, ".mcp.json")}, mcpFiles)
}
