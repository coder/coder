package agentmetrics_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/agentmetrics"
)

func TestValidateAggregationLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		labels      []string
		expectedErr bool
	}{
		{
			name: "empty list is valid",
		},
		{
			name:   "single valid entry",
			labels: []string{agentmetrics.LabelTemplateName},
		},
		{
			name:   "multiple valid entries",
			labels: []string{agentmetrics.LabelTemplateName, agentmetrics.LabelUsername},
		},
		{
			name:   "repeated valid entries are not invalid",
			labels: []string{agentmetrics.LabelTemplateName, agentmetrics.LabelUsername, agentmetrics.LabelUsername, agentmetrics.LabelUsername},
		},
		{
			name:        "empty entry is invalid",
			labels:      []string{""},
			expectedErr: true,
		},
		{
			name:   "all valid entries",
			labels: agentmetrics.LabelAll,
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := agentmetrics.ValidateAggregationLabels(tc.labels)
			if tc.expectedErr {
				require.Error(t, err)
			}
		})
	}
}
