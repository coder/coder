package chattool_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

// validSkillMD returns a valid SKILL.md with the given name and
// description.
func validSkillMD(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# Instructions\n\nDo the thing.\n"
}

func TestDiscoverSkills(t *testing.T) {
	t.Parallel()

	t.Run("FindsSkillsInWorkspace", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		// List the skills directory: returns two skill dirs.
		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, req workspacesdk.LSRequest) (workspacesdk.LSResponse, error) {
				require.Equal(t, []string{"/work/.agents/skills"}, req.Path)
				return workspacesdk.LSResponse{
					Contents: []workspacesdk.LSFile{
						{Name: "my-skill", IsDir: true, AbsolutePathString: "/work/.agents/skills/my-skill"},
						{Name: "other-skill", IsDir: true, AbsolutePathString: "/work/.agents/skills/other-skill"},
					},
				}, nil
			},
		)

		// Read SKILL.md for my-skill.
		conn.EXPECT().ReadFile(
			gomock.Any(),
			"/work/.agents/skills/my-skill/SKILL.md",
			int64(0),
			int64(64*1024+1),
		).Return(
			io.NopCloser(strings.NewReader(validSkillMD("my-skill", "first skill"))),
			"text/markdown",
			nil,
		)

		// Read SKILL.md for other-skill.
		conn.EXPECT().ReadFile(
			gomock.Any(),
			"/work/.agents/skills/other-skill/SKILL.md",
			int64(0),
			int64(64*1024+1),
		).Return(
			io.NopCloser(strings.NewReader(validSkillMD("other-skill", "second skill"))),
			"text/markdown",
			nil,
		)

		skills, err := chattool.DiscoverSkills(context.Background(), slogtest.Make(t, nil), conn, []string{"/work/.agents/skills"}, "SKILL.md")
		require.NoError(t, err)
		require.Len(t, skills, 2)
		assert.Equal(t, "my-skill", skills[0].Name)
		assert.Equal(t, "first skill", skills[0].Description)
		assert.Equal(t, "other-skill", skills[1].Name)
	})

	t.Run("SkillsDirMissing", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{},
			codersdk.NewTestError(404, "POST", "/api/v0/list-directory"),
		)

		skills, err := chattool.DiscoverSkills(context.Background(), slogtest.Make(t, nil), conn, []string{"/work/.agents/skills"}, "SKILL.md")
		require.NoError(t, err)
		require.Empty(t, skills)
	})

	t.Run("SkipsMissingSKILLmd", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{
					{Name: "broken", IsDir: true, AbsolutePathString: "/work/.agents/skills/broken"},
				},
			}, nil,
		)

		// SKILL.md doesn't exist.
		conn.EXPECT().ReadFile(
			gomock.Any(),
			"/work/.agents/skills/broken/SKILL.md",
			int64(0),
			int64(64*1024+1),
		).Return(
			nil, "",
			codersdk.NewTestError(404, "GET", "/api/v0/read-file"),
		)

		skills, err := chattool.DiscoverSkills(context.Background(), slogtest.Make(t, nil), conn, []string{"/work/.agents/skills"}, "SKILL.md")
		require.NoError(t, err)
		require.Empty(t, skills)
	})

	t.Run("SkipsInvalidFrontmatter", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{
					{Name: "bad", IsDir: true, AbsolutePathString: "/work/.agents/skills/bad"},
				},
			}, nil,
		)

		// No frontmatter delimiters.
		conn.EXPECT().ReadFile(
			gomock.Any(), gomock.Any(), int64(0), gomock.Any(),
		).Return(
			io.NopCloser(strings.NewReader("just some markdown")),
			"text/markdown",
			nil,
		)

		skills, err := chattool.DiscoverSkills(context.Background(), slogtest.Make(t, nil), conn, []string{"/work/.agents/skills"}, "SKILL.md")
		require.NoError(t, err)
		require.Empty(t, skills)
	})

	t.Run("SkipsMismatchedDirName", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{
					{Name: "dir-name", IsDir: true, AbsolutePathString: "/work/.agents/skills/dir-name"},
				},
			}, nil,
		)

		// name in frontmatter doesn't match dir name.
		conn.EXPECT().ReadFile(
			gomock.Any(), gomock.Any(), int64(0), gomock.Any(),
		).Return(
			io.NopCloser(strings.NewReader(validSkillMD("different-name", "desc"))),
			"text/markdown",
			nil,
		)

		skills, err := chattool.DiscoverSkills(context.Background(), slogtest.Make(t, nil), conn, []string{"/work/.agents/skills"}, "SKILL.md")
		require.NoError(t, err)
		require.Empty(t, skills)
	})

	t.Run("SkipsNonKebabCase", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{
					{Name: "UPPER", IsDir: true, AbsolutePathString: "/work/.agents/skills/UPPER"},
				},
			}, nil,
		)

		conn.EXPECT().ReadFile(
			gomock.Any(), gomock.Any(), int64(0), gomock.Any(),
		).Return(
			io.NopCloser(strings.NewReader(validSkillMD("UPPER", "bad name"))),
			"text/markdown",
			nil,
		)

		skills, err := chattool.DiscoverSkills(context.Background(), slogtest.Make(t, nil), conn, []string{"/work/.agents/skills"}, "SKILL.md")
		require.NoError(t, err)
		require.Empty(t, skills)
	})

	t.Run("SkipsNonDirectories", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{
					{Name: "README.md", IsDir: false, AbsolutePathString: "/work/.agents/skills/README.md"},
				},
			}, nil,
		)

		skills, err := chattool.DiscoverSkills(context.Background(), slogtest.Make(t, nil), conn, []string{"/work/.agents/skills"}, "SKILL.md")
		require.NoError(t, err)
		require.Empty(t, skills)
	})

	t.Run("QuotedDescription", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{
					{Name: "my-skill", IsDir: true, AbsolutePathString: "/work/.agents/skills/my-skill"},
				},
			}, nil,
		)

		// Description uses YAML-style quotes.
		md := "---\nname: my-skill\ndescription: \"A quoted description\"\n---\n\nBody.\n"
		conn.EXPECT().ReadFile(
			gomock.Any(), gomock.Any(), int64(0), gomock.Any(),
		).Return(
			io.NopCloser(strings.NewReader(md)),
			"text/markdown",
			nil,
		)

		skills, err := chattool.DiscoverSkills(context.Background(), slogtest.Make(t, nil), conn, []string{"/work/.agents/skills"}, "SKILL.md")
		require.NoError(t, err)
		require.Len(t, skills, 1)
		assert.Equal(t, "A quoted description", skills[0].Description)
	})

	t.Run("OversizedSKILLmdTruncated", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{
					{Name: "big-skill", IsDir: true, AbsolutePathString: "/work/.agents/skills/big-skill"},
				},
			}, nil,
		)

		// Build a SKILL.md larger than 64KB. The frontmatter is
		// at the start so it survives truncation.
		bigBody := strings.Repeat("x", 70*1024)
		md := "---\nname: big-skill\ndescription: large\n---\n" + bigBody
		conn.EXPECT().ReadFile(
			gomock.Any(), gomock.Any(), int64(0), gomock.Any(),
		).Return(
			io.NopCloser(strings.NewReader(md)),
			"text/markdown",
			nil,
		)

		skills, err := chattool.DiscoverSkills(context.Background(), slogtest.Make(t, nil), conn, []string{"/work/.agents/skills"}, "SKILL.md")
		require.NoError(t, err)
		// The skill should still be discovered since the
		// frontmatter fits within the truncation limit.
		require.Len(t, skills, 1)
		assert.Equal(t, "big-skill", skills[0].Name)
	})

	t.Run("BOMHandled", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{
					{Name: "bom-skill", IsDir: true, AbsolutePathString: "/work/.agents/skills/bom-skill"},
				},
			}, nil,
		)

		// UTF-8 BOM prefix before the frontmatter.
		md := "\xef\xbb\xbf---\nname: bom-skill\ndescription: has BOM\n---\n\nBody.\n"
		conn.EXPECT().ReadFile(
			gomock.Any(), gomock.Any(), int64(0), gomock.Any(),
		).Return(
			io.NopCloser(strings.NewReader(md)),
			"text/markdown",
			nil,
		)

		skills, err := chattool.DiscoverSkills(context.Background(), slogtest.Make(t, nil), conn, []string{"/work/.agents/skills"}, "SKILL.md")
		require.NoError(t, err)
		require.Len(t, skills, 1)
		assert.Equal(t, "bom-skill", skills[0].Name)
	})
}

func TestFormatSkillIndex(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, chattool.FormatSkillIndex(nil))
	})

	t.Run("RendersIndex", func(t *testing.T) {
		t.Parallel()

		skills := []chattool.SkillMeta{
			{Name: "alpha", Description: "First"},
			{Name: "beta", Description: "Second"},
		}
		idx := chattool.FormatSkillIndex(skills)
		assert.Contains(t, idx, "<available-skills>")
		assert.Contains(t, idx, "- alpha: First")
		assert.Contains(t, idx, "- beta: Second")
		assert.Contains(t, idx, "</available-skills>")
		assert.Contains(t, idx, "read_skill")
	})
}

func TestLoadSkillBody(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsBodyAndFiles", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		skill := chattool.SkillMeta{
			Name:        "my-skill",
			Description: "desc",
			Dir:         "/work/.agents/skills/my-skill",
		}

		// Read the full SKILL.md.
		conn.EXPECT().ReadFile(
			gomock.Any(),
			"/work/.agents/skills/my-skill/SKILL.md",
			int64(0),
			int64(64*1024+1),
		).Return(
			io.NopCloser(strings.NewReader(validSkillMD("my-skill", "desc"))),
			"text/markdown",
			nil,
		)

		// List supporting files.
		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{
					{Name: "SKILL.md"},
					{Name: "helper.md"},
					{Name: "roles", IsDir: true},
				},
			}, nil,
		)

		content, err := chattool.LoadSkillBody(context.Background(), conn, skill, "SKILL.md")
		require.NoError(t, err)
		assert.Contains(t, content.Body, "Do the thing.")
		assert.Equal(t, []string{"helper.md", "roles/"}, content.Files)
	})
}

func TestLoadSkillFile(t *testing.T) {
	t.Parallel()

	t.Run("ValidFile", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		skill := chattool.SkillMeta{
			Name: "my-skill",
			Dir:  "/work/.agents/skills/my-skill",
		}

		conn.EXPECT().ReadFile(
			gomock.Any(),
			"/work/.agents/skills/my-skill/roles/reviewer.md",
			int64(0),
			int64(512*1024+1),
		).Return(
			io.NopCloser(strings.NewReader("review instructions")),
			"text/markdown",
			nil,
		)

		content, err := chattool.LoadSkillFile(
			context.Background(), conn, skill, "roles/reviewer.md",
		)
		require.NoError(t, err)
		assert.Equal(t, "review instructions", content)
	})

	t.Run("PathTraversalRejected", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		skill := chattool.SkillMeta{
			Name: "my-skill",
			Dir:  "/work/.agents/skills/my-skill",
		}

		_, err := chattool.LoadSkillFile(
			context.Background(), conn, skill, "../../etc/passwd",
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "traversal")
	})

	t.Run("AbsolutePathRejected", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		skill := chattool.SkillMeta{
			Name: "my-skill",
			Dir:  "/work/.agents/skills/my-skill",
		}

		_, err := chattool.LoadSkillFile(
			context.Background(), conn, skill, "/etc/passwd",
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "absolute")
	})

	t.Run("HiddenFileRejected", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		skill := chattool.SkillMeta{
			Name: "my-skill",
			Dir:  "/work/.agents/skills/my-skill",
		}

		_, err := chattool.LoadSkillFile(
			context.Background(), conn, skill, ".git/config",
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "hidden")
	})

	t.Run("EmptyPathRejected", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		skill := chattool.SkillMeta{
			Name: "my-skill",
			Dir:  "/work/.agents/skills/my-skill",
		}

		_, err := chattool.LoadSkillFile(
			context.Background(), conn, skill, "",
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("OversizedFileTruncated", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		skill := chattool.SkillMeta{
			Name: "my-skill",
			Dir:  "/work/.agents/skills/my-skill",
		}

		// Build a file that exceeds maxSkillFileBytes (512KB).
		bigContent := strings.Repeat("x", 512*1024+100)

		conn.EXPECT().ReadFile(
			gomock.Any(),
			"/work/.agents/skills/my-skill/large.txt",
			int64(0),
			int64(512*1024+1),
		).Return(
			io.NopCloser(strings.NewReader(bigContent)),
			"text/plain",
			nil,
		)

		content, err := chattool.LoadSkillFile(
			context.Background(), conn, skill, "large.txt",
		)
		require.NoError(t, err)
		assert.Equal(t, 512*1024, len(content),
			"content should be truncated to maxSkillFileBytes")
	})
}

func TestReadSkillTool(t *testing.T) {
	t.Parallel()

	t.Run("ValidSkill", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		skills := []chattool.SkillMeta{{
			Name:        "my-skill",
			Description: "test",
			Dir:         "/work/.agents/skills/my-skill",
		}}

		conn.EXPECT().ReadFile(
			gomock.Any(), gomock.Any(), int64(0), gomock.Any(),
		).Return(
			io.NopCloser(strings.NewReader(validSkillMD("my-skill", "test"))),
			"text/markdown",
			nil,
		)
		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{
					{Name: "SKILL.md"},
				},
			}, nil,
		)

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return conn, nil
			},
			GetSkills: func() []chattool.SkillMeta { return skills },
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":"my-skill"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Contains(t, resp.Content, "Do the thing.")
	})

	t.Run("UnknownSkill", func(t *testing.T) {
		t.Parallel()

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				t.Fatal("unexpected call to GetWorkspaceConn")
				return nil, xerrors.New("unreachable")
			},
			GetSkills: func() []chattool.SkillMeta { return nil },
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":"nonexistent"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "not found")
	})

	t.Run("EmptyName", func(t *testing.T) {
		t.Parallel()

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				t.Fatal("unexpected call to GetWorkspaceConn")
				return nil, xerrors.New("unreachable")
			},
			GetSkills: func() []chattool.SkillMeta { return nil },
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":""}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "required")
	})
}

func TestReadSkillFileTool(t *testing.T) {
	t.Parallel()

	t.Run("ValidFile", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		skills := []chattool.SkillMeta{{
			Name: "my-skill",
			Dir:  "/work/.agents/skills/my-skill",
		}}

		conn.EXPECT().ReadFile(
			gomock.Any(),
			"/work/.agents/skills/my-skill/roles/reviewer.md",
			int64(0),
			int64(512*1024+1),
		).Return(
			io.NopCloser(strings.NewReader("reviewer guide")),
			"text/markdown",
			nil,
		)

		tool := chattool.ReadSkillFile(chattool.ReadSkillOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return conn, nil
			},
			GetSkills: func() []chattool.SkillMeta { return skills },
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill_file",
			Input: `{"name":"my-skill","path":"roles/reviewer.md"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Contains(t, resp.Content, "reviewer guide")
	})

	t.Run("TraversalRejected", func(t *testing.T) {
		t.Parallel()

		skills := []chattool.SkillMeta{{
			Name: "my-skill",
			Dir:  "/work/.agents/skills/my-skill",
		}}

		tool := chattool.ReadSkillFile(chattool.ReadSkillOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				t.Fatal("unexpected call to GetWorkspaceConn")
				return nil, xerrors.New("unreachable")
			},
			GetSkills: func() []chattool.SkillMeta { return skills },
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill_file",
			Input: `{"name":"my-skill","path":"../../etc/passwd"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "traversal")
	})
}
