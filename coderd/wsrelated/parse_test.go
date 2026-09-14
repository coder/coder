package wsrelated_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/wsrelated"
)

func TestParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  wsrelated.Config
	}{
		{
			name:  "Empty",
			input: "",
			want:  wsrelated.Config{},
		},
		{
			name:  "Template",
			input: "template",
			want:  wsrelated.Config{Template: true},
		},
		{
			name:  "LatestBuild",
			input: "latest_build",
			want:  wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{}},
		},
		{
			name:  "JobSelectsAncestor",
			input: "latest_build.job",
			want:  wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{Job: &wsrelated.Job{}}},
		},
		{
			name:  "QueuePositionSelectsAncestors",
			input: "latest_build.job.queue_position",
			want:  wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{Job: &wsrelated.Job{QueuePosition: true}}},
		},
		{
			name:  "TemplateVersion",
			input: "latest_build.template_version",
			want:  wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{TemplateVersion: true}},
		},
		{
			name:  "Metadata",
			input: "latest_build.resources.metadata",
			want: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{
				Resources: &wsrelated.Resources{Metadata: true},
			}},
		},
		{
			name:  "AppStatusesDeep",
			input: "latest_build.resources.agents.apps.statuses",
			want: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{
				Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{Apps: &wsrelated.Apps{Statuses: true}}},
			}},
		},
		{
			name:  "WildcardJob",
			input: "latest_build.job.*",
			want:  wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{Job: &wsrelated.Job{QueuePosition: true}}},
		},
		{
			name:  "WildcardAgents",
			input: "latest_build.resources.agents.*",
			want: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{
				Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{
					Apps:       &wsrelated.Apps{Statuses: true},
					Scripts:    true,
					LogSources: true,
				}},
			}},
		},
		{
			name:  "WildcardLatestBuild",
			input: "latest_build.*",
			want:  wsrelated.Config{LatestBuild: new(wsrelated.AllLatestBuild())},
		},
		{
			name:  "WildcardOnLeaf",
			input: "template.*",
			want:  wsrelated.Config{Template: true},
		},
		{
			name:  "RootWildcard",
			input: "*",
			want:  wsrelated.All(),
		},
		{
			name:  "MultiplePaths",
			input: "template,latest_build.template_version",
			want: wsrelated.Config{
				Template:    true,
				LatestBuild: &wsrelated.LatestBuild{TemplateVersion: true},
			},
		},
		{
			name:  "MergedSiblings",
			input: "latest_build.job,latest_build.resources.metadata",
			want: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{
				Job:       &wsrelated.Job{},
				Resources: &wsrelated.Resources{Metadata: true},
			}},
		},
		{
			name:  "OverlappingAncestorAndChild",
			input: "latest_build,latest_build.job",
			want:  wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{Job: &wsrelated.Job{}}},
		},
		{
			name:  "WhitespaceAndTrailingComma",
			input: " template , latest_build , ",
			want: wsrelated.Config{
				Template:    true,
				LatestBuild: &wsrelated.LatestBuild{},
			},
		},
		{
			name:  "WildcardSubtreeCoversExplicitChild",
			input: "latest_build.resources.*,latest_build.resources.agents.scripts",
			want: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{
				Resources: &wsrelated.Resources{
					Metadata: true,
					Agents: &wsrelated.Agents{
						Apps:       &wsrelated.Apps{Statuses: true},
						Scripts:    true,
						LogSources: true,
					},
				},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := wsrelated.Parse(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{name: "UnknownTopLevel", input: "bogus"},
		{name: "DoubleComma", input: "template,,template"},
		{name: "LeadingComma", input: ",template"},
		{name: "WhitespaceOnlyEntries", input: ", ,"},
		{name: "UnknownChild", input: "latest_build.bogus"},
		{name: "PathBeyondLeaf", input: "latest_build.job.queue_position.bogus"},
		{name: "MidWildcard", input: "latest_build.*.agents"},
		{name: "EmptySegment", input: "latest_build..job"},
		{name: "LeadingWildcardSegment", input: "*.template"},
		{name: "UnknownWithValidSibling", input: "template,bogus"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := wsrelated.Parse(tc.input)
			require.Error(t, err)
		})
	}
}
