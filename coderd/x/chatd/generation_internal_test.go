package chatd //nolint:testpackage // Exercises unexported generation helpers.

import (
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
)

func TestExclusiveBatchRejected(t *testing.T) {
	t.Parallel()

	call := func(name string) fantasy.ToolCallContent {
		return fantasy.ToolCallContent{ToolCallID: "call_" + name, ToolName: name}
	}
	exclusive := map[string]bool{"advisor": true}

	cases := []struct {
		name       string
		toolCalls  []fantasy.ToolCallContent
		exclusives map[string]bool
		want       bool
	}{
		{name: "ExclusiveAlone", toolCalls: []fantasy.ToolCallContent{call("advisor")}, exclusives: exclusive},
		{name: "NoExclusive", toolCalls: []fantasy.ToolCallContent{call("execute"), call("read_file")}, exclusives: exclusive},
		{name: "NoExclusiveNames", toolCalls: []fantasy.ToolCallContent{call("advisor"), call("execute")}},
		{
			name:       "ExclusiveMixed",
			toolCalls:  []fantasy.ToolCallContent{call("advisor"), call("execute")},
			exclusives: exclusive,
			want:       true,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, exclusiveBatchRejected(test.toolCalls, test.exclusives))
		})
	}
}

func TestCompactionMetricIdentity(t *testing.T) {
	t.Parallel()

	compaction := &generationCompaction{
		Options: chatloop.GenerateCompactionOptions{
			Model: &chattest.FakeModel{ProviderName: "anthropic", ModelName: "claude-sonnet-4-5"},
		},
	}

	provider, model := compactionMetricIdentity(compaction)
	require.Equal(t, "anthropic", provider)
	require.Equal(t, "claude-sonnet-4-5", model)

	// With an override, metrics use the prepare-time identity, not the
	// chat model carried by the options.
	compaction.Override = &resolvedModelOverride{
		ResolvedProvider: "openai",
		ResolvedModel:    "gpt-4.1-mini",
	}
	provider, model = compactionMetricIdentity(compaction)
	require.Equal(t, "openai", provider)
	require.Equal(t, "gpt-4.1-mini", model)
}

func TestGenerationCompactionContextLimit(t *testing.T) {
	t.Parallel()

	require.EqualValues(t, 0, generationCompactionContextLimit(nil))

	// The decision path must see the prepare-time compaction limit (the
	// stricter of the chat and override models' limits), not the chat
	// model's limit.
	compaction := &generationCompaction{
		Options: chatloop.GenerateCompactionOptions{ContextLimit: 50_000},
	}
	require.EqualValues(t, 50_000, generationCompactionContextLimit(compaction))
}

func TestRecordGenerationFinishFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		wantRecorded bool
	}{
		{
			name:         "TerminalFailureRecordsError",
			err:          normalizeTaskTransitionError(chatstate.ErrTransitionNotAllowed, "finish generation error"),
			wantRecorded: true,
		},
		{
			name:         "ExpectedExitSkips",
			err:          normalizeTaskTransitionError(errTaskExpectedExit, "finish generation error"),
			wantRecorded: false,
		},
		{
			name:         "RetryableSkips",
			err:          normalizeTaskTransitionError(xerrors.New("transient infrastructure failure"), "finish generation error"),
			wantRecorded: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			turn := newRunnerDebugTurn(testutil.Context(t, testutil.WaitShort), testutil.Logger(t))
			recordGenerationFinishFailure(turn, tt.err)
			require.Equal(t, tt.wantRecorded, turn.statusSet)
			if tt.wantRecorded {
				require.Equal(t, chatdebug.StatusError, turn.status)
			}
		})
	}
}

func TestRecordThinkingStages(t *testing.T) {
	t.Parallel()

	// The provider attribute is the model's wire protocol, which differs
	// from the configured provider type for bedrock.
	prepared := generationPrepared{
		StageModel: chatloop.StageModel{Provider: "anthropic", ProviderType: "bedrock", Model: "claude", Effort: "high"},
	}

	t.Run("PairsByIndex", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		starter := &taskStarter{server: &Server{stages: tracer}}
		base := time.Now().Add(-time.Minute)

		starter.recordThinkingStages(t.Context(), prepared, chatloop.PersistedStep{
			ReasoningStartedAt:   []time.Time{base, base.Add(10 * time.Second)},
			ReasoningCompletedAt: []time.Time{base.Add(2 * time.Second), base.Add(15 * time.Second)},
		})

		ended := recorder.Ended()
		require.Len(t, ended, 2)
		require.Equal(t, base.UTC(), ended[0].StartTime().UTC())
		require.Equal(t, base.Add(2*time.Second).UTC(), ended[0].EndTime().UTC())
		require.Equal(t, base.Add(10*time.Second).UTC(), ended[1].StartTime().UTC())
		require.Equal(t, base.Add(15*time.Second).UTC(), ended[1].EndTime().UTC())
		for _, span := range ended {
			require.Equal(t, string(chatloop.StageThinking), span.Name())
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrProvider, "anthropic"))
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrProviderType, "bedrock"))
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrModel, "claude"))
			require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrReasoningEffort, "high"))
		}
	})

	t.Run("StopsAtFirstUnpairedStart", func(t *testing.T) {
		t.Parallel()
		tracer, recorder := newStageTestTracer(t)
		starter := &taskStarter{server: &Server{stages: tracer}}
		base := time.Now().Add(-time.Minute)

		starter.recordThinkingStages(t.Context(), prepared, chatloop.PersistedStep{
			ReasoningStartedAt:   []time.Time{base, base.Add(10 * time.Second), base.Add(20 * time.Second)},
			ReasoningCompletedAt: []time.Time{base.Add(2 * time.Second)},
		})

		require.Len(t, recorder.Ended(), 1)
	})
}
