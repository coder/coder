package autostart_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scaletest/autostart"
)

func TestRunResult(t *testing.T) {
	t.Parallel()

	configTime := time.Now().UTC()
	scheduledTime := configTime.Add(2 * time.Minute)
	completionTime := scheduledTime.Add(30 * time.Second)

	result := autostart.RunResult{
		WorkspaceID:    uuid.New(),
		WorkspaceName:  "test-workspace",
		ConfigTime:     configTime,
		ScheduledTime:  scheduledTime,
		CompletionTime: completionTime,
		Success:        true,
	}

	// Test end-to-end latency.
	endToEnd := result.EndToEndLatency()
	expectedEndToEnd := 2*time.Minute + 30*time.Second
	require.Equal(t, expectedEndToEnd, endToEnd)

	// Test trigger to completion latency.
	triggerToCompletion := result.TriggerToCompletionLatency()
	expectedTriggerToCompletion := 30 * time.Second
	require.Equal(t, expectedTriggerToCompletion, triggerToCompletion)
}

func TestRunResults(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	runs := []autostart.RunResult{
		{
			WorkspaceID:    uuid.New(),
			WorkspaceName:  "workspace-1",
			ConfigTime:     now,
			ScheduledTime:  now.Add(1 * time.Minute),
			CompletionTime: now.Add(1*time.Minute + 10*time.Second),
			Success:        true,
		},
		{
			WorkspaceID:    uuid.New(),
			WorkspaceName:  "workspace-2",
			ConfigTime:     now,
			ScheduledTime:  now.Add(1 * time.Minute),
			CompletionTime: now.Add(1*time.Minute + 20*time.Second),
			Success:        true,
		},
		{
			WorkspaceID:    uuid.New(),
			WorkspaceName:  "workspace-3",
			ConfigTime:     now,
			ScheduledTime:  now.Add(1 * time.Minute),
			CompletionTime: now.Add(1*time.Minute + 30*time.Second),
			Success:        true,
		},
		{
			WorkspaceID:   uuid.New(),
			WorkspaceName: "workspace-4",
			Success:       false,
			Error:         "build failed",
		},
	}

	results := autostart.NewRunResults(runs)

	require.Equal(t, 4, results.TotalRuns)
	require.Equal(t, 3, results.SuccessfulRuns)
	require.Equal(t, 1, results.FailedRuns)

	// Verify percentiles are calculated correctly.
	// P50 should be the middle value (20s).
	require.Equal(t, 20*time.Second, results.TriggerToCompletionP50)
	// With 3 values, P95 is at index int((3-1)*0.95) = 1, which is 20s.
	require.Equal(t, 20*time.Second, results.TriggerToCompletionP95)
	// P99 is also at index int((3-1)*0.99) = 1, which is 20s.
	require.Equal(t, 20*time.Second, results.TriggerToCompletionP99)

	// End-to-end latencies should include the 1 minute delay.
	require.Equal(t, 1*time.Minute+20*time.Second, results.EndToEndLatencyP50)
	require.Equal(t, 1*time.Minute+20*time.Second, results.EndToEndLatencyP95)
	require.Equal(t, 1*time.Minute+20*time.Second, results.EndToEndLatencyP99)
}
