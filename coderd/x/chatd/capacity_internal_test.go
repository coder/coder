package chatd //nolint:testpackage // Tests the acquisition loop's capacity wait bookkeeping.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

func newCapacityWaitWorker(t *testing.T) (*chatWorker, *quartz.Mock) {
	t.Helper()
	clock := quartz.NewMock(t)
	tracer, _ := newStageTestTracer(t)
	return &chatWorker{
		server:            &Server{stages: tracer},
		opts:              chatWorkerOptions{Clock: clock},
		capacityWaitSince: make(map[uuid.UUID]time.Time),
	}, clock
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

	t.Run("FirstRefusalStartsTheClock", func(t *testing.T) {
		t.Parallel()
		worker, clock := newCapacityWaitWorker(t)
		chatID := uuid.New()

		worker.noteCapacityRefused(chatID)
		first := worker.capacityWaitSince[chatID]
		clock.Advance(time.Second)
		worker.noteCapacityRefused(chatID)
		require.Equal(t, first, worker.capacityWaitSince[chatID], "a later refusal keeps the first start")
	})

	t.Run("SkippedChatForgetsItsWait", func(t *testing.T) {
		t.Parallel()
		worker, _ := newCapacityWaitWorker(t)
		chatID := uuid.New()

		worker.noteCapacityRefused(chatID)
		worker.forgetCapacityWait(chatID)
		require.NotContains(t, worker.capacityWaitSince, chatID)
	})

	t.Run("PruneKeepsCandidates", func(t *testing.T) {
		t.Parallel()
		worker, _ := newCapacityWaitWorker(t)
		waiting, gone := uuid.New(), uuid.New()

		worker.noteCapacityRefused(waiting)
		worker.noteCapacityRefused(gone)
		worker.pruneCapacityWaits(candidateRows(waiting, uuid.New()))
		require.Contains(t, worker.capacityWaitSince, waiting)
		require.NotContains(t, worker.capacityWaitSince, gone)
	})

	t.Run("RecordClearsTheWait", func(t *testing.T) {
		t.Parallel()
		worker, clock := newCapacityWaitWorker(t)
		chat := database.Chat{ID: uuid.New()}

		worker.noteCapacityRefused(chat.ID)
		clock.Advance(time.Second)
		worker.recordCapacityWait(t.Context(), chat)
		require.NotContains(t, worker.capacityWaitSince, chat.ID)
	})
}
