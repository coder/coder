package prometheusmetrics

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/agentmetrics"
)

func TestFilterAcceptableAgentLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "template label is ignored",
			input:    []string{agentmetrics.LabelTemplateName},
			expected: []string{},
		},
		{
			name:     "all other labels are returned",
			input:    agentmetrics.LabelAll,
			expected: []string{agentmetrics.LabelAgentName, agentmetrics.LabelUsername, agentmetrics.LabelWorkspaceName},
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.expected, filterAcceptableAgentLabels(tc.input))
		})
	}
}
