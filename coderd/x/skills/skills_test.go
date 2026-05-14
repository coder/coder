package skills_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/skills"
)

func TestParsePersonalSkillMarkdown(t *testing.T) {
	t.Parallel()

	t.Run("AcceptsNonKebabCaseName", func(t *testing.T) {
		t.Parallel()

		content, err := skills.ParsePersonalSkillMarkdown([]byte(
			"---\nname: NotKebab\ndescription: Parser accepts this\n---\nBody.\n",
		))

		require.NoError(t, err)
		require.Equal(t, "NotKebab", content.Name)
		require.Equal(t, "Parser accepts this", content.Description)
		require.Equal(t, "Body.", content.Body)
	})

	t.Run("AcceptsOversizedContent", func(t *testing.T) {
		t.Parallel()

		raw := []byte(userSkillMarkdownForTest(
			"oversized-skill",
			"Parser accepts oversized content.",
			strings.Repeat("a", skills.MaxPersonalSkillSizeBytes),
		))
		require.Greater(t, len(raw), skills.MaxPersonalSkillSizeBytes)

		content, err := skills.ParsePersonalSkillMarkdown(raw)

		require.NoError(t, err)
		require.Equal(t, "oversized-skill", content.Name)
		require.Len(t, content.Body, skills.MaxPersonalSkillSizeBytes)
	})
}

func TestValidatePersonalSkillMarkdown(t *testing.T) {
	t.Parallel()

	t.Run("ValidWithDescription", func(t *testing.T) {
		t.Parallel()

		content, err := skills.ValidatePersonalSkillMarkdown([]byte(
			"---\nname: my-skill\ndescription: Does a thing\n---\nUse this skill.\n",
		))

		require.NoError(t, err)
		require.Equal(t, "my-skill", content.Name)
		require.Equal(t, "Does a thing", content.Description)
		require.Equal(t, skills.SourcePersonal, content.Source)
		require.Equal(t, "Use this skill.", content.Body)
	})

	t.Run("ValidWithoutDescription", func(t *testing.T) {
		t.Parallel()

		content, err := skills.ValidatePersonalSkillMarkdown([]byte(
			"---\nname: my-skill\n---\nUse this skill.\n",
		))

		require.NoError(t, err)
		require.Equal(t, "my-skill", content.Name)
		require.Empty(t, content.Description)
		require.Equal(t, skills.SourcePersonal, content.Source)
		require.Equal(t, "Use this skill.", content.Body)
	})

	t.Run("MissingOpeningDelimiter", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ValidatePersonalSkillMarkdown([]byte("name: my-skill\n---\nBody.\n"))

		require.ErrorContains(t, err, "missing opening frontmatter delimiter")
	})

	t.Run("MissingClosingDelimiter", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ValidatePersonalSkillMarkdown([]byte("---\nname: my-skill\nBody.\n"))

		require.ErrorContains(t, err, "missing closing frontmatter delimiter")
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ValidatePersonalSkillMarkdown([]byte(
			"---\ndescription: No name\n---\nBody.\n",
		))

		require.ErrorIs(t, err, skills.ErrInvalidSkillName)
	})

	t.Run("NonKebabCaseName", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ValidatePersonalSkillMarkdown([]byte(
			"---\nname: Not_Kebab\n---\nBody.\n",
		))

		require.ErrorIs(t, err, skills.ErrInvalidSkillName)
		require.ErrorContains(t, err, "Not_Kebab")
	})

	t.Run("EmptyBody", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ValidatePersonalSkillMarkdown([]byte(
			"---\nname: my-skill\n---\n\n",
		))

		require.ErrorIs(t, err, skills.ErrSkillBodyRequired)
		require.ErrorContains(t, err, "my-skill")
	})

	t.Run("OversizedContent", func(t *testing.T) {
		t.Parallel()

		raw := []byte(strings.Repeat("a", skills.MaxPersonalSkillSizeBytes+1))
		_, err := skills.ValidatePersonalSkillMarkdown(raw)

		require.ErrorIs(t, err, skills.ErrSkillTooLarge)
	})
}

func userSkillMarkdownForTest(name string, description string, body string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body + "\n"
}

func TestMergeSkills(t *testing.T) {
	t.Parallel()

	t.Run("PersonalOnlyUsesBareAlias", func(t *testing.T) {
		t.Parallel()

		resolved := skills.MergeSkills(
			[]skills.Skill{{Name: "my-skill", Description: "Mine"}},
			nil,
		)

		require.Equal(t, []skills.ResolvedSkill{{
			Skill: skills.Skill{
				Name:        "my-skill",
				Description: "Mine",
				Source:      skills.SourcePersonal,
			},
			Alias: "my-skill",
		}}, resolved)
	})

	t.Run("WorkspaceOnlyUsesBareAlias", func(t *testing.T) {
		t.Parallel()

		resolved := skills.MergeSkills(
			nil,
			[]skills.Skill{{Name: "my-skill", Description: "Workspace"}},
		)

		require.Equal(t, []skills.ResolvedSkill{{
			Skill: skills.Skill{
				Name:        "my-skill",
				Description: "Workspace",
				Source:      skills.SourceWorkspace,
			},
			Alias: "my-skill",
		}}, resolved)
	})

	t.Run("NonCollidingSkillsUseBareAliases", func(t *testing.T) {
		t.Parallel()

		resolved := skills.MergeSkills(
			[]skills.Skill{{Name: "personal-skill"}},
			[]skills.Skill{{Name: "workspace-skill"}},
		)

		require.Equal(t, []skills.ResolvedSkill{
			{
				Skill: skills.Skill{
					Name:   "personal-skill",
					Source: skills.SourcePersonal,
				},
				Alias: "personal-skill",
			},
			{
				Skill: skills.Skill{
					Name:   "workspace-skill",
					Source: skills.SourceWorkspace,
				},
				Alias: "workspace-skill",
			},
		}, resolved)
	})

	t.Run("CollidingSkillsUseQualifiedAliases", func(t *testing.T) {
		t.Parallel()

		resolved := skills.MergeSkills(
			[]skills.Skill{{Name: "shared-skill", Description: "Mine"}},
			[]skills.Skill{{Name: "shared-skill", Description: "Workspace"}},
		)

		require.Equal(t, []skills.ResolvedSkill{
			{
				Skill: skills.Skill{
					Name:        "shared-skill",
					Description: "Mine",
					Source:      skills.SourcePersonal,
				},
				Alias: "personal/shared-skill",
			},
			{
				Skill: skills.Skill{
					Name:        "shared-skill",
					Description: "Workspace",
					Source:      skills.SourceWorkspace,
				},
				Alias: "workspace/shared-skill",
			},
		}, resolved)

		_, err := skills.Lookup(resolved, "shared-skill")
		require.ErrorIs(t, err, skills.ErrSkillNotFound)
	})
}

func TestLookup(t *testing.T) {
	t.Parallel()

	t.Run("BareNameOnNonCollidingSkill", func(t *testing.T) {
		t.Parallel()

		resolved := skills.MergeSkills(
			[]skills.Skill{{Name: "personal-skill"}},
			[]skills.Skill{{Name: "workspace-skill"}},
		)

		personal, err := skills.Lookup(resolved, "personal-skill")
		require.NoError(t, err)
		require.Equal(t, skills.SourcePersonal, personal.Source)
		require.Equal(t, "personal-skill", personal.Name)

		workspace, err := skills.Lookup(resolved, "workspace-skill")
		require.NoError(t, err)
		require.Equal(t, skills.SourceWorkspace, workspace.Source)
		require.Equal(t, "workspace-skill", workspace.Name)
	})

	t.Run("QualifiedAliasWorksWithoutCollision", func(t *testing.T) {
		t.Parallel()

		resolved := skills.MergeSkills(
			[]skills.Skill{{Name: "personal-skill"}},
			[]skills.Skill{{Name: "workspace-skill"}},
		)

		personal, err := skills.Lookup(resolved, "personal/personal-skill")
		require.NoError(t, err)
		require.Equal(t, skills.SourcePersonal, personal.Source)
		require.Equal(t, "personal-skill", personal.Name)

		workspace, err := skills.Lookup(resolved, "workspace/workspace-skill")
		require.NoError(t, err)
		require.Equal(t, skills.SourceWorkspace, workspace.Source)
		require.Equal(t, "workspace-skill", workspace.Name)
	})

	t.Run("UnknownLookupReturnsNotFound", func(t *testing.T) {
		t.Parallel()

		_, err := skills.Lookup(nil, "missing-skill")

		require.ErrorIs(t, err, skills.ErrSkillNotFound)
		require.True(t, errors.Is(err, skills.ErrSkillNotFound))
		require.ErrorContains(t, err, "missing-skill")
	})
}
