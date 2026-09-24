package chatd //nolint:testpackage // Tests the acquisition loop's capacity wait bookkeeping.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func newCapacityWaitWorker(t *testing.T) (*chatWorker, *quartz.Mock, *tracetest.SpanRecorder) {
	t.Helper()
	clock := quartz.NewMock(t)
	tracer, recorder := newStageTestTracer(t)
	return &chatWorker{
		server:        &Server{stages: tracer},
		opts:          chatWorkerOptions{Clock: clock},
		capacityWaits: make(map[uuid.UUID]capacityWait),
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

		worker.noteCapacityRefused(waiting.ID, waiting.HistoryVersion, waiting.UpdatedAt)
		worker.noteCapacityRefused(gone.ID, gone.HistoryVersion, gone.UpdatedAt)
		worker.pruneCapacityWaits(candidateRows(waiting.ID, uuid.New()))
		require.Contains(t, worker.capacityWaits, waiting.ID)
		require.NotContains(t, worker.capacityWaits, gone.ID)
	})

	t.Run("RecordMeasuresFromFirstRefusal", func(t *testing.T) {
		t.Parallel()
		worker, clock, recorder := newCapacityWaitWorker(t)
		chat := database.Chat{
			ID:           uuid.New(),
			Status:       database.ChatStatusRunning,
			ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		}

		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion, chat.UpdatedAt)
		clock.Advance(time.Second)
		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion, chat.UpdatedAt)
		clock.Advance(time.Second)
		worker.recordCapacityWait(t.Context(), chat)
		worker.recordCapacityWait(t.Context(), chat)

		require.NotContains(t, worker.capacityWaits, chat.ID)
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

		worker.recordCapacityWait(t.Context(), database.Chat{ID: uuid.New(), Status: database.ChatStatusRunning})
		require.Empty(t, recorder.Ended())
	})

	// A refusal remembered for one prompt must not be charged to a
	// later prompt of the same chat: the chat left this worker's
	// candidate set while every candidate batch was truncated at its
	// limit, so it was never pruned, and it returned with a new history
	// version.
	t.Run("StaleRefusalRecordsNothing", func(t *testing.T) {
		t.Parallel()
		worker, clock, recorder := newCapacityWaitWorker(t)
		chat := database.Chat{ID: uuid.New(), Status: database.ChatStatusRunning, HistoryVersion: 3}

		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion, chat.UpdatedAt)
		clock.Advance(time.Hour)
		chat.HistoryVersion = 4
		worker.recordCapacityWait(t.Context(), chat)

		require.Empty(t, recorder.Ended())
		require.NotContains(t, worker.capacityWaits, chat.ID)
	})

	t.Run("RefusalAtNewVersionRestartsWait", func(t *testing.T) {
		t.Parallel()
		worker, clock, recorder := newCapacityWaitWorker(t)
		chat := database.Chat{ID: uuid.New(), Status: database.ChatStatusRunning, HistoryVersion: 3}

		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion, chat.UpdatedAt)
		clock.Advance(time.Hour)
		chat.HistoryVersion = 4
		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion, chat.UpdatedAt)
		clock.Advance(time.Second)
		worker.recordCapacityWait(t.Context(), chat)

		ended := recorder.Ended()
		require.Len(t, ended, 1)
		require.Equal(t, time.Second, ended[0].EndTime().Sub(ended[0].StartTime()))
	})

	// Another replica acquiring and abandoning the chat writes
	// updated_at without changing history_version.
	t.Run("RowWriteRecordsNothing", func(t *testing.T) {
		t.Parallel()
		worker, clock, recorder := newCapacityWaitWorker(t)
		chat := database.Chat{
			ID:             uuid.New(),
			Status:         database.ChatStatusRunning,
			HistoryVersion: 3,
			UpdatedAt:      clock.Now(),
		}

		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion, chat.UpdatedAt)
		clock.Advance(time.Hour)
		chat.UpdatedAt = clock.Now()
		worker.recordCapacityWait(t.Context(), chat)

		require.Empty(t, recorder.Ended())
		require.NotContains(t, worker.capacityWaits, chat.ID)
	})

	t.Run("RefusalAfterRowWriteRestartsWait", func(t *testing.T) {
		t.Parallel()
		worker, clock, recorder := newCapacityWaitWorker(t)
		chat := database.Chat{
			ID:             uuid.New(),
			Status:         database.ChatStatusRunning,
			HistoryVersion: 3,
			UpdatedAt:      clock.Now(),
		}

		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion, chat.UpdatedAt)
		clock.Advance(time.Hour)
		chat.UpdatedAt = clock.Now()
		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion, chat.UpdatedAt)
		clock.Advance(time.Second)
		worker.recordCapacityWait(t.Context(), chat)

		ended := recorder.Ended()
		require.Len(t, ended, 1)
		require.Equal(t, time.Second, ended[0].EndTime().Sub(ended[0].StartTime()))
	})

	// The limiter admits every non-running chat, so a chat interrupted
	// while it waited was not admitted for capacity.
	t.Run("InterruptingChatRecordsNothing", func(t *testing.T) {
		t.Parallel()
		worker, clock, recorder := newCapacityWaitWorker(t)
		chat := database.Chat{ID: uuid.New(), Status: database.ChatStatusRunning}

		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion, chat.UpdatedAt)
		clock.Advance(time.Second)
		chat.Status = database.ChatStatusInterrupting
		worker.recordCapacityWait(t.Context(), chat)

		require.Empty(t, recorder.Ended())
		require.NotContains(t, worker.capacityWaits, chat.ID)
	})
}

// TestAcquireOnceRecordsCapacityWait runs acquisition passes against a
// real store: one pass refuses every running chat, and a later pass
// admits them.
func TestAcquireOnceRecordsCapacityWait(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t)
	tracer, recorder := newStageTestTracer(t)
	server := newUnstartedServer(t, f.pubsub, f.db)
	server.stages = tracer
	admission := newFakeAdmission()
	opts := testOptions(t, f, nil)
	opts.Clock = clock
	opts.AgentCapacityLimiter = admission
	worker, err := newChatWorker(server, opts)
	require.NoError(t, err)
	manager := newRunnerManager(ctx, server, worker.opts)

	// The first chat is refused by the limiter and the rest are
	// skipped because its pool refused, so both refusal paths run.
	refused := f.createRunningChat(t)
	skipped := f.createRunningChat(t)
	rowWritten := f.createRunningChat(t)
	_, err = f.sqlDB.ExecContext(ctx,
		"UPDATE chats SET updated_at = NOW() - INTERVAL '1 hour' WHERE id = $1", refused.ID)
	require.NoError(t, err)
	for _, chat := range []database.Chat{refused, skipped, rowWritten} {
		admission.refuse(chat.ID)
	}
	worker.acquireOnce(ctx, worker.opts.WorkerID, manager)
	require.Equal(t, 1, admission.admitCallCount())
	require.Len(t, worker.capacityWaits, 3)

	clock.Advance(5 * time.Second)
	_, err = f.sqlDB.ExecContext(ctx,
		"UPDATE chats SET updated_at = NOW() + INTERVAL '1 second' WHERE id = $1", rowWritten.ID)
	require.NoError(t, err)
	for _, chat := range []database.Chat{refused, skipped, rowWritten} {
		admission.allow(chat.ID)
	}
	worker.acquireOnce(ctx, worker.opts.WorkerID, manager)

	require.ElementsMatch(t, []uuid.UUID{refused.ID, skipped.ID, rowWritten.ID}, admission.admittedOrder())
	require.Empty(t, worker.capacityWaits)
	recorded := make([]string, 0, 2)
	for _, span := range recorder.Ended() {
		require.Equal(t, string(chatloop.StageCapacityWait), span.Name())
		require.Equal(t, 5*time.Second, span.EndTime().Sub(span.StartTime()))
		require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrOrganizationName, f.org.Name))
		require.Contains(t, span.Attributes(), attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindRoot)))
		for _, attr := range span.Attributes() {
			if attr.Key == chatloop.AttrChatID {
				recorded = append(recorded, attr.Value.AsString())
			}
		}
	}
	require.ElementsMatch(t, []string{refused.ID.String(), skipped.ID.String()}, recorded)
}

// TestAcquireOncePrunesOnlyShortBatches runs one acquisition pass
// against a mocked store. Every candidate transaction fails, so only
// the prune decision is exercised.
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
			worker.opts.Pubsub = dbpubsub.NewInMemory()
			worker.opts.AcquisitionBatchSize = 1
			db.EXPECT().InTx(gomock.Any(), gomock.Any()).Return(xerrors.New("skip")).AnyTimes()

			ids := make([]uuid.UUID, tt.batchRows)
			for i := range ids {
				ids[i] = uuid.New()
			}
			db.EXPECT().GetChatWorkerAcquisitionCandidates(gomock.Any(), gomock.Any()).
				Return(candidateRows(ids...), nil)

			waiting := database.Chat{ID: uuid.New()}
			worker.noteCapacityRefused(waiting.ID, waiting.HistoryVersion, waiting.UpdatedAt)
			worker.acquireOnce(t.Context(), uuid.New(), nil)

			if tt.wantKept {
				require.Contains(t, worker.capacityWaits, waiting.ID)
			} else {
				require.NotContains(t, worker.capacityWaits, waiting.ID)
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
		worker.noteCapacityRefused(chat.ID, chat.HistoryVersion, chat.UpdatedAt)

		acquired, err := worker.acquireCandidate(testutil.Context(t, testutil.WaitShort), worker.opts.WorkerID, nil, chat.ID)
		require.NoError(t, err)
		require.False(t, acquired)
		require.NotContains(t, worker.capacityWaits, chat.ID)
	})

	t.Run("MissingChat", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		worker := newWorker(t, f)
		chatID := uuid.New()
		worker.noteCapacityRefused(chatID, 1, time.Time{})

		acquired, err := worker.acquireCandidate(testutil.Context(t, testutil.WaitShort), worker.opts.WorkerID, nil, chatID)
		require.NoError(t, err)
		require.False(t, acquired)
		require.NotContains(t, worker.capacityWaits, chatID)
	})
}
