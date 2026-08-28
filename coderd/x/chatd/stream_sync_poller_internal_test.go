package chatd

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)

// TestStreamSyncPollerConcurrentRegisterUnregister churns subscriber
// registration while pollOnce delivers hints concurrently. Before the poller
// became a callback fanout, unregister closed the subscriber's hint channel
// while pollOnce could still be mid-send on its lock-free snapshot, panicking
// with "send on closed channel" (GHSA-7x3x-59xg-4hrc). Run with -race.
func TestStreamSyncPollerConcurrentRegisterUnregister(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	db.EXPECT().GetChatStreamSyncRows(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, ids []uuid.UUID) ([]database.GetChatStreamSyncRowsRow, error) {
			rows := make([]database.GetChatStreamSyncRowsRow, 0, len(ids))
			for _, id := range ids {
				rows = append(rows, database.GetChatStreamSyncRowsRow{ID: id})
			}
			return rows, nil
		},
	)

	poller := newStreamSyncPoller(context.Background(), db, nil, slogtest.Make(t, nil))
	defer poller.Close()

	chatID := uuid.New()
	done := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				unregister := poller.Register(chatID, func(streamSyncHint) {})
				unregister()
			}
		}()
	}
	for range 1000 {
		poller.pollOnce()
	}
	close(done)
	wg.Wait()
}

// TestStreamSyncPollerNilRegister verifies a nil poller degrades to a no-op
// registration instead of terminating subscribers.
func TestStreamSyncPollerNilRegister(t *testing.T) {
	t.Parallel()

	var poller *streamSyncPoller
	unregister := poller.Register(uuid.New(), func(streamSyncHint) {
		t.Fatal("nil poller must never deliver hints")
	})
	unregister()
}
