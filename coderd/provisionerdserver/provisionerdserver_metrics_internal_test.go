package provisionerdserver

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
)

func TestWorkspaceCreationOutcomesMetricLogic(t *testing.T) {
	t.Parallel()

	// Test the logic conditions for incrementing the metric.
	testCases := []struct {
		name            string
		buildNumber     int32
		isPrebuild      bool
		isClaim         bool
		jobSucceeded    bool
		shouldIncrement bool
		expectedStatus  string
		description     string
	}{
		{
			name:            "FirstStartBuildSuccess",
			buildNumber:     1,
			isPrebuild:      false,
			isClaim:         false,
			jobSucceeded:    true,
			shouldIncrement: true,
			expectedStatus:  "success",
			description:     "Should increment with success status for successful first build",
		},
		{
			name:            "FirstStartBuildFailure",
			buildNumber:     1,
			isPrebuild:      false,
			isClaim:         false,
			jobSucceeded:    false,
			shouldIncrement: true,
			expectedStatus:  "failure",
			description:     "Should increment with failure status when job fails",
		},
		{
			name:            "SecondBuildShouldNotIncrement",
			buildNumber:     2,
			isPrebuild:      false,
			isClaim:         false,
			jobSucceeded:    true,
			shouldIncrement: false,
			expectedStatus:  "success",
			description:     "Should not increment for subsequent builds",
		},
		{
			name:            "PrebuildShouldNotIncrement",
			buildNumber:     1,
			isPrebuild:      true,
			isClaim:         false,
			jobSucceeded:    true,
			shouldIncrement: false,
			expectedStatus:  "success",
			description:     "Should not increment for prebuild creation",
		},
		{
			name:            "PrebuildClaimShouldNotIncrement",
			buildNumber:     1,
			isPrebuild:      false,
			isClaim:         true,
			jobSucceeded:    true,
			shouldIncrement: false,
			expectedStatus:  "success",
			description:     "Should not increment for prebuild claims",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// Simulate the workspace data.
			orgName := "test-org"
			templateName := "test-template"
			presetName := ""

			// Create a test metrics object.
			testMetrics := NewMetrics(slogtest.Make(t, nil), prometheus.NewRegistry())

			initialValue := promtest.ToFloat64(testMetrics.workspaceCreationOutcomesTotal.WithLabelValues(
				orgName,
				templateName,
				presetName,
				tc.expectedStatus,
			))

			flags := WorkspaceTimingFlags{
				IsPrebuild:   tc.isPrebuild,
				IsClaim:      tc.isClaim,
				IsFirstBuild: tc.buildNumber == 1,
			}

			// Call the metric update function.
			testMetrics.UpdateWorkspaceCreationOutcomesMetric(
				ctx,
				flags,
				orgName,
				templateName,
				presetName,
				tc.jobSucceeded,
			)

			// Verify the metric.
			newValue := promtest.ToFloat64(testMetrics.workspaceCreationOutcomesTotal.WithLabelValues(
				orgName,
				templateName,
				presetName,
				tc.expectedStatus,
			))

			if tc.shouldIncrement {
				require.Equal(t, initialValue+1, newValue, tc.description)
			} else {
				require.Equal(t, initialValue, newValue, tc.description)
			}
		})
	}
}
