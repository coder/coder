package chatdebug_test

import (
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
)

func TestTruncateLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{name: "Empty", input: "", maxLen: 10, want: ""},
		{name: "WhitespaceOnly", input: "   \t\n  ", maxLen: 10, want: ""},
		{name: "ShortText", input: "hello world", maxLen: 20, want: "hello world"},
		{name: "ExactLength", input: "abcde", maxLen: 5, want: "abcde"},
		{name: "LongTextTruncated", input: "abcdefghij", maxLen: 5, want: "abcd…"},
		{name: "NegativeMaxLen", input: "hello", maxLen: -1, want: ""},
		{name: "ZeroMaxLen", input: "hello", maxLen: 0, want: ""},
		{name: "SingleRuneLimit", input: "hello", maxLen: 1, want: "…"},
		{name: "MultipleWhitespaceRuns", input: "  hello   world  \t again  ", maxLen: 100, want: "hello world again"},
		{name: "UnicodeRunes", input: "こんにちは世界", maxLen: 3, want: "こん…"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := chatdebug.TruncateLabel(tc.input, tc.maxLen)
			require.Equal(t, tc.want, got)
			require.LessOrEqual(t, utf8.RuneCountInString(got), max(tc.maxLen, 0))
		})
	}
}

func TestSeedSummary(t *testing.T) {
	t.Parallel()

	t.Run("NonEmptyLabel", func(t *testing.T) {
		t.Parallel()
		got := chatdebug.SeedSummary("hello world")
		require.Equal(t, map[string]any{"first_message": "hello world"}, got)
	})

	t.Run("EmptyLabel", func(t *testing.T) {
		t.Parallel()
		got := chatdebug.SeedSummary("")
		require.Nil(t, got)
	})
}

func TestService_AggregateRunSummary(t *testing.T) {
	t.Parallel()

	t.Run("NilRunID", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, uuid.Nil, nil)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("RecomputesHasErrorAndCompletedEndpointLabel", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		step1 := createTestStep(t, fixture, run.ID)
		_, err := fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
			ID:     step1.ID,
			ChatID: fixture.chat.ID,
			Status: chatdebug.StatusError,
			Attempts: []chatdebug.Attempt{{
				Number: 1,
				Status: "failed",
				Method: "POST",
				Path:   "/failed",
			}},
		})
		require.NoError(t, err)

		step2 := createTestStep(t, fixture, run.ID)
		_, err = fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
			ID:     step2.ID,
			ChatID: fixture.chat.ID,
			Status: chatdebug.StatusCompleted,
			Attempts: []chatdebug.Attempt{{
				Number: 1,
				Status: "completed",
				Method: "POST",
				Path:   "/v1/messages",
			}},
		})
		require.NoError(t, err)

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, nil)
		require.NoError(t, err)
		require.Equal(t, true, got["has_error"])
		require.Equal(t, "POST /v1/messages", got["endpoint_label"])
	})

	t.Run("MultipleStepsSumTokens", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		step1 := createTestStep(t, fixture, run.ID)
		updateTestStepWithUsage(t, fixture, step1.ID, 10, 5, 2, 3)

		step2 := createTestStep(t, fixture, run.ID)
		updateTestStepWithUsage(t, fixture, step2.ID, 15, 7, 1, 4)

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, nil)
		require.NoError(t, err)
		require.EqualValues(t, 2, got["step_count"])
		require.EqualValues(t, int64(25), got["total_input_tokens"])
		require.EqualValues(t, int64(12), got["total_output_tokens"])
		require.EqualValues(t, int64(3), got["total_cache_creation_tokens"])
		require.EqualValues(t, int64(7), got["total_cache_read_tokens"])
	})
}

// createTestStep creates the run's next debug step.
func createTestStep(
	t *testing.T,
	fixture testFixture,
	runID uuid.UUID,
) database.ChatDebugStep {
	t.Helper()
	step, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:     runID,
		ChatID:    fixture.chat.ID,
		Operation: chatdebug.OperationGenerate,
		Status:    chatdebug.StatusInProgress,
	})
	require.NoError(t, err)
	return step
}

// updateTestStepWithUsage completes a step and sets token usage fields.
func updateTestStepWithUsage(
	t *testing.T,
	fixture testFixture,
	stepID uuid.UUID,
	input, output, cacheCreation, cacheRead int64,
) {
	t.Helper()
	updateTestStepWithFullUsage(t, fixture, stepID, input, output, 0, cacheCreation, cacheRead)
}

// updateTestStepWithFullUsage completes a step with all token usage
// fields, including reasoning tokens.
func updateTestStepWithFullUsage(
	t *testing.T,
	fixture testFixture,
	stepID uuid.UUID,
	input, output, reasoning, cacheCreation, cacheRead int64,
) {
	t.Helper()
	_, err := fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
		ID:     stepID,
		ChatID: fixture.chat.ID,
		Status: chatdebug.StatusCompleted,
		Usage: map[string]any{
			"input_tokens":          input,
			"output_tokens":         output,
			"reasoning_tokens":      reasoning,
			"cache_creation_tokens": cacheCreation,
			"cache_read_tokens":     cacheRead,
		},
	})
	require.NoError(t, err)
}
