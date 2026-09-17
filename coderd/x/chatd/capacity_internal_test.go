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
	"github.com/coder/quartz"
)

func newCapacityWaitWorker(t *testing.T) (*chatWorker, *quartz.Mock, *tracetest.SpanRecorder) {
	t.Helper()
	clock := quartz.NewMock(t)
	tracer, recorder := newStageTestTracer(t)
	return &chatWorker{
		server:            &Server{stages: tracer},
		opts:              chatWorkerOptions{Clock: clock},
		capacityWaitSince: make(map[uuid.UUID]time.Time),
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
		waiting, gone := uuid.New(), uuid.New()

		worker.noteCapacityRefused(waiting)
		worker.noteCapacityRefused(gone)
		worker.pruneCapacityWaits(candidateRows(waiting, uuid.New()))
		require.Contains(t, worker.capacityWaitSince, waiting)
		require.NotContains(t, worker.capacityWaitSince, gone)
	})

	t.Run("RecordMeasuresFromFirstRefusal", func(t *testing.T) {
		t.Parallel()
		worker, clock, recorder := newCapacityWaitWorker(t)
		chat := database.Chat{ID: uuid.New(), ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}

		worker.noteCapacityRefused(chat.ID)
		clock.Advance(time.Second)
		worker.noteCapacityRefused(chat.ID)
		clock.Advance(time.Second)
		worker.recordCapacityWait(t.Context(), chat)
		worker.recordCapacityWait(t.Context(), chat)

		require.NotContains(t, worker.capacityWaitSince, chat.ID)
		ended := recorder.Ended()
		require.Len(t, ended, 1)
		span := ended[0]
		require.Equal(t, chatloop.StageCapacityWait, span.Name())
		require.Equal(t, 2*time.Second, span.EndTime().Sub(span.StartTime()))
		require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrScope, chatloop.ScopeTurn))
		require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrChatKind, chatloop.ChatKindSubagent))
		require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrChatID, chat.ID.String()))
	})

	t.Run("NeverRefusedRecordsNothing", func(t *testing.T) {
		t.Parallel()
		worker, _, recorder := newCapacityWaitWorker(t)

		worker.recordCapacityWait(t.Context(), database.Chat{ID: uuid.New()})
		require.Empty(t, recorder.Ended())
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

			waiting := uuid.New()
			worker.noteCapacityRefused(waiting)
			worker.acquireOnce(t.Context(), uuid.New(), nil)

			if tt.wantKept {
				require.Contains(t, worker.capacityWaitSince, waiting)
			} else {
				require.NotContains(t, worker.capacityWaitSince, waiting)
			}
		})
	}
}
