package chatd //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"io"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

func TestSanitizeInstructionMarkdown(t *testing.T) {
	t.Parallel()

	input := "line 1\r\n<!-- hidden -->\r\nline 2\r\n"
	require.Equal(t, "line 1\n\nline 2", sanitizeInstructionMarkdown(input))
}

func TestReadHomeInstructionFileNotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).DoAndReturn(
		func(context.Context, string, workspacesdk.LSRequest) (workspacesdk.LSResponse, error) {
			return workspacesdk.LSResponse{}, codersdk.NewTestError(404, "POST", "/api/v0/list-directory")
		},
	)

	content, sourcePath, truncated, err := readHomeInstructionFile(context.Background(), conn)
	require.NoError(t, err)
	require.Empty(t, content)
	require.Empty(t, sourcePath)
	require.False(t, truncated)
}

func TestReadHomeInstructionFileSuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)

	conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).DoAndReturn(
		func(context.Context, string, workspacesdk.LSRequest) (workspacesdk.LSResponse, error) {
			return workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{{
					Name:               "AGENTS.md",
					AbsolutePathString: "/home/coder/.coder/AGENTS.md",
				}},
			}, nil
		},
	)
	conn.EXPECT().ReadFile(
		gomock.Any(),
		"/home/coder/.coder/AGENTS.md",
		int64(0),
		int64(maxInstructionFileBytes+1),
	).Return(
		io.NopCloser(strings.NewReader("base\n<!-- hidden -->\nlocal")),
		"text/markdown",
		nil,
	)

	content, sourcePath, truncated, err := readHomeInstructionFile(context.Background(), conn)
	require.NoError(t, err)
	require.Equal(t, "base\n\nlocal", content)
	require.Equal(t, "/home/coder/.coder/AGENTS.md", sourcePath)
	require.False(t, truncated)
}

func TestReadHomeInstructionFileTruncates(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	content := strings.Repeat("a", maxInstructionFileBytes+8)

	conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
		workspacesdk.LSResponse{
			Contents: []workspacesdk.LSFile{{
				Name:               "AGENTS.md",
				AbsolutePathString: "/home/coder/.coder/AGENTS.md",
			}},
		},
		nil,
	)
	conn.EXPECT().ReadFile(
		gomock.Any(),
		"/home/coder/.coder/AGENTS.md",
		int64(0),
		int64(maxInstructionFileBytes+1),
	).Return(io.NopCloser(strings.NewReader(content)), "text/markdown", nil)

	got, _, truncated, err := readHomeInstructionFile(context.Background(), conn)
	require.NoError(t, err)
	require.True(t, truncated)
	require.Len(t, got, maxInstructionFileBytes)
}

func TestReadInstructionFile(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		conn.EXPECT().ReadFile(
			gomock.Any(),
			"/home/coder/project/AGENTS.md",
			int64(0),
			int64(maxInstructionFileBytes+1),
		).Return(
			io.NopCloser(strings.NewReader("project rules")),
			"text/markdown",
			nil,
		)

		content, source, truncated, err := readInstructionFile(
			context.Background(), conn, "/home/coder/project/AGENTS.md",
		)
		require.NoError(t, err)
		require.Equal(t, "project rules", content)
		require.Equal(t, "/home/coder/project/AGENTS.md", source)
		require.False(t, truncated)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		conn.EXPECT().ReadFile(
			gomock.Any(),
			"/home/coder/project/AGENTS.md",
			int64(0),
			int64(maxInstructionFileBytes+1),
		).Return(nil, "", codersdk.NewTestError(404, "GET", "/api/v0/read-file"))

		content, source, truncated, err := readInstructionFile(
			context.Background(), conn, "/home/coder/project/AGENTS.md",
		)
		require.NoError(t, err)
		require.Empty(t, content)
		require.Empty(t, source)
		require.False(t, truncated)
	})
}

func TestInsertSystemInstructionAfterSystemMessages(t *testing.T) {
	t.Parallel()

	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "base"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "hello"},
			},
		},
	}

	got := chatprompt.InsertSystem(prompt, "project rules")
	require.Len(t, got, 3)
	require.Equal(t, fantasy.MessageRoleSystem, got[0].Role)
	require.Equal(t, fantasy.MessageRoleSystem, got[1].Role)
	require.Equal(t, fantasy.MessageRoleUser, got[2].Role)

	part, ok := fantasy.AsMessagePart[fantasy.TextPart](got[1].Content[0])
	require.True(t, ok)
	require.Equal(t, "project rules", part.Text)
}

func TestFormatSystemInstructions(t *testing.T) {
	t.Parallel()

	t.Run("HomeAndPwdWithAgentContext", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("linux", "/home/coder/project", []instructionFileSection{
			{content: "home rules", source: "/home/coder/.coder/AGENTS.md"},
			{content: "project rules", source: "/home/coder/project/AGENTS.md"},
		}, nil)
		require.Contains(t, got, "Operating System: linux")
		require.Contains(t, got, "Working Directory: /home/coder/project")
		require.Contains(t, got, "Source: /home/coder/.coder/AGENTS.md")
		require.Contains(t, got, "home rules")
		require.Contains(t, got, "Source: /home/coder/project/AGENTS.md")
		require.Contains(t, got, "project rules")
		require.True(t, strings.HasPrefix(got, "<workspace-context>"))
		require.True(t, strings.HasSuffix(got, "</workspace-context>"))
	})

	t.Run("OnlyPwdFile", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("", "/home/coder/project", []instructionFileSection{
			{content: "project rules", source: "/home/coder/project/AGENTS.md"},
		}, nil)
		require.Contains(t, got, "project rules")
		require.Contains(t, got, "Source: /home/coder/project/AGENTS.md")
		require.NotContains(t, got, ".coder/AGENTS.md")
	})

	t.Run("OnlyAgentContext", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("darwin", "/Users/dev/repo", nil, nil)
		require.Contains(t, got, "Operating System: darwin")
		require.Contains(t, got, "Working Directory: /Users/dev/repo")
		require.NotContains(t, got, "Source:")
		require.True(t, strings.HasPrefix(got, "<workspace-context>"))
		require.True(t, strings.HasSuffix(got, "</workspace-context>"))
	})

	t.Run("OnlyHomeFile", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("", "", []instructionFileSection{
			{content: "home rules", source: "~/.coder/AGENTS.md"},
		}, nil)
		require.Contains(t, got, "Source: ~/.coder/AGENTS.md")
		require.Contains(t, got, "home rules")
		require.NotContains(t, got, "Operating System:")
		require.NotContains(t, got, "Working Directory:")
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("", "", nil, nil)
		require.Empty(t, got)
	})

	t.Run("TruncatedFile", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("windows", "", []instructionFileSection{
			{content: "rules", source: "/path/AGENTS.md", truncated: true},
		}, nil)
		require.Contains(t, got, "truncated to 64KiB")
		require.Contains(t, got, "Operating System: windows")
	})

	t.Run("AgentContextBeforeFiles", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("linux", "/home/project", []instructionFileSection{
			{content: "home", source: "/home/.coder/AGENTS.md"},
			{content: "pwd", source: "/home/project/AGENTS.md"},
		}, nil)
		osIdx := strings.Index(got, "Operating System:")
		dirIdx := strings.Index(got, "Working Directory:")
		homeSourceIdx := strings.Index(got, "Source: /home/.coder/AGENTS.md")
		pwdSourceIdx := strings.Index(got, "Source: /home/project/AGENTS.md")
		require.Less(t, osIdx, homeSourceIdx)
		require.Less(t, dirIdx, homeSourceIdx)
		require.Less(t, homeSourceIdx, pwdSourceIdx)
	})

	t.Run("EmptySectionsIgnored", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("linux", "", []instructionFileSection{
			{content: "", source: "/empty"},
			{content: "real", source: "/real/AGENTS.md"},
		}, nil)
		require.NotContains(t, got, "Source: /empty")
		require.Contains(t, got, "Source: /real/AGENTS.md")
	})
}

func TestPwdInstructionFilePath(t *testing.T) {
	t.Parallel()
	require.Equal(t, "/home/coder/project/AGENTS.md", pwdInstructionFilePath("/home/coder/project"))
	require.Empty(t, pwdInstructionFilePath(""))
}

func TestFormatSkillsBlock(t *testing.T) {
	t.Parallel()

	t.Run("SingleSkill", func(t *testing.T) {
		t.Parallel()
		got := formatSkillsBlock([]Skill{
			{Name: "review", Description: "Reviews code", Path: "/home/coder/.coder/skills/review/SKILL.md"},
		})
		require.Contains(t, got, "<available_skills>")
		require.Contains(t, got, "</available_skills>")
		require.Contains(t, got, "<name>review</name>")
		require.Contains(t, got, "<description>Reviews code</description>")
		require.Contains(t, got, "<location>/home/coder/.coder/skills/review/SKILL.md</location>")
	})

	t.Run("MultipleSkills", func(t *testing.T) {
		t.Parallel()
		got := formatSkillsBlock([]Skill{
			{Name: "alpha", Description: "First", Path: "/a/SKILL.md"},
			{Name: "beta", Description: "Second", Path: "/b/SKILL.md"},
		})
		require.Contains(t, got, "<name>alpha</name>")
		require.Contains(t, got, "<name>beta</name>")
		// Verify order: alpha before beta
		alphaIdx := strings.Index(got, "<name>alpha</name>")
		betaIdx := strings.Index(got, "<name>beta</name>")
		require.Less(t, alphaIdx, betaIdx)
	})

	t.Run("EmptySlice", func(t *testing.T) {
		t.Parallel()
		got := formatSkillsBlock([]Skill{})
		require.Empty(t, got)
	})

	t.Run("NilSlice", func(t *testing.T) {
		t.Parallel()
		got := formatSkillsBlock(nil)
		require.Empty(t, got)
	})

	t.Run("EmptyDescription", func(t *testing.T) {
		t.Parallel()
		got := formatSkillsBlock([]Skill{
			{Name: "test", Description: "", Path: "/test/SKILL.md"},
		})
		require.Contains(t, got, "<description></description>")
	})

	t.Run("SpecialCharactersEscaped", func(t *testing.T) {
		t.Parallel()
		got := formatSkillsBlock([]Skill{
			{Name: "test<>&", Description: "a \"quoted\" & 'escaped' <value>", Path: "/path/SKILL.md"},
		})
		require.Contains(t, got, "<name>test&lt;&gt;&amp;</name>")
		require.Contains(t, got, "a &quot;quoted&quot; &amp; &apos;escaped&apos; &lt;value&gt;")
	})
}

func TestFormatSystemInstructions_WithSkills(t *testing.T) {
	t.Parallel()

	t.Run("SkillsAfterAgentsMd", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("linux", "/home/coder", []instructionFileSection{
			{content: "home rules", source: "/home/coder/.coder/AGENTS.md"},
		}, []Skill{
			{Name: "review", Description: "Reviews code", Path: "/home/coder/.coder/skills/review/SKILL.md"},
		})
		require.Contains(t, got, "<available_skills>")
		require.Contains(t, got, "</available_skills>")
		agentsIdx := strings.Index(got, "home rules")
		skillsIdx := strings.Index(got, "<available_skills>")
		closeIdx := strings.Index(got, "</workspace-context>")
		require.Less(t, agentsIdx, skillsIdx)
		require.Less(t, skillsIdx, closeIdx)
	})

	t.Run("SkillsOnly", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("", "", nil, []Skill{
			{Name: "test", Description: "A test skill", Path: "/test/SKILL.md"},
		})
		require.Contains(t, got, "<workspace-context>")
		require.Contains(t, got, "<available_skills>")
		require.Contains(t, got, "</workspace-context>")
	})

	t.Run("NoSkills_NoBlock", func(t *testing.T) {
		t.Parallel()
		got := formatSystemInstructions("linux", "/home", []instructionFileSection{
			{content: "rules", source: "/AGENTS.md"},
		}, nil)
		require.NotContains(t, got, "<available_skills>")
	})
}

func TestFetchSkillsFromAgent(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().ListSkills(gomock.Any()).Return([]workspacesdk.SkillMetadata{
			{Name: "review", Description: "Reviews code", Path: "/home/coder/.coder/skills/review/SKILL.md"},
		}, nil)

		skills, err := fetchSkillsFromAgent(context.Background(), conn)
		require.NoError(t, err)
		require.Len(t, skills, 1)
		require.Equal(t, "review", skills[0].Name)
		require.Equal(t, "Reviews code", skills[0].Description)
		require.Equal(t, "/home/coder/.coder/skills/review/SKILL.md", skills[0].Path)
	})

	t.Run("AgentReturnsError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().ListSkills(gomock.Any()).Return(nil, xerrors.New("connection refused"))

		skills, err := fetchSkillsFromAgent(context.Background(), conn)
		require.Error(t, err)
		require.Nil(t, skills)
	})

	t.Run("NilConnection", func(t *testing.T) {
		t.Parallel()
		skills, err := fetchSkillsFromAgent(context.Background(), nil)
		require.NoError(t, err)
		require.Nil(t, skills)
	})

	t.Run("EmptySkills", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().ListSkills(gomock.Any()).Return([]workspacesdk.SkillMetadata{}, nil)

		skills, err := fetchSkillsFromAgent(context.Background(), conn)
		require.NoError(t, err)
		require.Empty(t, skills)
	})
}
