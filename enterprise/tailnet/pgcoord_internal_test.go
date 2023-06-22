package tailnet

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/database/dbmock"
	"github.com/coder/coder/coderd/database/pubsub"
	"github.com/coder/coder/testutil"
)

// TestHeartbeat_Cleanup is internal so that we can overwrite the cleanup period and not wait an hour for the timed
// cleanup.
func TestHeartbeat_Cleanup(t *testing.T) {
	t.Parallel()

	// this won't get notifications from the store, but we don't need them for this test.
	ps := pubsub.NewInMemory()

	ctrl := gomock.NewController(t)
	mStore := dbmock.NewMockStore(ctrl)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	waitForCleanup := make(chan struct{})
	mStore.EXPECT().CleanTailnetCoordinators(gomock.Any()).MinTimes(2).DoAndReturn(func(_ context.Context) error {
		<-waitForCleanup
		return nil
	})

	uut := &heartbeats{
		ctx:           ctx,
		logger:        logger,
		pubsub:        ps,
		store:         mStore,
		cleanupPeriod: time.Millisecond,
	}
	go uut.cleanupLoop()

	for i := 0; i < 2; i++ {
		select {
		case <-ctx.Done():
			t.Fatal("timeout")
		case waitForCleanup <- struct{}{}:
			// ok
		}
	}
	close(waitForCleanup)
}
