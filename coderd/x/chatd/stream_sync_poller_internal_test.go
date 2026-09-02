package chatd

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
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

// TestStreamSyncPollerStalledSubscriber verifies pollOnce completes even when
// a subscriber's consumer has stopped draining its update channel. The
// subscriber callback must stay nonblocking (drop-on-full); a blocking send
// would stall the shared poll loop for every chat on the replica.
func TestStreamSyncPollerStalledSubscriber(t *testing.T) {
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

	// A full, never-drained channel simulates a stalled consumer. The
	// drop-on-full callback mirrors the consumer in subscribeStreamLoop.
	stalled := make(chan streamSyncHint, 1)
	stalled <- streamSyncHint{}
	unregister := poller.Register(uuid.New(), func(hint streamSyncHint) {
		select {
		case stalled <- hint:
		default:
		}
	})
	defer unregister()

	pollDone := make(chan struct{})
	go func() {
		defer close(pollDone)
		poller.pollOnce()
	}()
	select {
	case <-pollDone:
	case <-time.After(testutil.WaitShort):
		t.Fatal("pollOnce blocked on a stalled subscriber")
	}
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
