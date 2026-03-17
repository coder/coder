package chat_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/scaletest/chat"
	"github.com/coder/coder/v2/scaletest/harness"
)

func TestNewSummary(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	modelConfigID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	startedAt := time.Date(2026, time.March, 17, 4, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(3 * time.Minute)
	followUpReleasedAt := startedAt.Add(90 * time.Second)

	summary := chat.NewSummary(chat.SummaryConfig{
		RunID:              "run-123",
		WorkspaceID:        workspaceID,
		ModelConfigID:      &modelConfigID,
		Count:              25,
		Turns:              10,
		Prompt:             "Reply with one short sentence.",
		FollowUpPrompt:     "Continue.",
		FollowUpStartDelay: 45 * time.Second,
		LLMMockURL:         "http://127.0.0.1:6061/v1",
		OutputSpecs:        []string{"text", "json:/tmp/results.json"},
	}, harness.Results{
		TotalRuns: 25,
		TotalPass: 24,
		TotalFail: 1,
		Elapsed:   httpapi.Duration(3 * time.Minute),
		ElapsedMS: (3 * time.Minute).Milliseconds(),
	}, startedAt, completedAt, &followUpReleasedAt)

	require.Equal(t, "run-123", summary.RunID)
	require.Equal(t, workspaceID, summary.WorkspaceID)
	require.True(t, summary.FollowUpDelayEnabled)
	require.Equal(t, followUpReleasedAt, *summary.FollowUpPhaseReleasedAt)
	require.Equal(t, 25, summary.Results.TotalRuns)
	require.Equal(t, int64(45000), summary.FollowUpStartDelayMS)
	require.Len(t, summary.RawOutputSpecs, 2)
	require.NotEmpty(t, summary.PromptFingerprint)
	require.NotEmpty(t, summary.FollowUpPromptFingerprint)

	compactJSON, err := summary.CompactJSON()
	require.NoError(t, err)
	require.Contains(t, string(compactJSON), `"run_id":"run-123"`)
}
