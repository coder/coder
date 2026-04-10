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

func writeSkillMetaFileInRoot(t *testing.T, skillsRoot, name, description string) string {
	t.Helper()

	skillDir := filepath.Join(skillsRoot, name)
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: "+name+"\ndescription: "+description+"\n---\nSkill body"),
		0o600,
	))

	return skillDir
}

func writeSkillMetaFile(t *testing.T, dir, name, description string) string {
	t.Helper()
	return writeSkillMetaFileInRoot(t, filepath.Join(dir, ".agents", "skills"), name, description)
}

func TestContextPartsFromDir(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsInstructionFilePart", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		instructionPath := filepath.Join(dir, "AGENTS.md")
		require.NoError(t, os.WriteFile(instructionPath, []byte("project instructions"), 0o600))

		parts := agentcontextconfig.ContextPartsFromDir(dir)
		contextParts := filterParts(parts, codersdk.ChatMessagePartTypeContextFile)
		skillParts := filterParts(parts, codersdk.ChatMessagePartTypeSkill)

		require.Len(t, parts, 1)
		require.Len(t, contextParts, 1)
		require.Empty(t, skillParts)
		require.Equal(t, instructionPath, contextParts[0].ContextFilePath)
		require.Equal(t, "project instructions", contextParts[0].ContextFileContent)
		require.False(t, contextParts[0].ContextFileTruncated)
	})

	t.Run("ReturnsSkillParts", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		skillDir := writeSkillMetaFile(t, dir, "my-skill", "A test skill")

		parts := agentcontextconfig.ContextPartsFromDir(dir)
		contextParts := filterParts(parts, codersdk.ChatMessagePartTypeContextFile)
		skillParts := filterParts(parts, codersdk.ChatMessagePartTypeSkill)

		require.Len(t, parts, 1)
		require.Empty(t, contextParts)
		require.Len(t, skillParts, 1)
		require.Equal(t, "my-skill", skillParts[0].SkillName)
		require.Equal(t, "A test skill", skillParts[0].SkillDescription)
		require.Equal(t, skillDir, skillParts[0].SkillDir)
		require.Equal(t, "SKILL.md", skillParts[0].ContextFileSkillMetaFile)
	})

	t.Run("ReturnsSkillPartsFromSkillsDir", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		skillDir := writeSkillMetaFileInRoot(
			t,
			filepath.Join(dir, "skills"),
			"my-skill",
			"A test skill",
		)

		parts := agentcontextconfig.ContextPartsFromDir(dir)
		contextParts := filterParts(parts, codersdk.ChatMessagePartTypeContextFile)
		skillParts := filterParts(parts, codersdk.ChatMessagePartTypeSkill)

		require.Len(t, parts, 1)
		require.Empty(t, contextParts)
		require.Len(t, skillParts, 1)
		require.Equal(t, "my-skill", skillParts[0].SkillName)
		require.Equal(t, "A test skill", skillParts[0].SkillDescription)
		require.Equal(t, skillDir, skillParts[0].SkillDir)
		require.Equal(t, "SKILL.md", skillParts[0].ContextFileSkillMetaFile)
	})

	t.Run("ReturnsEmptyForEmptyDir", func(t *testing.T) {
		t.Parallel()

		parts := agentcontextconfig.ContextPartsFromDir(t.TempDir())

		require.NotNil(t, parts)
		require.Empty(t, parts)
	})

	t.Run("ReturnsCombinedResults", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		instructionPath := filepath.Join(dir, "AGENTS.md")
		require.NoError(t, os.WriteFile(instructionPath, []byte("combined instructions"), 0o600))
		skillDir := writeSkillMetaFile(t, dir, "combined-skill", "Combined test skill")

		parts := agentcontextconfig.ContextPartsFromDir(dir)
		contextParts := filterParts(parts, codersdk.ChatMessagePartTypeContextFile)
		skillParts := filterParts(parts, codersdk.ChatMessagePartTypeSkill)

		require.Len(t, parts, 2)
		require.Len(t, contextParts, 1)
		require.Len(t, skillParts, 1)
		require.Equal(t, instructionPath, contextParts[0].ContextFilePath)
		require.Equal(t, "combined instructions", contextParts[0].ContextFileContent)
		require.Equal(t, "combined-skill", skillParts[0].SkillName)
		require.Equal(t, skillDir, skillParts[0].SkillDir)
	})
}

func setupConfigTestEnv(t *testing.T, overrides map[string]string) string {
	t.Helper()

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	t.Setenv(agentcontextconfig.EnvInstructionsDirs, "")
	t.Setenv(agentcontextconfig.EnvInstructionsFile, "")
	t.Setenv(agentcontextconfig.EnvSkillsDirs, "")
	t.Setenv(agentcontextconfig.EnvSkillMetaFile, "")
	t.Setenv(agentcontextconfig.EnvMCPConfigFiles, "")

	for key, value := range overrides {
		t.Setenv(key, value)
	}

	return fakeHome
}

func TestConfig(t *testing.T) {
	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	t.Run("Defaults", func(t *testing.T) {
		setupConfigTestEnv(t, nil)

		workDir := platformAbsPath("work")
		cfg, mcpFiles := agentcontextconfig.Config(workDir)

		// Parts is always non-nil.
		require.NotNil(t, cfg.Parts)
		// Default MCP config file is ".mcp.json" (relative),
		// resolved against the working directory.
		require.Equal(t, []string{filepath.Join(workDir, ".mcp.json")}, mcpFiles)
	})

	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	t.Run("CustomEnvVars", func(t *testing.T) {
		optInstructions := t.TempDir()
		optSkills := t.TempDir()
		optMCP := platformAbsPath("opt", "mcp.json")
		setupConfigTestEnv(t, map[string]string{
			agentcontextconfig.EnvInstructionsDirs: optInstructions,
			agentcontextconfig.EnvInstructionsFile: "CUSTOM.md",
			agentcontextconfig.EnvSkillsDirs:       optSkills,
			agentcontextconfig.EnvSkillMetaFile:    "META.yaml",
			agentcontextconfig.EnvMCPConfigFiles:   optMCP,
		})

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

	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	t.Run("WhitespaceInFileNames", func(t *testing.T) {
		fakeHome := setupConfigTestEnv(t, map[string]string{
			agentcontextconfig.EnvInstructionsFile: "  CLAUDE.md  ",
		})
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, fakeHome)

		workDir := t.TempDir()
		// Create a file matching the trimmed name.
		require.NoError(t, os.WriteFile(filepath.Join(fakeHome, "CLAUDE.md"), []byte("hello"), 0o600))

		cfg, _ := agentcontextconfig.Config(workDir)

		ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
		require.Len(t, ctxFiles, 1)
		require.Equal(t, "hello", ctxFiles[0].ContextFileContent)
	})

	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	t.Run("CommaSeparatedDirs", func(t *testing.T) {
		a := t.TempDir()
		b := t.TempDir()
		setupConfigTestEnv(t, map[string]string{
			agentcontextconfig.EnvInstructionsDirs: a + "," + b,
		})

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

	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	t.Run("ReadsInstructionFiles", func(t *testing.T) {
		workDir := t.TempDir()
		fakeHome := setupConfigTestEnv(t, nil)

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

	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	t.Run("ReadsWorkingDirInstructionFile", func(t *testing.T) {
		setupConfigTestEnv(t, nil)
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

	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	t.Run("TruncatesLargeInstructionFile", func(t *testing.T) {
		setupConfigTestEnv(t, nil)
		workDir := t.TempDir()
		largeContent := strings.Repeat("a", 64*1024+100)
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte(largeContent), 0o600))

		cfg, _ := agentcontextconfig.Config(workDir)

		ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
		require.Len(t, ctxFiles, 1)
		require.True(t, ctxFiles[0].ContextFileTruncated)
		require.Len(t, ctxFiles[0].ContextFileContent, 64*1024)
	})

	sanitizationTests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "SanitizesHTMLComments",
			input:    "visible\n<!-- hidden -->content",
			expected: "visible\ncontent",
		},
		{
			name:     "SanitizesInvisibleUnicode",
			input:    "before\u200bafter",
			expected: "beforeafter",
		},
		{
			name:     "NormalizesCRLF",
			input:    "line1\r\nline2\rline3",
			expected: "line1\nline2\nline3",
		},
	}
	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	for _, tt := range sanitizationTests {
		t.Run(tt.name, func(t *testing.T) {
			setupConfigTestEnv(t, nil)
			workDir := t.TempDir()
			require.NoError(t, os.WriteFile(
				filepath.Join(workDir, "AGENTS.md"),
				[]byte(tt.input),
				0o600,
			))

			cfg, _ := agentcontextconfig.Config(workDir)

			ctxFiles := filterParts(cfg.Parts, codersdk.ChatMessagePartTypeContextFile)
			require.Len(t, ctxFiles, 1)
			require.Equal(t, tt.expected, ctxFiles[0].ContextFileContent)
		})
	}

	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
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

	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	t.Run("SkipsMissingDirs", func(t *testing.T) {
		nonExistent := filepath.Join(t.TempDir(), "does-not-exist")
		setupConfigTestEnv(t, map[string]string{
			agentcontextconfig.EnvInstructionsDirs: nonExistent,
			agentcontextconfig.EnvSkillsDirs:       nonExistent,
		})

		workDir := t.TempDir()
		cfg, _ := agentcontextconfig.Config(workDir)

		// Non-nil empty slice (signals agent supports new format).
		require.NotNil(t, cfg.Parts)
		require.Empty(t, cfg.Parts)
	})

	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	t.Run("MCPConfigFilesResolvedSeparately", func(t *testing.T) {
		optMCP := platformAbsPath("opt", "custom.json")
		fakeHome := setupConfigTestEnv(t, map[string]string{
			agentcontextconfig.EnvMCPConfigFiles: optMCP,
		})
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, fakeHome)

		workDir := t.TempDir()
		_, mcpFiles := agentcontextconfig.Config(workDir)

		require.Equal(t, []string{optMCP}, mcpFiles)
	})

	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	t.Run("SkillNameMustMatchDir", func(t *testing.T) {
		fakeHome := setupConfigTestEnv(t, nil)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, fakeHome)

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

	//nolint:paralleltest // Uses t.Setenv to mutate process-wide environment.
	t.Run("DuplicateSkillsFirstWins", func(t *testing.T) {
		fakeHome := setupConfigTestEnv(t, nil)
		t.Setenv(agentcontextconfig.EnvInstructionsDirs, fakeHome)

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
