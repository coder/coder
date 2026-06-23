package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentsocket"
)

func TestResolveContextSourcePath(t *testing.T) {
	t.Parallel()

	t.Run("EmptyErrors", func(t *testing.T) {
		t.Parallel()
		_, err := resolveContextSourcePath("   ")
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty")
	})

	t.Run("PreservesTilde", func(t *testing.T) {
		t.Parallel()
		// A leading ~ is left for the agent to expand against its own home.
		got, err := resolveContextSourcePath("~")
		require.NoError(t, err)
		require.Equal(t, "~", got)

		got, err = resolveContextSourcePath("  ~/skills/deploy  ")
		require.NoError(t, err)
		require.Equal(t, "~/skills/deploy", got)
	})

	t.Run("KeepsAbsolute", func(t *testing.T) {
		t.Parallel()
		got, err := resolveContextSourcePath("/home/coder/AGENTS.md")
		require.NoError(t, err)
		require.Equal(t, "/home/coder/AGENTS.md", got)
	})

	t.Run("MakesRelativeAbsolute", func(t *testing.T) {
		t.Parallel()
		// "./" was the reported failure: a relative path must be resolved to an
		// absolute one before it reaches the agent.
		got, err := resolveContextSourcePath("./")
		require.NoError(t, err)
		require.True(t, filepath.IsAbs(got), "want absolute, got %q", got)
		want, err := filepath.Abs("./")
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}

func TestContextResourceFromSource(t *testing.T) {
	t.Parallel()

	// Built-in scan roots overlap user sources, and the agent attributes an
	// overlapping resource to the first (built-in / working-dir) root, leaving
	// SourcePath empty. `show` therefore has to fall back to path containment.
	const (
		coderHome = "/home/coder/.coder"
		skillsDir = "/home/coder/coder/.agents/skills"
	)

	cases := []struct {
		name    string
		res     agentsocket.ContextResource
		srcPath string
		want    bool
	}{
		{
			name: "ExplicitSourcePathMatch",
			// SourcePath wins even when Source points elsewhere.
			res: agentsocket.ContextResource{
				Kind:       "instruction_file",
				Source:     "/var/lib/builtin/AGENTS.md",
				SourcePath: "/home/coder/AGENTS.md",
			},
			srcPath: "/home/coder/AGENTS.md",
			want:    true,
		},
		{
			name: "InstructionFileUnderBuiltinRoot",
			// Empty SourcePath: matched only by containment under the root.
			res: agentsocket.ContextResource{
				Kind:   "instruction_file",
				Source: coderHome + "/AGENTS.md",
			},
			srcPath: coderHome,
			want:    true,
		},
		{
			name: "SkillUnderSkillsRoot",
			res: agentsocket.ContextResource{
				Kind:   "skill",
				Source: skillsDir + "/deploy/SKILL.md",
			},
			srcPath: skillsDir,
			want:    true,
		},
		{
			name: "ExactFileSource",
			res: agentsocket.ContextResource{
				Kind:   "instruction_file",
				Source: "/home/coder/AGENTS.md",
			},
			srcPath: "/home/coder/AGENTS.md",
			want:    true,
		},
		{
			name: "DifferentSubtreeDoesNotMatch",
			res: agentsocket.ContextResource{
				Kind:   "skill",
				Source: "/home/coder/other/skills/deploy/SKILL.md",
			},
			srcPath: skillsDir,
			want:    false,
		},
		{
			name: "SymlinkedSkillNotUnderBuiltinRoot",
			// A skill resolved outside the built-in root must not be attributed
			// to it just because the user added the built-in root.
			res: agentsocket.ContextResource{
				Kind:   "skill",
				Source: "/home/coder/my-agent/skills/deploy/SKILL.md",
			},
			srcPath: coderHome,
			want:    false,
		},
		{
			name: "McpServerMatchedBySourcePath",
			// Source carries the server name, not a path; SourcePath still matches.
			res: agentsocket.ContextResource{
				Kind:       "mcp_server",
				Name:       "playwright",
				Source:     "playwright",
				SourcePath: "/home/coder/.mcp.json",
			},
			srcPath: "/home/coder/.mcp.json",
			want:    true,
		},
		{
			name: "McpServerNotMatchedByName",
			// Server name in Source must never be treated as a path to contain.
			res: agentsocket.ContextResource{
				Kind:   "mcp_server",
				Name:   "playwright",
				Source: "playwright",
			},
			srcPath: "playwright",
			want:    false,
		},
		{
			name: "PrefixBoundaryNotMatched",
			// /a/bc is not under /a/b despite the shared string prefix.
			res: agentsocket.ContextResource{
				Kind:   "instruction_file",
				Source: "/a/bc/file.md",
			},
			srcPath: "/a/b",
			want:    false,
		},
		{
			name: "EmptySourcePathNeverMatches",
			res: agentsocket.ContextResource{
				Kind:   "instruction_file",
				Source: "/home/coder/AGENTS.md",
			},
			srcPath: "",
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, contextResourceFromSource(tc.res, tc.srcPath))
		})
	}
}
