package chatdebug_test

import (
	"encoding/json"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
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

func TestExtractFirstUserText(t *testing.T) {
	t.Parallel()

	t.Run("EmptyPrompt", func(t *testing.T) {
		t.Parallel()
		got := chatdebug.ExtractFirstUserText(fantasy.Prompt{})
		require.Equal(t, "", got)
	})

	t.Run("NoUserMessages", func(t *testing.T) {
		t.Parallel()
		prompt := fantasy.Prompt{
			{
				Role:    fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "system"}},
			},
			{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "assistant"}},
			},
		}
		got := chatdebug.ExtractFirstUserText(prompt)
		require.Equal(t, "", got)
	})

	t.Run("FirstUserMessageMixedParts", func(t *testing.T) {
		t.Parallel()
		prompt := fantasy.Prompt{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "hello "},
					fantasy.FilePart{Filename: "test.png"},
					fantasy.TextPart{Text: "world"},
				},
			},
		}
		got := chatdebug.ExtractFirstUserText(prompt)
		require.Equal(t, "hello world", got)
	})

	t.Run("MultipleUserMessagesReturnsFirst", func(t *testing.T) {
		t.Parallel()
		prompt := fantasy.Prompt{
			{
				Role:    fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "system"}},
			},
			{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "first"}},
			},
			{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "second"}},
			},
		}
		got := chatdebug.ExtractFirstUserText(prompt)
		require.Equal(t, "first", got)
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

	t.Run("ZeroSteps", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		// No steps created. Call with a base summary containing
		// first_message so we can verify it is preserved.
		base := map[string]any{"first_message": "hello world"}
		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, base)
		require.NoError(t, err)
		require.Equal(t, "hello world", got["first_message"])
		require.EqualValues(t, 0, got["step_count"])
		require.EqualValues(t, int64(0), got["total_input_tokens"])
		require.EqualValues(t, int64(0), got["total_output_tokens"])
		require.NotContains(t, got, "total_reasoning_tokens")
		require.NotContains(t, got, "total_cache_creation_tokens")
		require.NotContains(t, got, "total_cache_read_tokens")
		require.NotContains(t, got, "has_error")
		require.NotContains(t, got, "endpoint_label")
	})

	t.Run("NilBaseSummary", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		// Create a step with usage.
		step := createTestStep(t, fixture, run.ID)
		updateTestStepWithUsage(t, fixture, step.ID, 10, 5, 0, 0)

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, nil)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.EqualValues(t, 1, got["step_count"])
		require.EqualValues(t, int64(10), got["total_input_tokens"])
		require.EqualValues(t, int64(5), got["total_output_tokens"])
	})

	t.Run("PreservesFirstMessage", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		step := createTestStep(t, fixture, run.ID)
		updateTestStepWithUsage(t, fixture, step.ID, 20, 10, 0, 0)

		base := map[string]any{"first_message": "hello world"}
		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, base)
		require.NoError(t, err)
		require.Equal(t, "hello world", got["first_message"])
		require.EqualValues(t, 1, got["step_count"])
		require.EqualValues(t, int64(20), got["total_input_tokens"])
		require.EqualValues(t, int64(10), got["total_output_tokens"])
	})

	t.Run("ClearsStaleDerivedFields", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		step := createTestStep(t, fixture, run.ID)
		updateTestStepWithUsage(t, fixture, step.ID, 10, 5, 0, 0)

		base := map[string]any{
			"first_message":               "hello world",
			"step_count":                  9,
			"total_input_tokens":          999,
			"total_output_tokens":         888,
			"total_reasoning_tokens":      777,
			"total_cache_creation_tokens": 100,
			"total_cache_read_tokens":     200,
			"has_error":                   true,
			"endpoint_label":              "POST /stale",
		}

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, base)
		require.NoError(t, err)
		require.Equal(t, "hello world", got["first_message"])
		require.EqualValues(t, 1, got["step_count"])
		require.EqualValues(t, int64(10), got["total_input_tokens"])
		require.EqualValues(t, int64(5), got["total_output_tokens"])
		// Stale reasoning tokens must be cleared because the step
		// has zero reasoning tokens.
		require.NotContains(t, got, "total_reasoning_tokens")
		require.NotContains(t, got, "total_cache_creation_tokens")
		require.NotContains(t, got, "total_cache_read_tokens")
		// has_error must be cleared because the step is not in error
		// status and has no error payload.
		require.NotContains(t, got, "has_error")
		require.NotContains(t, got, "endpoint_label")
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

		step2 := createTestStepN(t, fixture, run.ID, 2)
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

	t.Run("EndpointLabelPathOnlyWhenMethodEmpty", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		step := createTestStep(t, fixture, run.ID)
		_, err := fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
			ID:     step.ID,
			ChatID: fixture.chat.ID,
			Status: chatdebug.StatusCompleted,
			Attempts: []chatdebug.Attempt{{
				Number: 1,
				Status: "completed",
				Method: "",
				Path:   "/v1/messages",
			}},
		})
		require.NoError(t, err)

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, nil)
		require.NoError(t, err)
		require.Equal(t, "/v1/messages", got["endpoint_label"],
			"endpoint_label should be path-only when method is empty")
	})

	t.Run("InterruptedStepWithErrorExcludedFromHasError", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		// An interrupted step with a real error payload should NOT
		// trigger has_error. Interrupted means user-initiated
		// cancellation (e.g. clicking Stop).
		step := createTestStep(t, fixture, run.ID)
		_, err := fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
			ID:     step.ID,
			ChatID: fixture.chat.ID,
			Status: chatdebug.StatusInterrupted,
			Error:  map[string]any{"message": "user canceled"},
		})
		require.NoError(t, err)

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, nil)
		require.NoError(t, err)
		require.NotContains(t, got, "has_error",
			"interrupted steps should not trigger has_error even with error payload")
	})

	t.Run("MultipleStepsSumTokens", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		step1 := createTestStep(t, fixture, run.ID)
		updateTestStepWithUsage(t, fixture, step1.ID, 10, 5, 2, 3)

		step2 := createTestStepN(t, fixture, run.ID, 2)
		updateTestStepWithUsage(t, fixture, step2.ID, 15, 7, 1, 4)

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, nil)
		require.NoError(t, err)
		require.EqualValues(t, 2, got["step_count"])
		require.EqualValues(t, int64(25), got["total_input_tokens"])
		require.EqualValues(t, int64(12), got["total_output_tokens"])
		require.EqualValues(t, int64(3), got["total_cache_creation_tokens"])
		require.EqualValues(t, int64(7), got["total_cache_read_tokens"])
	})

	t.Run("StepWithNilUsageContributesZeroTokens", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		// Step with usage.
		step1 := createTestStep(t, fixture, run.ID)
		updateTestStepWithUsage(t, fixture, step1.ID, 10, 5, 0, 0)

		// Step without usage (just complete it, no usage).
		step2 := createTestStepN(t, fixture, run.ID, 2)
		_, err := fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
			ID:     step2.ID,
			ChatID: fixture.chat.ID,
			Status: chatdebug.StatusCompleted,
		})
		require.NoError(t, err)

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, nil)
		require.NoError(t, err)
		// Both steps are counted even though one has no usage.
		require.EqualValues(t, 2, got["step_count"])
		require.EqualValues(t, int64(10), got["total_input_tokens"])
		require.EqualValues(t, int64(5), got["total_output_tokens"])
	})

	t.Run("ZeroCacheTotalsOmitCacheFields", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		step := createTestStep(t, fixture, run.ID)
		updateTestStepWithUsage(t, fixture, step.ID, 10, 5, 0, 0)

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, nil)
		require.NoError(t, err)
		_, hasCacheCreation := got["total_cache_creation_tokens"]
		_, hasCacheRead := got["total_cache_read_tokens"]
		require.False(t, hasCacheCreation,
			"cache creation tokens should be omitted when zero")
		require.False(t, hasCacheRead,
			"cache read tokens should be omitted when zero")
	})

	t.Run("ReasoningTokensSummedAcrossSteps", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		step1 := createTestStep(t, fixture, run.ID)
		updateTestStepWithFullUsage(t, fixture, step1.ID, 10, 5, 20, 0, 0)

		step2 := createTestStepN(t, fixture, run.ID, 2)
		updateTestStepWithFullUsage(t, fixture, step2.ID, 15, 7, 30, 0, 0)

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, nil)
		require.NoError(t, err)
		require.EqualValues(t, 2, got["step_count"])
		require.EqualValues(t, int64(25), got["total_input_tokens"])
		require.EqualValues(t, int64(12), got["total_output_tokens"])
		require.EqualValues(t, int64(50), got["total_reasoning_tokens"],
			"reasoning tokens should be summed across steps")
	})

	t.Run("ZeroReasoningTokensOmitsField", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		step := createTestStep(t, fixture, run.ID)
		updateTestStepWithFullUsage(t, fixture, step.ID, 10, 5, 0, 0, 0)

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, nil)
		require.NoError(t, err)
		_, hasReasoning := got["total_reasoning_tokens"]
		require.False(t, hasReasoning,
			"reasoning tokens should be omitted when zero")
	})

	t.Run("MalformedUsageJSONSkipped", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		// Step 1 has valid usage and should contribute to totals.
		step1 := createTestStep(t, fixture, run.ID)
		updateTestStepWithUsage(t, fixture, step1.ID, 10, 5, 0, 0)

		// Step 2 is stamped with structurally-valid JSONB that cannot
		// unmarshal into fantasy.Usage (string where int64 is
		// expected). Write directly through the store so the jsonb
		// cast succeeds while the Go unmarshal fails, exercising the
		// "skipping malformed step usage JSON" log-and-continue path.
		step2 := createTestStepN(t, fixture, run.ID, 2)
		_, err := fixture.db.UpdateChatDebugStep(fixture.ctx, database.UpdateChatDebugStepParams{
			ID:     step2.ID,
			ChatID: fixture.chat.ID,
			Usage: pqtype.NullRawMessage{
				RawMessage: json.RawMessage(`{"input_tokens":"not-a-number"}`),
				Valid:      true,
			},
			Now: time.Now(),
		})
		require.NoError(t, err)

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, nil)
		require.NoError(t, err,
			"malformed usage JSON must be skipped, not surfaced as an error")

		// Both steps are counted, but only step1's tokens contribute.
		require.EqualValues(t, 2, got["step_count"])
		require.EqualValues(t, int64(10), got["total_input_tokens"])
		require.EqualValues(t, int64(5), got["total_output_tokens"])
	})
}

// createTestStep is a thin helper that creates a debug step with
// step number 1 for the given run.
func createTestStep(
	t *testing.T,
	fixture testFixture,
	runID uuid.UUID,
) database.ChatDebugStep {
	t.Helper()
	return createTestStepN(t, fixture, runID, 1)
}

// createTestStepN creates a debug step with the given step number.
func createTestStepN(
	t *testing.T,
	fixture testFixture,
	runID uuid.UUID,
	stepNumber int32,
) database.ChatDebugStep {
	t.Helper()
	step, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:      runID,
		ChatID:     fixture.chat.ID,
		StepNumber: stepNumber,
		Operation:  chatdebug.OperationGenerate,
		Status:     chatdebug.StatusInProgress,
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
