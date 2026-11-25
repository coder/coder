package notifications

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestNotifier_NoFetchAfterGracefulStop(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	// The mock store fails the test on any call, which is how a fetch
	// would show up.
	store := dbmock.NewMockStore(gomock.NewController(t))
	clock := &lateTickClock{Clock: quartz.NewMock(t), fetch: make(chan func() error, 1)}

	n := newNotifier(ctx, codersdk.NotificationsConfig{}, uuid.New(), testutil.Logger(t), store, nil, nil, nil, clock)
	runErr := make(chan error, 1)
	go func() {
		runErr <- n.run(nil, nil)
	}()
	fetch := testutil.TryReceive(ctx, t, clock.fetch)

	n.stop()

	require.NoError(t, fetch())
	require.NoError(t, testutil.TryReceive(ctx, t, runErr))
}

// lateTickClock hands the ticker callback to the test, so the test can
// deliver a tick after the ticker's context is canceled.
type lateTickClock struct {
	quartz.Clock
	fetch chan func() error
}

func (c *lateTickClock) TickerFunc(ctx context.Context, _ time.Duration, f func() error, _ ...string) quartz.Waiter {
	c.fetch <- f
	return contextWaiter{ctx: ctx}
}

type contextWaiter struct {
	ctx context.Context
}

func (w contextWaiter) Wait(...string) error {
	<-w.ctx.Done()
	return w.ctx.Err()
}
