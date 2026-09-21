package wsrelated_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/wsrelated"
)

func TestConfigQueryParam(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		config wsrelated.Config
		want   string
	}{
		{
			name:   "Empty",
			config: wsrelated.Config{},
			want:   "",
		},
		{
			name:   "Template",
			config: wsrelated.Config{Template: true},
			want:   "template",
		},
		{
			name:   "LatestBuildOnly",
			config: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{}},
			want:   "latest_build",
		},
		{
			name:   "TemplateVersion",
			config: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{TemplateVersion: true}},
			want:   "latest_build.template_version",
		},
		{
			name:   "JobWithoutQueuePosition",
			config: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{Job: &wsrelated.Job{}}},
			want:   "latest_build.job",
		},
		{
			name:   "JobQueuePosition",
			config: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{Job: &wsrelated.Job{QueuePosition: true}}},
			want:   "latest_build.job.queue_position",
		},
		{
			name:   "ResourcesOnly",
			config: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{Resources: &wsrelated.Resources{}}},
			want:   "latest_build.resources",
		},
		{
			name:   "AgentsOnly",
			config: wsrelated.Config{LatestBuild: &wsrelated.LatestBuild{Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{}}}},
			want:   "latest_build.resources.agents",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.config.QueryParam())
		})
	}
}

// TestConfigQueryParamRoundTrip asserts that encoding a Config and parsing the
// result yields an equal Config.
func TestConfigQueryParamRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		config wsrelated.Config
	}{
		{name: "Empty", config: wsrelated.Config{}},
		{name: "All", config: wsrelated.All()},
		{name: "Template", config: wsrelated.Config{Template: true}},
		{
			name: "Mixed",
			config: wsrelated.Config{
				Template: true,
				LatestBuild: &wsrelated.LatestBuild{
					Job: &wsrelated.Job{},
					Resources: &wsrelated.Resources{
						Metadata: true,
						Agents:   &wsrelated.Agents{Scripts: true},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := wsrelated.Parse(tc.config.QueryParam())
			require.NoError(t, err)
			require.Equal(t, tc.config, got)
		})
	}
}
