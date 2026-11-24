package skills_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
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
		name                                       string
		personal, organization, project, workspace []skills.Skill
		want                                       []skills.ResolvedSkill
	}{
		{
			name:         "NonCollidingSkillsUseBareAliasesSortedByName",
			personal:     []skills.Skill{{Name: "personal-skill"}},
			organization: []skills.Skill{{Name: "org-skill"}},
			project:      []skills.Skill{{Name: "project-skill"}},
			workspace:    []skills.Skill{{Name: "workspace-skill"}},
			want: []skills.ResolvedSkill{
				resolved("org-skill", skills.SourceOrganization, "org-skill", ""),
				resolved("personal-skill", skills.SourcePersonal, "personal-skill", ""),
				resolved("project-skill", skills.SourceProject, "project-skill", ""),
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
			project:      shared,
			workspace:    shared,
			want: []skills.ResolvedSkill{
				resolved("personal/shared", skills.SourcePersonal, "shared", "Shared"),
				resolved("org/shared", skills.SourceOrganization, "shared", "Shared"),
				resolved("project/shared", skills.SourceProject, "shared", "Shared"),
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
			require.Equal(t, tc.want, skills.MergeSkills(
				skills.SourceSkills{Source: skills.SourcePersonal, Skills: tc.personal},
				skills.SourceSkills{Source: skills.SourceOrganization, Skills: tc.organization},
				skills.SourceSkills{Source: skills.SourceProject, Skills: tc.project},
				skills.SourceSkills{Source: skills.SourceWorkspace, Skills: tc.workspace},
			))
		})
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()

	shared := []skills.Skill{{Name: "shared"}}
	merged := skills.MergeSkills(
		skills.SourceSkills{Source: skills.SourcePersonal, Skills: append([]skills.Skill{{Name: "personal-skill"}}, shared...)},
		skills.SourceSkills{Source: skills.SourceOrganization, Skills: append([]skills.Skill{{Name: "org-skill"}}, shared...)},
		skills.SourceSkills{Source: skills.SourceWorkspace, Skills: []skills.Skill{{Name: "workspace-skill"}}},
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

func TestWithModelInvocable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		raw            string
		modelInvocable bool
		// want is the returned content. Empty means the frontmatter cannot
		// be edited, so raw comes back with ok false.
		want string
	}{
		{
			name:           "AgreeingContentIsUnchanged",
			raw:            "---\nname: my-skill\n---\nBody.\n",
			modelInvocable: true,
			want:           "---\nname: my-skill\n---\nBody.\n",
		},
		{
			name: "AddsKey",
			raw:  "---\nname: my-skill\ndescription: Does a thing\n---\nBody.\n",
			want: "---\ndisable-model-invocation: true\nname: my-skill\ndescription: Does a thing\n---\nBody.\n",
		},
		{
			name: "AddsKeyWithCRLFAndBOM",
			raw:  "\xef\xbb\xbf---\r\nname: my-skill\r\n---\r\nBody.\r\n",
			want: "\xef\xbb\xbf---\r\ndisable-model-invocation: true\r\nname: my-skill\r\n---\r\nBody.\r\n",
		},
		{
			name: "AddsKeyWithFirstKeyIndentation",
			raw:  "---\n# Skill.\n  name: my-skill\n  description: Does a thing\n---\nBody.\n",
			want: "---\n# Skill.\n  disable-model-invocation: true\n  name: my-skill\n  description: Does a thing\n---\nBody.\n",
		},
		{
			name:           "ReplacesTrue",
			raw:            "---\nname: my-skill\ndisable-model-invocation: true # Ask first.\ndescription: Does a thing\n---\nBody.\n",
			modelInvocable: true,
			want:           "---\nname: my-skill\ndisable-model-invocation: false # Ask first.\ndescription: Does a thing\n---\nBody.\n",
		},
		{
			name: "ReplacesFalse",
			raw:  "---\nname: my-skill\ndisable-model-invocation: false\n---\nBody.\n",
			want: "---\nname: my-skill\ndisable-model-invocation: true\n---\nBody.\n",
		},
		{
			// A second key would make the frontmatter unparsable.
			name:           "ReplacesValueOfQuotedKey",
			raw:            "---\nname: my-skill\n\"disable-model-invocation\" : true\n---\nBody.\n",
			modelInvocable: true,
			want:           "---\nname: my-skill\n\"disable-model-invocation\" : false\n---\nBody.\n",
		},
		{
			// Only YAML booleans count, so the quoted string reads as unset.
			name: "ReplacesQuotedString",
			raw:  "---\nname: my-skill\ndisable-model-invocation: \"true\"\n---\nBody.\n",
			want: "---\nname: my-skill\ndisable-model-invocation: true\n---\nBody.\n",
		},
		{
			// YAML counts columns in characters, not bytes, so the accented
			// letter before the key matters.
			name:           "ReplacesInFlowStyle",
			raw:            "---\n{name: my-skill, description: \"Caf\u00e9\", disable-model-invocation: true, license: MIT}\n---\nBody.\n",
			modelInvocable: true,
			want:           "---\n{name: my-skill, description: \"Caf\u00e9\", disable-model-invocation: false, license: MIT}\n---\nBody.\n",
		},
		{
			name: "AddsKeyInFlowStyle",
			raw:  "---\n{name: my-skill, description: Does a thing}\n---\nBody.\n",
			want: "---\n{disable-model-invocation: true, name: my-skill, description: Does a thing}\n---\nBody.\n",
		},
		{
			// A quoted value can wrap onto a line that looks like the key.
			name:           "WrappedValueIsKept",
			raw:            "---\nname: my-skill\ndescription: \"a\ndisable-model-invocation: b\nc\"\ndisable-model-invocation: true\n---\nBody.\n",
			modelInvocable: true,
			want:           "---\nname: my-skill\ndescription: \"a\ndisable-model-invocation: b\nc\"\ndisable-model-invocation: false\n---\nBody.\n",
		},
		{
			// YAML ends a line at U+2028, so the key is on its fourth line.
			name:           "UnicodeLineBreakBeforeKey",
			raw:            "---\ndescription: \"a\u2028b\"\nname: my-skill\ndisable-model-invocation: true\n---\nBody.\n",
			modelInvocable: true,
			want:           "---\ndescription: \"a\u2028b\"\nname: my-skill\ndisable-model-invocation: false\n---\nBody.\n",
		},
		{
			// The value comes from a merge, so there is no key to replace.
			// A key in the mapping itself overrides the merged one.
			name:           "MergedKeyIsOverridden",
			raw:            "---\nbase: &base {disable-model-invocation: true}\n<<: *base\nname: my-skill\n---\nBody.\n",
			modelInvocable: true,
			want:           "---\ndisable-model-invocation: false\nbase: &base {disable-model-invocation: true}\n<<: *base\nname: my-skill\n---\nBody.\n",
		},
		{
			name: "ListValueCannotBeEdited",
			raw:  "---\nname: my-skill\ndisable-model-invocation:\n  - nested\n---\nBody.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			content, ok := skills.WithModelInvocable(tt.raw, tt.modelInvocable)
			if tt.want == "" {
				require.False(t, ok)
				require.Equal(t, tt.raw, content)
				return
			}
			require.True(t, ok)
			require.Equal(t, tt.want, content)

			// The edit changes nothing but the setting.
			require.Equal(t, !tt.modelInvocable, workspacesdk.SkillDisablesModelInvocation(content))
			before, err := skills.ParsePersonalSkillMarkdown([]byte(tt.raw))
			require.NoError(t, err)
			after, err := skills.ParsePersonalSkillMarkdown([]byte(content))
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
