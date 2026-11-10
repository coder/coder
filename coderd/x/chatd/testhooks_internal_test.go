package chatd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWaitUntilIdleForTestTimeout(t *testing.T) {
	t.Parallel()

	t.Run("RunnerOpen", func(t *testing.T) {
		t.Parallel()
		server := &Server{chatWorker: &chatWorker{manager: &runnerManager{
			runners: map[runnerKey]*runnerRecord{{}: nil},
		}}}

		ctx, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
		defer cancel()
		require.ErrorIs(t, WaitUntilIdleForTest(ctx, server), context.DeadlineExceeded)
	})

	t.Run("InflightWork", func(t *testing.T) {
		t.Parallel()
		server := &Server{}
		release := make(chan struct{})
		t.Cleanup(func() {
			close(release)
			server.drainInflight()
		})
		require.NoError(t, server.goInflight(func() { <-release }))

		ctx, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
		defer cancel()
		require.ErrorIs(t, WaitUntilIdleForTest(ctx, server), context.DeadlineExceeded)
	})
}
