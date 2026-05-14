package chattool_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	skillspkg "github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

// validSkillMD returns a valid SKILL.md with the given name and
// description.
func validSkillMD(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# Instructions\n\nDo the thing.\n"
}

func responseName(t *testing.T, resp fantasy.ToolResponse) string {
	t.Helper()

	var payload struct {
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &payload))
	return payload.Name
}

func TestFormatResolvedSkillIndex(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, chattool.FormatResolvedSkillIndex(nil))
	})

	t.Run("PersonalOnly", func(t *testing.T) {
		t.Parallel()

		idx := chattool.FormatResolvedSkillIndex([]skillspkg.ResolvedSkill{{
			Skill: skillspkg.Skill{
				Name:        "personal-review",
				Description: "Personal review process",
				Source:      skillspkg.SourcePersonal,
			},
			Alias: "personal-review",
		}})
		assert.Contains(t, idx, "- personal-review: Personal review process")
		assert.NotContains(t, idx, "qualified alias")
	})

	t.Run("WorkspaceOnlyMatchesLegacy", func(t *testing.T) {
		t.Parallel()

		resolved := []skillspkg.ResolvedSkill{{
			Skill: skillspkg.Skill{
				Name:        "deep-review",
				Description: "Review",
				Source:      skillspkg.SourceWorkspace,
			},
			Alias: "deep-review",
		}}
		assert.Equal(t,
			"<available-skills>\n"+
				"Use read_skill to load a skill's full instructions before following them.\n"+
				"Use read_skill_file to read supporting files referenced by a skill.\n"+
				"\n"+
				"- deep-review: Review\n"+
				"</available-skills>",
			chattool.FormatResolvedSkillIndex(resolved),
		)
	})

	t.Run("MixedNonColliding", func(t *testing.T) {
		t.Parallel()

		idx := chattool.FormatResolvedSkillIndex([]skillspkg.ResolvedSkill{
			{
				Skill: skillspkg.Skill{
					Name:        "personal-review",
					Description: "Personal review process",
					Source:      skillspkg.SourcePersonal,
				},
				Alias: "personal-review",
			},
			{
				Skill: skillspkg.Skill{
					Name:        "deep-review",
					Description: "Workspace review process",
					Source:      skillspkg.SourceWorkspace,
				},
				Alias: "deep-review",
			},
		})
		assert.Contains(t, idx, "- personal-review: Personal review process")
		assert.Contains(t, idx, "- deep-review: Workspace review process")
		assert.NotContains(t, idx, "personal/personal-review")
		assert.NotContains(t, idx, "workspace/deep-review")
	})

	t.Run("CollidingNames", func(t *testing.T) {
		t.Parallel()

		resolved := skillspkg.MergeSkills(
			[]skillspkg.Skill{{Name: "review", Description: "Personal", Source: skillspkg.SourcePersonal}},
			[]skillspkg.Skill{{Name: "review", Description: "Workspace", Source: skillspkg.SourceWorkspace}},
		)
		idx := chattool.FormatResolvedSkillIndex(resolved)
		assert.Contains(t, idx, "- personal/review: Personal")
		assert.Contains(t, idx, "- workspace/review: Workspace")
		assert.Contains(t, idx, "pass that qualified alias to read_skill")
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

	t.Run("PersonalSkill", func(t *testing.T) {
		t.Parallel()

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			ResolveAlias: func(alias string) (skillspkg.ResolvedSkill, error) {
				require.Equal(t, "my-skill", alias)
				return skillspkg.ResolvedSkill{
					Skill: skillspkg.Skill{
						Name:        "my-skill",
						Description: "test",
						Source:      skillspkg.SourcePersonal,
					},
					Alias: "my-skill",
				}, nil
			},
			LoadPersonalSkillBody: func(context.Context, string) (skillspkg.ParsedSkill, error) {
				return skillspkg.ParsedSkill{
					Skill: skillspkg.Skill{
						Name:        "my-skill",
						Description: "test",
						Source:      skillspkg.SourcePersonal,
					},
					Body: "Personal instructions.",
				}, nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":"my-skill"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Contains(t, resp.Content, "Personal instructions.")
		assert.Contains(t, resp.Content, `"files":[]`)
	})

	t.Run("PersonalQualifiedAliasPreservesAlias", func(t *testing.T) {
		t.Parallel()

		var loadedName string
		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			ResolveAlias: func(alias string) (skillspkg.ResolvedSkill, error) {
				require.Equal(t, "personal/my-skill", alias)
				return skillspkg.ResolvedSkill{
					Skill: skillspkg.Skill{
						Name:        "my-skill",
						Description: "test",
						Source:      skillspkg.SourcePersonal,
					},
					Alias: "personal/my-skill",
				}, nil
			},
			LoadPersonalSkillBody: func(_ context.Context, name string) (skillspkg.ParsedSkill, error) {
				loadedName = name
				return skillspkg.ParsedSkill{
					Skill: skillspkg.Skill{
						Name:        "my-skill",
						Description: "test",
						Source:      skillspkg.SourcePersonal,
					},
					Body: "Personal instructions.",
				}, nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":"personal/my-skill"}`,
		})

		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Equal(t, "personal/my-skill", responseName(t, resp))
		assert.Equal(t, "my-skill", loadedName)
	})

	t.Run("WorkspaceQualifiedAlias", func(t *testing.T) {
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
			workspacesdk.LSResponse{}, nil,
		)

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return conn, nil
			},
			GetSkills: func() []chattool.SkillMeta { return skills },
			ResolveAlias: func(alias string) (skillspkg.ResolvedSkill, error) {
				require.Equal(t, "workspace/my-skill", alias)
				return skillspkg.ResolvedSkill{
					Skill: skillspkg.Skill{
						Name:        "my-skill",
						Description: "test",
						Source:      skillspkg.SourceWorkspace,
					},
					Alias: "workspace/my-skill",
				}, nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":"workspace/my-skill"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Equal(t, "workspace/my-skill", responseName(t, resp))
		assert.Contains(t, resp.Content, "Do the thing.")
	})

	t.Run("CollisionAliasRoundTrip", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		workspaceSkills := []chattool.SkillMeta{{
			Name:        "deploy",
			Description: "workspace deploy",
			Dir:         "/work/.agents/skills/deploy",
		}}

		conn.EXPECT().ReadFile(
			gomock.Any(), gomock.Any(), int64(0), gomock.Any(),
		).Return(
			io.NopCloser(strings.NewReader(validSkillMD("deploy", "workspace deploy"))),
			"text/markdown",
			nil,
		)
		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{}, nil,
		)

		resolveAlias := func(alias string) (skillspkg.ResolvedSkill, error) {
			switch alias {
			case "personal/deploy":
				return skillspkg.ResolvedSkill{
					Skill: skillspkg.Skill{
						Name:        "deploy",
						Description: "personal deploy",
						Source:      skillspkg.SourcePersonal,
					},
					Alias: "personal/deploy",
				}, nil
			case "workspace/deploy":
				return skillspkg.ResolvedSkill{
					Skill: skillspkg.Skill{
						Name:        "deploy",
						Description: "workspace deploy",
						Source:      skillspkg.SourceWorkspace,
					},
					Alias: "workspace/deploy",
				}, nil
			default:
				return skillspkg.ResolvedSkill{}, skillspkg.ErrSkillNotFound
			}
		}
		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return conn, nil
			},
			GetSkills:    func() []chattool.SkillMeta { return workspaceSkills },
			ResolveAlias: resolveAlias,
			LoadPersonalSkillBody: func(_ context.Context, name string) (skillspkg.ParsedSkill, error) {
				require.Equal(t, "deploy", name)
				return skillspkg.ParsedSkill{
					Skill: skillspkg.Skill{
						Name:        "deploy",
						Description: "personal deploy",
						Source:      skillspkg.SourcePersonal,
					},
					Body: "Personal deploy instructions.",
				}, nil
			},
		})

		workspaceResp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":"workspace/deploy"}`,
		})
		require.NoError(t, err)
		assert.False(t, workspaceResp.IsError)
		workspaceName := responseName(t, workspaceResp)
		assert.Equal(t, "workspace/deploy", workspaceName)
		workspaceResolved, err := resolveAlias(workspaceName)
		require.NoError(t, err)
		assert.Equal(t, skillspkg.SourceWorkspace, workspaceResolved.Source)

		personalResp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-2",
			Name:  "read_skill",
			Input: `{"name":"personal/deploy"}`,
		})
		require.NoError(t, err)
		assert.False(t, personalResp.IsError)
		personalName := responseName(t, personalResp)
		assert.Equal(t, "personal/deploy", personalName)
		personalResolved, err := resolveAlias(personalName)
		require.NoError(t, err)
		assert.Equal(t, skillspkg.SourcePersonal, personalResolved.Source)

		_, err = resolveAlias("deploy")
		require.ErrorIs(t, err, skillspkg.ErrSkillNotFound)
	})

	t.Run("MissingPersonalSkill", func(t *testing.T) {
		t.Parallel()

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			ResolveAlias: func(alias string) (skillspkg.ResolvedSkill, error) {
				return skillspkg.ResolvedSkill{
					Skill: skillspkg.Skill{Name: alias, Source: skillspkg.SourcePersonal},
					Alias: alias,
				}, nil
			},
			LoadPersonalSkillBody: func(context.Context, string) (skillspkg.ParsedSkill, error) {
				return skillspkg.ParsedSkill{}, skillspkg.ErrSkillNotFound
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":"missing-skill"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, `skill "missing-skill" not found`)
	})

	t.Run("PersonalSkillLoaderErrorIsSanitized", func(t *testing.T) {
		t.Parallel()

		sink := testutil.NewFakeSink(t)
		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			Logger: sink.Logger(),
			ResolveAlias: func(alias string) (skillspkg.ResolvedSkill, error) {
				return skillspkg.ResolvedSkill{
					Skill: skillspkg.Skill{Name: alias, Source: skillspkg.SourcePersonal},
					Alias: alias,
				}, nil
			},
			LoadPersonalSkillBody: func(context.Context, string) (skillspkg.ParsedSkill, error) {
				return skillspkg.ParsedSkill{}, xerrors.New("synthetic private storage failure")
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":"my-skill"}`,
		})

		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "failed to load personal skill")
		assert.NotContains(t, resp.Content, "synthetic private storage failure")
		entries := sink.Entries(func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelError && e.Message == "failed to load personal skill"
		})
		require.Len(t, entries, 1)
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

	t.Run("PersonalSkillUnsupported", func(t *testing.T) {
		t.Parallel()

		tool := chattool.ReadSkillFile(chattool.ReadSkillOptions{
			ResolveAlias: func(alias string) (skillspkg.ResolvedSkill, error) {
				return skillspkg.ResolvedSkill{
					Skill: skillspkg.Skill{Name: alias, Source: skillspkg.SourcePersonal},
					Alias: alias,
				}, nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill_file",
			Input: `{"name":"my-skill","path":"helper.md"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "not supported for personal skills")
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
