package chatd //nolint:testpackage // Tests the acquisition loop's capacity wait bookkeeping.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func newCapacityWaitWorker(t *testing.T) (*chatWorker, *quartz.Mock, *tracetest.SpanRecorder) {
	t.Helper()
	clock := quartz.NewMock(t)
	tracer, recorder := newStageTestTracer(t)
	return &chatWorker{
		server:            &Server{stages: tracer},
		opts:              chatWorkerOptions{Clock: clock},
		capacityWaitSince: make(map[uuid.UUID]capacityWait),
	}, clock, recorder
}

func candidateRows(ids ...uuid.UUID) []database.GetChatWorkerAcquisitionCandidatesRow {
	rows := make([]database.GetChatWorkerAcquisitionCandidatesRow, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, database.GetChatWorkerAcquisitionCandidatesRow{ID: id})
	}
	return rows
}

func TestCapacityWaitBookkeeping(t *testing.T) {
	t.Parallel()

	t.Run("PruneKeepsCandidates", func(t *testing.T) {
		t.Parallel()
		worker, _, _ := newCapacityWaitWorker(t)
		waiting, gone := database.Chat{ID: uuid.New()}, database.Chat{ID: uuid.New()}

		worker.noteCapacityRefused(waiting.ID, waiting.HistoryVersion)
		worker.noteCapacityRefused(gone.ID, gone.HistoryVersion)
		worker.pruneCapacityWaits(candidateRows(waiting.ID, uuid.New()))
		require.Contains(t, worker.capacityWaitSince, waiting.ID)
		require.NotContains(t, worker.capacityWaitSince, gone.ID)
	})

	t.Run("RecordMeasuresFromFirstRefusal", func(t *testing.T) {
		t.Parallel()
		worker, clock, recorder := newCapacityWaitWorker(t)
		chat := database.Chat{ID: uuid.New(), ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}

		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion)
		clock.Advance(time.Second)
		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion)
		clock.Advance(time.Second)
		worker.recordCapacityWait(t.Context(), chat)
		worker.recordCapacityWait(t.Context(), chat)

		require.NotContains(t, worker.capacityWaitSince, chat.ID)
		ended := recorder.Ended()
		require.Len(t, ended, 1)
		span := ended[0]
		require.Equal(t, string(chatloop.StageCapacityWait), span.Name())
		require.Equal(t, 2*time.Second, span.EndTime().Sub(span.StartTime()))
		require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrScope, string(chatloop.ScopeTurn)))
		require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindSubagent)))
		require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrChatID, chat.ID.String()))
	})

	t.Run("NeverRefusedRecordsNothing", func(t *testing.T) {
		t.Parallel()
		worker, _, recorder := newCapacityWaitWorker(t)

		worker.recordCapacityWait(t.Context(), database.Chat{ID: uuid.New()})
		require.Empty(t, recorder.Ended())
	})

	// A refusal remembered for one prompt must not be charged to a
	// later prompt of the same chat: the chat left this worker's
	// candidate set while every batch was full, so it was never
	// pruned, and it returned with a new history version.
	t.Run("StaleRefusalRecordsNothing", func(t *testing.T) {
		t.Parallel()
		worker, clock, recorder := newCapacityWaitWorker(t)
		chat := database.Chat{ID: uuid.New(), HistoryVersion: 3}

		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion)
		clock.Advance(time.Hour)
		chat.HistoryVersion = 4
		worker.recordCapacityWait(t.Context(), chat)

		require.Empty(t, recorder.Ended())
		require.NotContains(t, worker.capacityWaitSince, chat.ID)
	})

	t.Run("RefusalAtNewVersionRestartsWait", func(t *testing.T) {
		t.Parallel()
		worker, clock, recorder := newCapacityWaitWorker(t)
		chat := database.Chat{ID: uuid.New(), HistoryVersion: 3}

		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion)
		clock.Advance(time.Hour)
		chat.HistoryVersion = 4
		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion)
		clock.Advance(time.Second)
		worker.recordCapacityWait(t.Context(), chat)

		ended := recorder.Ended()
		require.Len(t, ended, 1)
		require.Equal(t, time.Second, ended[0].EndTime().Sub(ended[0].StartTime()))
	})
}

// TestAcquireOncePrunesOnlyShortBatches runs one acquisition pass
// against a mocked store. The worker has no pubsub, so every candidate
// fails before acquisition and only the prune decision is exercised.
func TestAcquireOncePrunesOnlyShortBatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		batchRows int
		wantKept  bool
	}{
		{name: "FullBatchKeepsWait", batchRows: 2, wantKept: true},
		{name: "ShortBatchPrunesWait", batchRows: 1, wantKept: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			worker, _, _ := newCapacityWaitWorker(t)
			worker.opts.Store = db
			worker.opts.Logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			worker.opts.AcquisitionBatchSize = 1

			ids := make([]uuid.UUID, tt.batchRows)
			for i := range ids {
				ids[i] = uuid.New()
			}
			db.EXPECT().GetChatWorkerAcquisitionCandidates(gomock.Any(), gomock.Any()).
				Return(candidateRows(ids...), nil)

			waiting := database.Chat{ID: uuid.New()}
			worker.noteCapacityRefused(waiting.ID, waiting.HistoryVersion)
			worker.acquireOnce(t.Context(), uuid.New(), nil)

			if tt.wantKept {
				require.Contains(t, worker.capacityWaitSince, waiting.ID)
			} else {
				require.NotContains(t, worker.capacityWaitSince, waiting.ID)
			}
		})
	}
}

// TestAcquireCandidateForgetsWaitOnSkip acquires against a real store
// so the transaction body runs: a candidate skipped for a reason other
// than capacity drops its remembered wait.
func TestAcquireCandidateForgetsWaitOnSkip(t *testing.T) {
	t.Parallel()

	newWorker := func(t *testing.T, f *workerTestFixture) *chatWorker {
		t.Helper()
		worker, err := newChatWorker(newUnstartedServer(t, f.pubsub, f.db), testOptions(t, f, nil))
		require.NoError(t, err)
		return worker
	}

	t.Run("FreshlyOwnedChat", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		chat := f.createRunningChat(t)
		acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
		worker := newWorker(t, f)
		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion)

		acquired, err := worker.acquireCandidate(testutil.Context(t, testutil.WaitShort), worker.opts.WorkerID, nil, chat.ID)
		require.NoError(t, err)
		require.False(t, acquired)
		require.NotContains(t, worker.capacityWaitSince, chat.ID)
	})

	t.Run("MissingChat", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		worker := newWorker(t, f)
		chatID := uuid.New()
		worker.noteCapacityRefused(chatID, 1)

		acquired, err := worker.acquireCandidate(testutil.Context(t, testutil.WaitShort), worker.opts.WorkerID, nil, chatID)
		require.NoError(t, err)
		require.False(t, acquired)
		require.NotContains(t, worker.capacityWaitSince, chatID)
	})
}
