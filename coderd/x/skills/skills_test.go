package skills_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/skills"
)

func TestParsePersonalSkillMarkdown(t *testing.T) {
	t.Parallel()

	t.Run("ValidWithDescription", func(t *testing.T) {
		t.Parallel()

		content, err := skills.ParsePersonalSkillMarkdown([]byte(
			"---\nname: my-skill\ndescription: Does a thing\n---\nUse this skill.\n",
		))

		require.NoError(t, err)
		require.Equal(t, "my-skill", content.Name)
		require.Equal(t, "Does a thing", content.Description)
		require.Equal(t, skills.SourcePersonal, content.Source)
		require.Equal(t, "Use this skill.", content.Body)
	})

	t.Run("ValidWithFoldedDescription", func(t *testing.T) {
		t.Parallel()

		content, err := skills.ParsePersonalSkillMarkdown([]byte(strings.Join([]string{
			"---",
			"name: brainstorming",
			"description: >",
			"  Use before any creative work: features, components, functionality changes,",
			"  or behavior modifications. Turns ideas into approved designs through",
			"  collaborative dialog. Hard gate: no implementation action until the",
			"  design is presented and approved.",
			"---",
			"Use this skill.",
		}, "\n")))

		require.NoError(t, err)
		require.Equal(t, "brainstorming", content.Name)
		require.Equal(t, strings.Join([]string{
			"Use before any creative work: features, components, functionality changes,",
			"or behavior modifications. Turns ideas into approved designs through",
			"collaborative dialog. Hard gate: no implementation action until the",
			"design is presented and approved.",
		}, " "), content.Description)
		require.Equal(t, skills.SourcePersonal, content.Source)
		require.Equal(t, "Use this skill.", content.Body)
	})

	t.Run("ValidWithoutDescription", func(t *testing.T) {
		t.Parallel()

		content, err := skills.ParsePersonalSkillMarkdown([]byte(
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

		_, err := skills.ParsePersonalSkillMarkdown([]byte("name: my-skill\n---\nBody.\n"))

		require.ErrorContains(t, err, "missing opening frontmatter delimiter")
	})

	t.Run("MissingClosingDelimiter", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ParsePersonalSkillMarkdown([]byte("---\nname: my-skill\nBody.\n"))

		require.ErrorContains(t, err, "missing closing frontmatter delimiter")
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ParsePersonalSkillMarkdown([]byte(
			"---\ndescription: No name\n---\nBody.\n",
		))

		require.ErrorIs(t, err, skills.ErrInvalidSkillName)
		require.ErrorContains(t, err, "frontmatter must contain a 'name' field")
	})

	t.Run("NonStringName", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ParsePersonalSkillMarkdown([]byte(
			"---\nname: null\n---\nBody.\n",
		))

		require.ErrorIs(t, err, skills.ErrInvalidSkillName)
	})

	t.Run("NonKebabCaseName", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ParsePersonalSkillMarkdown([]byte(
			"---\nname: Not_Kebab\n---\nBody.\n",
		))

		require.ErrorIs(t, err, skills.ErrInvalidSkillName)
		require.ErrorContains(t, err, "Not_Kebab")
	})

	t.Run("NameTooLong", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ParsePersonalSkillMarkdown([]byte(personalSkillMarkdownForTest(
			strings.Repeat("a", skills.MaxPersonalSkillNameBytes+1),
			"Too long",
			"Body.",
		)))

		require.ErrorIs(t, err, skills.ErrInvalidSkillName)
		require.ErrorContains(t, err, "maximum is 256 bytes")
	})

	t.Run("DescriptionTooLong", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ParsePersonalSkillMarkdown([]byte(personalSkillMarkdownForTest(
			"my-skill",
			strings.Repeat("a", skills.MaxPersonalSkillDescriptionBytes+1),
			"Body.",
		)))

		require.ErrorIs(t, err, skills.ErrSkillDescriptionTooLarge)
		require.ErrorContains(t, err, "maximum is 4096 bytes")
	})

	t.Run("EmptyBody", func(t *testing.T) {
		t.Parallel()

		_, err := skills.ParsePersonalSkillMarkdown([]byte(
			"---\nname: my-skill\n---\n\n",
		))

		require.ErrorIs(t, err, skills.ErrSkillBodyRequired)
		require.ErrorContains(t, err, "my-skill")
	})

	t.Run("OversizedContent", func(t *testing.T) {
		t.Parallel()

		raw := []byte(strings.Repeat("a", skills.MaxPersonalSkillSizeBytes+1))
		_, err := skills.ParsePersonalSkillMarkdown(raw)

		require.ErrorIs(t, err, skills.ErrSkillTooLarge)
	})
}

func personalSkillMarkdownForTest(name string, description string, body string) string {
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

		personal, err := skills.Lookup(resolved, "personal/shared-skill")
		require.NoError(t, err)
		require.Equal(t, skills.SourcePersonal, personal.Source)
		require.Equal(t, "shared-skill", personal.Name)

		workspace, err := skills.Lookup(resolved, "workspace/shared-skill")
		require.NoError(t, err)
		require.Equal(t, skills.SourceWorkspace, workspace.Source)
		require.Equal(t, "shared-skill", workspace.Name)

		_, err = skills.Lookup(resolved, "shared-skill")
		require.ErrorIs(t, err, skills.ErrSkillAmbiguous)
		require.ErrorContains(t, err, "personal/shared-skill")
		require.ErrorContains(t, err, "workspace/shared-skill")
	})

	t.Run("DuplicatesWithinSourceKeepFirst", func(t *testing.T) {
		t.Parallel()

		resolved := skills.MergeSkills(
			[]skills.Skill{
				{Name: "duplicate-skill", Description: "First"},
				{Name: "duplicate-skill", Description: "Second"},
			},
			[]skills.Skill{
				{Name: "workspace-skill", Description: "Workspace"},
				{Name: "workspace-skill", Description: "Workspace duplicate"},
			},
		)

		require.Equal(t, []skills.ResolvedSkill{
			{
				Skill: skills.Skill{
					Name:        "duplicate-skill",
					Description: "First",
					Source:      skills.SourcePersonal,
				},
				Alias: "duplicate-skill",
			},
			{
				Skill: skills.Skill{
					Name:        "workspace-skill",
					Description: "Workspace",
					Source:      skills.SourceWorkspace,
				},
				Alias: "workspace-skill",
			},
		}, resolved)
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

	t.Run("BareNameFallsBackToSingleQualifiedAliasMatch", func(t *testing.T) {
		t.Parallel()

		resolved := []skills.ResolvedSkill{{
			Skill: skills.Skill{Name: "personal-skill", Source: skills.SourcePersonal},
			Alias: "personal/personal-skill",
		}}

		personal, err := skills.Lookup(resolved, "personal-skill")

		require.NoError(t, err)
		require.Equal(t, skills.SourcePersonal, personal.Source)
		require.Equal(t, "personal-skill", personal.Name)
	})

	t.Run("UnknownLookupReturnsNotFound", func(t *testing.T) {
		t.Parallel()

		_, err := skills.Lookup(nil, "missing-skill")

		require.ErrorIs(t, err, skills.ErrSkillNotFound)
		require.ErrorContains(t, err, "missing-skill")
	})
}
