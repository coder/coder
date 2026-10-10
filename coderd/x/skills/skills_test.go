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

	resolved := func(alias string, source skills.Source, name, description string) skills.ResolvedSkill {
		return skills.ResolvedSkill{
			Skill: skills.Skill{Name: name, Description: description, Source: source},
			Alias: alias,
		}
	}
	shared := []skills.Skill{{Name: "shared", Description: "Shared"}}

	for _, tc := range []struct {
		name                              string
		personal, organization, workspace []skills.Skill
		want                              []skills.ResolvedSkill
	}{
		{
			name:         "NonCollidingSkillsUseBareAliasesSortedByName",
			personal:     []skills.Skill{{Name: "personal-skill"}},
			organization: []skills.Skill{{Name: "org-skill"}},
			workspace:    []skills.Skill{{Name: "workspace-skill"}},
			want: []skills.ResolvedSkill{
				resolved("org-skill", skills.SourceOrganization, "org-skill", ""),
				resolved("personal-skill", skills.SourcePersonal, "personal-skill", ""),
				resolved("workspace-skill", skills.SourceWorkspace, "workspace-skill", ""),
			},
		},
		{
			name:      "PersonalAndWorkspaceCollide",
			personal:  shared,
			workspace: shared,
			want: []skills.ResolvedSkill{
				resolved("personal/shared", skills.SourcePersonal, "shared", "Shared"),
				resolved("workspace/shared", skills.SourceWorkspace, "shared", "Shared"),
			},
		},
		{
			name:         "PersonalAndOrganizationCollide",
			personal:     shared,
			organization: shared,
			want: []skills.ResolvedSkill{
				resolved("personal/shared", skills.SourcePersonal, "shared", "Shared"),
				resolved("org/shared", skills.SourceOrganization, "shared", "Shared"),
			},
		},
		{
			name:         "OrganizationAndWorkspaceCollide",
			organization: shared,
			workspace:    shared,
			want: []skills.ResolvedSkill{
				resolved("org/shared", skills.SourceOrganization, "shared", "Shared"),
				resolved("workspace/shared", skills.SourceWorkspace, "shared", "Shared"),
			},
		},
		{
			name:         "AllSourcesCollide",
			personal:     shared,
			organization: shared,
			workspace:    shared,
			want: []skills.ResolvedSkill{
				resolved("personal/shared", skills.SourcePersonal, "shared", "Shared"),
				resolved("org/shared", skills.SourceOrganization, "shared", "Shared"),
				resolved("workspace/shared", skills.SourceWorkspace, "shared", "Shared"),
			},
		},
		{
			name: "DuplicatesWithinSourceKeepFirst",
			personal: []skills.Skill{
				{Name: "duplicate-skill", Description: "First"},
				{Name: "duplicate-skill", Description: "Second"},
			},
			want: []skills.ResolvedSkill{
				resolved("duplicate-skill", skills.SourcePersonal, "duplicate-skill", "First"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, skills.MergeSkills(tc.personal, tc.organization, tc.workspace))
		})
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()

	shared := []skills.Skill{{Name: "shared"}}
	merged := skills.MergeSkills(
		append([]skills.Skill{{Name: "personal-skill"}}, shared...),
		append([]skills.Skill{{Name: "org-skill"}}, shared...),
		[]skills.Skill{{Name: "workspace-skill"}},
	)

	for _, tc := range []struct {
		name            string
		resolved        []skills.ResolvedSkill
		lookup          string
		wantSource      skills.Source
		wantName        string
		wantErr         error
		wantErrContains []string
	}{
		{name: "BareNameWithoutCollision", resolved: merged, lookup: "workspace-skill", wantSource: skills.SourceWorkspace, wantName: "workspace-skill"},
		{name: "QualifiedPersonalWithoutCollision", resolved: merged, lookup: "personal/personal-skill", wantSource: skills.SourcePersonal, wantName: "personal-skill"},
		{name: "QualifiedOrganizationWithoutCollision", resolved: merged, lookup: "org/org-skill", wantSource: skills.SourceOrganization, wantName: "org-skill"},
		{name: "QualifiedWorkspaceWithoutCollision", resolved: merged, lookup: "workspace/workspace-skill", wantSource: skills.SourceWorkspace, wantName: "workspace-skill"},
		{name: "QualifiedOrganizationOnCollision", resolved: merged, lookup: "org/shared", wantSource: skills.SourceOrganization, wantName: "shared"},
		{
			name:            "BareNameOnCollisionIsAmbiguous",
			resolved:        merged,
			lookup:          "shared",
			wantErr:         skills.ErrSkillAmbiguous,
			wantErrContains: []string{"personal/shared", "org/shared"},
		},
		{
			name: "BareNameFallsBackToSingleQualifiedAliasMatch",
			resolved: []skills.ResolvedSkill{{
				Skill: skills.Skill{Name: "personal-skill", Source: skills.SourcePersonal},
				Alias: "personal/personal-skill",
			}},
			lookup:     "personal-skill",
			wantSource: skills.SourcePersonal,
			wantName:   "personal-skill",
		},
		{name: "UnknownLookupReturnsNotFound", lookup: "missing-skill", wantErr: skills.ErrSkillNotFound, wantErrContains: []string{"missing-skill"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := skills.Lookup(tc.resolved, tc.lookup)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, 1, strings.Count(err.Error(), tc.wantErr.Error()))
				for _, want := range tc.wantErrContains {
					require.ErrorContains(t, err, want)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantSource, got.Source)
			require.Equal(t, tc.wantName, got.Name)
		})
	}
}
