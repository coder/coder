package chatdebug_test

import (
	"context"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/testutil"
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
		svc := chatdebug.NewService(dbmock.NewMockStore(gomock.NewController(t)), testutil.Logger(t), nil)
		base := map[string]any{"first_message": "hello world"}
		got, err := svc.AggregateRunSummary(testutil.Context(t, testutil.WaitLong), uuid.Nil, base)
		require.NoError(t, err)
		require.Equal(t, base, got)
	})

	t.Run("QueryAsChatdReturnsError", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		runID := uuid.New()
		db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).DoAndReturn(
			func(ctx context.Context, _ uuid.UUID) ([]database.ChatDebugStep, error) {
				actor, ok := dbauthz.ActorFromContext(ctx)
				require.True(t, ok)
				require.Equal(t, rbac.SubjectTypeChatd, actor.Type)
				return nil, xerrors.New("boom")
			})
		svc := chatdebug.NewService(db, testutil.Logger(t), nil)
		got, err := svc.AggregateRunSummary(testutil.Context(t, testutil.WaitLong), runID, nil)
		require.ErrorContains(t, err, "boom")
		require.Nil(t, got)
	})

	t.Run("RecomputesHasErrorAndCompletedEndpointLabel", func(t *testing.T) {
		t.Parallel()
		fixture := newFixture(t)
		run := createRun(t, fixture)

		// Step 2 is inserted first so the label proves the query
		// orders rows by step_number rather than insertion. The rows
		// go through the store because CreateStep numbers steps itself.
		for _, step := range []struct {
			number   int32
			status   chatdebug.Status
			attempts []chatdebug.Attempt
			usage    map[string]any
		}{
			{
				number:   2,
				status:   chatdebug.StatusCompleted,
				attempts: []chatdebug.Attempt{{Number: 1, Status: "completed", Method: "POST", Path: "/second"}},
				usage:    map[string]any{"input_tokens": 15, "output_tokens": 7, "reasoning_tokens": 30, "cache_creation_tokens": 1, "cache_read_tokens": 4},
			},
			{
				number: 1,
				status: chatdebug.StatusError,
				attempts: []chatdebug.Attempt{
					{Number: 1, Status: "failed", Method: "POST", Path: "/failed"},
					{Number: 2, Status: "completed", Method: "POST", Path: "/v1/messages"},
				},
				usage: map[string]any{"input_tokens": 10, "output_tokens": 5, "reasoning_tokens": 20, "cache_creation_tokens": 2, "cache_read_tokens": 3},
			},
		} {
			created, err := fixture.db.InsertChatDebugStep(fixture.ctx, database.InsertChatDebugStepParams{
				RunID:      run.ID,
				ChatID:     fixture.chat.ID,
				StepNumber: step.number,
				Operation:  string(chatdebug.OperationGenerate),
				Status:     string(chatdebug.StatusInProgress),
			})
			require.NoError(t, err)
			_, err = fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
				ID:       created.ID,
				ChatID:   fixture.chat.ID,
				Status:   step.status,
				Usage:    step.usage,
				Attempts: step.attempts,
			})
			require.NoError(t, err)
		}

		got, err := fixture.svc.AggregateRunSummary(fixture.ctx, run.ID, map[string]any{"first_message": "hello world"})
		require.NoError(t, err)
		require.Equal(t, map[string]any{
			"first_message":               "hello world",
			"step_count":                  2,
			"total_input_tokens":          int64(25),
			"total_output_tokens":         int64(12),
			"total_reasoning_tokens":      int64(50),
			"total_cache_creation_tokens": int64(3),
			"total_cache_read_tokens":     int64(7),
			"has_error":                   true,
			"endpoint_label":              "POST /v1/messages",
		}, got)
	})
}
