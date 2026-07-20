package templatebuilder_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/templatebuilder"
)

func TestExtractPrerequisites(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		readme   string
		expected string
	}{
		{
			name: "BothMarkers",
			readme: "# Title\n\n" +
				"<!-- prerequisites:start -->\n" +
				"## Prerequisites\n\n" +
				"Install Docker.\n" +
				"<!-- prerequisites:end -->\n\n" +
				"## Architecture\n",
			expected: "## Prerequisites\n\nInstall Docker.",
		},
		{
			name: "MultipleH2Sections",
			readme: "# Title\n\n" +
				"<!-- prerequisites:start -->\n" +
				"## Prerequisites\n\n" +
				"Auth stuff.\n\n" +
				"## Required permissions\n\n" +
				"Policy JSON.\n" +
				"<!-- prerequisites:end -->\n\n" +
				"## Architecture\n",
			expected: "## Prerequisites\n\nAuth stuff.\n\n## Required permissions\n\nPolicy JSON.",
		},
		{
			name:     "NoMarkers",
			readme:   "# Title\n\n## Prerequisites\n\nSome content.\n\n## Architecture\n",
			expected: "",
		},
		{
			name: "StartMarkerOnly",
			readme: "# Title\n\n" +
				"<!-- prerequisites:start -->\n" +
				"## Prerequisites\n\nContent.\n",
			expected: "",
		},
		{
			name: "EndMarkerOnly",
			readme: "# Title\n\n" +
				"## Prerequisites\n\nContent.\n" +
				"<!-- prerequisites:end -->\n",
			expected: "",
		},
		{
			name: "EmptyBetweenMarkers",
			readme: "<!-- prerequisites:start -->\n" +
				"<!-- prerequisites:end -->\n",
			expected: "",
		},
		{
			name: "NestedH3Headings",
			readme: "<!-- prerequisites:start -->\n" +
				"## Prerequisites\n\n" +
				"### Infrastructure\n\n" +
				"Docker socket required.\n\n" +
				"### Authentication\n\n" +
				"Use kubeconfig.\n" +
				"<!-- prerequisites:end -->\n",
			expected: "## Prerequisites\n\n### Infrastructure\n\nDocker socket required.\n\n### Authentication\n\nUse kubeconfig.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := templatebuilder.ExtractPrerequisites(tt.readme)
			require.Equal(t, tt.expected, result)
		})
	}
}
