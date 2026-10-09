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

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	skillspkg "github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

// skillFileReadLimit is the byte limit the skill tools pass to ReadFile:
// maxSkillFileBytes plus one, so an oversize file is detected.
const skillFileReadLimit = int64(512*1024 + 1)

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

// responseDir extracts the "dir" field from a read_skill response. It is
// empty when the field is absent, as it is for personal skills.
func responseDir(t *testing.T, resp fantasy.ToolResponse) string {
	t.Helper()

	var payload struct {
		Dir string `json:"dir"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &payload))
	return payload.Dir
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
		assert.NotContains(t, idx, "read_skill_file")
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
				"Use read_skill_file to read supporting files referenced by a workspace or plugin skill.\n"+
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
		assert.Contains(t, idx, "read_skill_file")
		assert.NotContains(t, idx, "personal/personal-review")
		assert.NotContains(t, idx, "workspace/deep-review")
	})

	t.Run("CollidingNames", func(t *testing.T) {
		t.Parallel()

		resolved := skillspkg.MergeSkills(
			[]skillspkg.Skill{{Name: "review", Description: "Personal", Source: skillspkg.SourcePersonal}},
			[]skillspkg.Skill{{Name: "review", Description: "Workspace", Source: skillspkg.SourceWorkspace}},
			nil,
		)
		idx := chattool.FormatResolvedSkillIndex(resolved)
		assert.Contains(t, idx, "- personal/review: Personal")
		assert.Contains(t, idx, "- workspace/review: Workspace")
		assert.Contains(t, idx, "pass that qualified alias to read_skill")
		assert.NotContains(t, idx, "plugin/pluginname/name")
	})

	t.Run("PluginSkillLabelAndHint", func(t *testing.T) {
		t.Parallel()

		resolved := []skillspkg.ResolvedSkill{{
			Skill: skillspkg.Skill{
				Name:        "deploy",
				Description: "Deploy via acme",
				Source:      skillspkg.SourcePlugin,
				PluginName:  "acme",
			},
			Alias: "deploy",
		}}
		assert.Equal(t,
			"<available-skills>\n"+
				"Use read_skill to load a skill's full instructions before following them.\n"+
				"Use read_skill_file to read supporting files referenced by a workspace or plugin skill.\n"+
				"\n"+
				"- deploy (plugin: acme): Deploy via acme\n"+
				"</available-skills>",
			chattool.FormatResolvedSkillIndex(resolved),
		)
	})

	t.Run("CollidingPluginNames", func(t *testing.T) {
		t.Parallel()

		resolved := skillspkg.MergeSkills(
			nil,
			[]skillspkg.Skill{{Name: "deploy", Description: "Workspace"}},
			[]skillspkg.Skill{
				{Name: "deploy", Description: "Acme", PluginName: "acme"},
				{Name: "deploy", Description: "Beta", PluginName: "beta"},
			},
		)
		idx := chattool.FormatResolvedSkillIndex(resolved)
		assert.Contains(t, idx, "- workspace/deploy: Workspace")
		assert.Contains(t, idx, "- plugin/acme/deploy (plugin: acme): Acme")
		assert.Contains(t, idx, "- plugin/beta/deploy (plugin: beta): Beta")
		assert.Contains(t, idx, "When a skill is listed as personal/name, workspace/name, or plugin/pluginname/name, pass that qualified alias to read_skill.")
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
			skillFileReadLimit,
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
			skillFileReadLimit,
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

	t.Run("PinnedBodyFromMeta", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		// Meta carries the pushed SKILL.md, so the body is served from the
		// pin: the conn must never be asked to ReadFile the SKILL.md. The
		// supporting-file list is still a live, best-effort LS.
		skills := []chattool.SkillMeta{{
			Name:        "my-skill",
			Description: "test",
			Dir:         "/work/.agents/skills/my-skill",
			Meta:        []byte(validSkillMD("my-skill", "test")),
		}}

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{
				Contents: []workspacesdk.LSFile{
					{Name: "SKILL.md"},
					{Name: "helper.md"},
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
		assert.Contains(t, resp.Content, "helper.md")
		// Workspace skills expose the absolute skill directory so the
		// model can reach supporting files with read_file/execute.
		assert.Equal(t, "/work/.agents/skills/my-skill", responseDir(t, resp))
	})

	t.Run("PinnedBodyServedWhenWorkspaceUnreachable", func(t *testing.T) {
		t.Parallel()

		// With the body pinned, an unreachable workspace must not block
		// read_skill: the body is returned and the file list degrades to empty.
		skills := []chattool.SkillMeta{{
			Name: "my-skill",
			Dir:  "/work/.agents/skills/my-skill",
			Meta: []byte(validSkillMD("my-skill", "test")),
		}}

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return nil, xerrors.New("workspace is stopped")
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
		assert.Contains(t, resp.Content, `"files":[]`)
		// The dir comes from the pinned SkillMeta, so it is still
		// returned even when the workspace is unreachable and the file
		// list degrades to empty.
		assert.Equal(t, "/work/.agents/skills/my-skill", responseDir(t, resp))
	})

	t.Run("PersonalSkill", func(t *testing.T) {
		t.Parallel()

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			PersonalSkills: personalSkillsForTest("my-skill"),
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
		// Personal skills are database-backed and have no directory.
		assert.Empty(t, responseDir(t, resp))
		assert.NotContains(t, resp.Content, `"dir"`)
	})

	t.Run("PersonalQualifiedAliasPreservesAlias", func(t *testing.T) {
		t.Parallel()

		var loadedName string
		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			PersonalSkills: personalSkillsForTest("my-skill"),
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
			Meta:        []byte(validSkillMD("my-skill", "test")),
		}}

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{}, nil,
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
			Input: `{"name":"workspace/my-skill"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Equal(t, "workspace/my-skill", responseName(t, resp))
		assert.Contains(t, resp.Content, "Do the thing.")
		assert.Equal(t, "/work/.agents/skills/my-skill", responseDir(t, resp))
	})

	t.Run("CollisionAliasRoundTrip", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)

		workspaceSkills := []chattool.SkillMeta{{
			Name:        "deploy",
			Description: "workspace deploy",
			Dir:         "/work/.agents/skills/deploy",
			Meta:        []byte(validSkillMD("deploy", "workspace deploy")),
		}}

		conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
			workspacesdk.LSResponse{}, nil,
		)

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return conn, nil
			},
			GetSkills:      func() []chattool.SkillMeta { return workspaceSkills },
			PersonalSkills: personalSkillsForTest("deploy"),
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
		assert.Equal(t, "/work/.agents/skills/deploy", responseDir(t, workspaceResp))

		personalResp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-2",
			Name:  "read_skill",
			Input: `{"name":"personal/deploy"}`,
		})
		require.NoError(t, err)
		assert.False(t, personalResp.IsError)
		personalName := responseName(t, personalResp)
		assert.Equal(t, "personal/deploy", personalName)
		assert.Contains(t, personalResp.Content, "Personal deploy instructions.")

		bareResp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-3",
			Name:  "read_skill",
			Input: `{"name":"deploy"}`,
		})
		require.NoError(t, err)
		assert.True(t, bareResp.IsError)
		assert.Contains(t, bareResp.Content, "skill lookup is ambiguous")
	})

	t.Run("MissingPersonalSkill", func(t *testing.T) {
		t.Parallel()

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			PersonalSkills: personalSkillsForTest("missing-skill"),
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

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			PersonalSkills: personalSkillsForTest("my-skill"),
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
		assert.Contains(t, resp.Content, `failed to load personal skill "my-skill"`)
		assert.NotContains(t, resp.Content, "synthetic private storage failure")
	})

	t.Run("AmbiguousLookupSurfacesAliases", func(t *testing.T) {
		t.Parallel()

		tool := chattool.ReadSkill(ambiguousSkillOptionsForTest())

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":"deploy"}`,
		})

		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "skill lookup is ambiguous")
		assert.Contains(t, resp.Content, "personal/deploy")
		assert.Contains(t, resp.Content, "workspace/deploy")
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

	t.Run("NoSkillsConfigured", func(t *testing.T) {
		t.Parallel()

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":"my-skill"}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, `skill "my-skill" not found`)
	})
}

func personalSkillsForTest(names ...string) []skillspkg.Skill {
	skills := make([]skillspkg.Skill, 0, len(names))
	for _, name := range names {
		skills = append(skills, skillspkg.Skill{Name: name, Description: "test", Source: skillspkg.SourcePersonal})
	}
	return skills
}

// ambiguousSkillOptionsForTest pins a workspace "deploy" alongside a
// personal "deploy", so the bare name is ambiguous.
func ambiguousSkillOptionsForTest() chattool.ReadSkillOptions {
	pinned := []chattool.SkillMeta{{
		Name: "deploy",
		Dir:  "/work/.agents/skills/deploy",
		Meta: []byte(validSkillMD("deploy", "test")),
	}}
	return chattool.ReadSkillOptions{
		GetSkills:      func() []chattool.SkillMeta { return pinned },
		PersonalSkills: personalSkillsForTest("deploy"),
	}
}

// TestReadSkillToolPluginSource covers read_skill and read_skill_file for
// skills shipped inside Agent Plugins, pinned alongside a same-named
// workspace skill.
func TestReadSkillToolPluginSource(t *testing.T) {
	t.Parallel()

	// Rows are in pinned order (source ASC), so plugin rows come first.
	acmeDeploy := chattool.SkillMeta{
		Name:        "deploy",
		Description: "acme deploy",
		PluginName:  "acme",
		Dir:         "/work/.agents/plugins/acme/skills/deploy",
		Meta:        []byte("---\nname: deploy\ndescription: acme deploy\n---\n\nAcme body.\n"),
	}
	betaDeploy := chattool.SkillMeta{
		Name:        "deploy",
		Description: "beta deploy",
		PluginName:  "beta",
		Dir:         "/work/.agents/plugins/beta/skills/deploy",
		Meta:        []byte("---\nname: deploy\ndescription: beta deploy\n---\n\nBeta body.\n"),
	}
	workspaceDeploy := chattool.SkillMeta{
		Name:        "deploy",
		Description: "workspace deploy",
		Dir:         "/work/.agents/skills/deploy",
		Meta:        []byte("---\nname: deploy\ndescription: workspace deploy\n---\n\nWorkspace body.\n"),
	}
	pinned := []chattool.SkillMeta{acmeDeploy, betaDeploy, workspaceDeploy}

	t.Run("SameNameAcrossPluginsServesDistinctBodies", func(t *testing.T) {
		t.Parallel()

		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return nil, xerrors.New("workspace is stopped")
			},
			GetSkills: func() []chattool.SkillMeta { return pinned },
		})

		read := func(alias string) fantasy.ToolResponse {
			resp, err := tool.Run(context.Background(), fantasy.ToolCall{
				ID:    "call-" + alias,
				Name:  "read_skill",
				Input: `{"name":"` + alias + `"}`,
			})
			require.NoError(t, err)
			require.False(t, resp.IsError, resp.Content)
			return resp
		}

		acme := read("plugin/acme/deploy")
		assert.Equal(t, "plugin/acme/deploy", responseName(t, acme))
		assert.Contains(t, acme.Content, "Acme body.")
		assert.NotContains(t, acme.Content, "Workspace body.")
		assert.Equal(t, "/work/.agents/plugins/acme/skills/deploy", responseDir(t, acme))

		beta := read("plugin/beta/deploy")
		assert.Contains(t, beta.Content, "Beta body.")
		assert.Equal(t, "/work/.agents/plugins/beta/skills/deploy", responseDir(t, beta))

		workspace := read("workspace/deploy")
		assert.Contains(t, workspace.Content, "Workspace body.")
		assert.Equal(t, "/work/.agents/skills/deploy", responseDir(t, workspace))

		ambiguous, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-bare",
			Name:  "read_skill",
			Input: `{"name":"deploy"}`,
		})
		require.NoError(t, err)
		assert.True(t, ambiguous.IsError)
		assert.Contains(t, ambiguous.Content, "plugin/acme/deploy")
		assert.Contains(t, ambiguous.Content, "plugin/beta/deploy")
	})

	t.Run("BareAliasForUniquePluginSkill", func(t *testing.T) {
		t.Parallel()

		onlyPlugin := []chattool.SkillMeta{acmeDeploy}
		tool := chattool.ReadSkill(chattool.ReadSkillOptions{
			GetSkills: func() []chattool.SkillMeta { return onlyPlugin },
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill",
			Input: `{"name":"deploy"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Contains(t, resp.Content, "Acme body.")
	})

	t.Run("ReadSkillFileUsesPluginSkillDir", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		conn := agentconnmock.NewMockAgentConn(ctrl)
		conn.EXPECT().ReadFile(
			gomock.Any(),
			"/work/.agents/plugins/beta/skills/deploy/roles/reviewer.md",
			int64(0),
			skillFileReadLimit,
		).Return(
			io.NopCloser(strings.NewReader("beta reviewer guide")),
			"text/markdown",
			nil,
		)

		tool := chattool.ReadSkillFile(chattool.ReadSkillOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return conn, nil
			},
			GetSkills: func() []chattool.SkillMeta { return pinned },
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill_file",
			Input: `{"name":"plugin/beta/deploy","path":"roles/reviewer.md"}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError, resp.Content)
		assert.Contains(t, resp.Content, "beta reviewer guide")
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
			skillFileReadLimit,
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
			PersonalSkills: personalSkillsForTest("my-skill"),
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

	t.Run("AmbiguousLookupSurfacesAliases", func(t *testing.T) {
		t.Parallel()

		tool := chattool.ReadSkillFile(ambiguousSkillOptionsForTest())

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_skill_file",
			Input: `{"name":"deploy","path":"helper.md"}`,
		})

		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "skill lookup is ambiguous")
		assert.Contains(t, resp.Content, "personal/deploy")
		assert.Contains(t, resp.Content, "workspace/deploy")
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
