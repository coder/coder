package frontmatter_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/frontmatter"
)

func TestParse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		readme   string
		wantMeta frontmatter.Frontmatter
		wantBody string
		wantErr  bool
	}{
		{
			name: "AllKnownKeys",
			readme: "---\n" +
				"display_name: Kubernetes (Deployment)\n" +
				"description: Provision Kubernetes Deployments as Coder workspaces\n" +
				"icon: ../../../site/static/icon/k8s.png\n" +
				"maintainer_github: coder\n" +
				"verified: true\n" +
				"tags: [kubernetes, container]\n" +
				"agent_description: Use this for Kubernetes-native workloads.\n" +
				"---\n" +
				"# Kubernetes\n\nBody text.\n",
			wantMeta: frontmatter.Frontmatter{
				DisplayName:      "Kubernetes (Deployment)",
				Description:      "Provision Kubernetes Deployments as Coder workspaces",
				Icon:             "../../../site/static/icon/k8s.png",
				MaintainerGithub: "coder",
				Verified:         true,
				Tags:             []string{"kubernetes", "container"},
				AgentDescription: "Use this for Kubernetes-native workloads.",
			},
			wantBody: "# Kubernetes\n\nBody text.",
		},
		{
			name:     "OnlyAgentDescription",
			readme:   "---\nagent_description: Just the agent key.\n---\n# Title\n",
			wantMeta: frontmatter.Frontmatter{AgentDescription: "Just the agent key."},
			wantBody: "# Title",
		},
		{
			name:     "UnknownKeysIgnored",
			readme:   "---\nagent_description: Hello\nfoo: bar\nnested:\n  a: b\n---\n# Title\n",
			wantMeta: frontmatter.Frontmatter{AgentDescription: "Hello"},
			wantBody: "# Title",
		},
		{
			name:     "TagsParsedAsList",
			readme:   "---\ntags:\n  - go\n  - docker\n---\n# Title\n",
			wantMeta: frontmatter.Frontmatter{Tags: []string{"go", "docker"}},
			wantBody: "# Title",
		},
		{
			name:     "CRLFTolerated",
			readme:   "---\r\nagent_description: windows line endings\r\n---\r\n# Title\r\n",
			wantMeta: frontmatter.Frontmatter{AgentDescription: "windows line endings"},
			wantBody: "# Title",
		},
		{
			name:     "LeadingBlankLinesTolerated",
			readme:   "\n\n---\nagent_description: leading blanks\n---\n# Title\n",
			wantMeta: frontmatter.Frontmatter{AgentDescription: "leading blanks"},
			wantBody: "# Title",
		},
		{
			name:    "NoFencesReturnsError",
			readme:  "# Just a heading\n\nNo frontmatter here.\n",
			wantErr: true,
		},
		{
			name:    "OnlyOneFenceReturnsError",
			readme:  "---\nagent_description: missing closing fence\n# Title\n",
			wantErr: true,
		},
		{
			name:    "MalformedYAMLReturnsError",
			readme:  "---\n\tthis: : : not valid yaml\n  - broken\n---\n# Title\n",
			wantErr: true,
		},
		{
			name:    "EmptyStringReturnsError",
			readme:  "",
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			meta, body, err := frontmatter.Parse(tc.readme)
			if tc.wantErr {
				require.Error(t, err)
				require.Equal(t, frontmatter.Frontmatter{}, meta)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantMeta, meta)
			require.Equal(t, tc.wantBody, body)
		})
	}
}

func TestParseDoesNotPanic(t *testing.T) {
	t.Parallel()

	// A grab-bag of hostile inputs must never panic.
	for _, in := range []string{
		"",
		"---",
		"------",
		"---\n---",
		"---\n---\n",
		"---\n\x00\x00\n---\nbody",
		strings.Repeat("---\n", 100),
	} {
		require.NotPanics(t, func() {
			_, _, _ = frontmatter.Parse(in)
		})
	}
}

func TestAgentDescription(t *testing.T) {
	t.Parallel()

	t.Run("Trimmed", func(t *testing.T) {
		t.Parallel()
		got := frontmatter.AgentDescription("---\nagent_description: \"  spaced  \"\n---\n# T\n")
		require.Equal(t, "spaced", got)
	})

	t.Run("EmptyWhenAbsent", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, frontmatter.AgentDescription("---\ndescription: short\n---\n# T\n"))
	})

	t.Run("EmptyWhenWhitespaceOnly", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, frontmatter.AgentDescription("---\nagent_description: \"   \"\n---\n# T\n"))
	})

	t.Run("EmptyOnMalformed", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, frontmatter.AgentDescription("no frontmatter at all"))
	})

	t.Run("TruncatedToMaxRunes", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("x", frontmatter.AgentDescriptionMaxRunes+500)
		got := frontmatter.AgentDescription("---\nagent_description: " + long + "\n---\n# T\n")
		gotRunes := []rune(got)
		require.Len(t, gotRunes, frontmatter.AgentDescriptionMaxRunes)
		require.Equal(t, '…', gotRunes[len(gotRunes)-1])
	})
}
