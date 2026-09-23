package chatd

import (
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
)

// recordedToolCall is one invocation of the toolCallStageRecorder's
// record callback.
type recordedToolCall struct {
	dispatchIndex int
	startedAt     time.Time
	completedAt   time.Time
}

// fakeBillingRecorder captures the callbacks forwarded to the inner
// billing recorder.
type fakeBillingRecorder struct {
	starts    []int
	completes []int
}

func (r *fakeBillingRecorder) RecordStart(dispatchIndex int, _ time.Time) {
	r.starts = append(r.starts, dispatchIndex)
}

func (r *fakeBillingRecorder) RecordComplete(dispatchIndex int, _ time.Time) {
	r.completes = append(r.completes, dispatchIndex)
}

func TestToolCallStageRecorder(t *testing.T) {
	t.Parallel()

	newRecorder := func() (*toolCallStageRecorder, *fakeBillingRecorder, *[]recordedToolCall) {
		inner := &fakeBillingRecorder{}
		var recorded []recordedToolCall
		recorder := &toolCallStageRecorder{
			inner:  inner,
			starts: map[int]time.Time{},
			record: func(dispatchIndex int, startedAt, completedAt time.Time) {
				recorded = append(recorded, recordedToolCall{dispatchIndex, startedAt, completedAt})
			},
		}
		return recorder, inner, &recorded
	}

	t.Run("PairsStartWithComplete", func(t *testing.T) {
		t.Parallel()
		recorder, inner, recorded := newRecorder()
		startedAt := time.Now().Add(-time.Second)
		completedAt := time.Now()

		recorder.RecordStart(1, startedAt)
		recorder.RecordComplete(1, completedAt)

		require.Equal(t, []int{1}, inner.starts)
		require.Equal(t, []int{1}, inner.completes)
		require.Equal(t, []recordedToolCall{{1, startedAt, completedAt}}, *recorded)
	})

	t.Run("CompleteWithoutStartIsDropped", func(t *testing.T) {
		t.Parallel()
		recorder, inner, recorded := newRecorder()

		recorder.RecordComplete(3, time.Now())

		require.Equal(t, []int{3}, inner.completes)
		require.Empty(t, *recorded)
	})

	t.Run("NilInnerIsAllowed", func(t *testing.T) {
		t.Parallel()
		var recorded []recordedToolCall
		recorder := &toolCallStageRecorder{
			starts: map[int]time.Time{},
			record: func(dispatchIndex int, startedAt, completedAt time.Time) {
				recorded = append(recorded, recordedToolCall{dispatchIndex, startedAt, completedAt})
			},
		}
		recorder.RecordStart(0, time.Now())
		recorder.RecordComplete(0, time.Now())
		require.Len(t, recorded, 1)
	})
}

func TestToolCallName(t *testing.T) {
	t.Parallel()

	calls := []fantasy.ToolCallContent{{ToolName: "read_file"}, {ToolName: "execute"}}
	require.Equal(t, "read_file", toolCallName(calls, 0))
	require.Equal(t, "execute", toolCallName(calls, 1))
	require.Equal(t, "", toolCallName(calls, 2))
	require.Equal(t, "", toolCallName(calls, -1))
	require.Equal(t, "", toolCallName(nil, 0))
}

func TestRecordThinkingStages(t *testing.T) {
	t.Parallel()

	prepared := generationPrepared{
		ResolvedProvider: "anthropic",
		StageModel:       chatloop.StageModel{ProviderType: "anthropic", Model: "claude", Effort: "high"},
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

func TestServerOrganizationName(t *testing.T) {
	t.Parallel()

	t.Run("CachesResolvedName", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, nil)}
		orgID := uuid.New()
		db.EXPECT().GetOrganizationByID(gomock.Any(), orgID).
			Return(database.Organization{ID: orgID, Name: "acme"}, nil).Times(1)

		require.Equal(t, "acme", server.organizationName(t.Context(), orgID))
		require.Equal(t, "acme", server.organizationName(t.Context(), orgID))
	})

	t.Run("FailedLookupIsNotCached", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})}
		orgID := uuid.New()
		db.EXPECT().GetOrganizationByID(gomock.Any(), orgID).
			Return(database.Organization{}, xerrors.New("boom")).Times(1)
		db.EXPECT().GetOrganizationByID(gomock.Any(), orgID).
			Return(database.Organization{ID: orgID, Name: "acme"}, nil).Times(1)

		require.Equal(t, "", server.organizationName(t.Context(), orgID))
		require.Equal(t, "acme", server.organizationName(t.Context(), orgID))
	})

	t.Run("NilIDSkipsTheStore", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, nil)}

		require.Equal(t, "", server.organizationName(t.Context(), uuid.Nil))
	})
}
