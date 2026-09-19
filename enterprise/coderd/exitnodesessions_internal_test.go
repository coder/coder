package coderd

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agplcoderd "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestExitNodeReplicaReaper(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	clock := quartz.NewMock(t)
	tickerTrap := clock.Trap().NewTicker("exit_node_replica_reaper")
	defer tickerTrap.Close()
	store := dbmock.NewMockStore(gomock.NewController(t))
	ps := pubsub.NewInMemory()
	t.Cleanup(func() { require.NoError(t, ps.Close()) })
	exitNodeID := uuid.New()
	replicaID := uuid.New()
	// The mock ticker discards a tick when the previous one has not been
	// consumed yet, so the test waits for the first reap before advancing
	// the clock again.
	firstReap := make(chan struct{})
	store.EXPECT().GetAllLiveExitNodeReplicas(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, time.Time) ([]database.ExitNodeReplica, error) {
			close(firstReap)
			return []database.ExitNodeReplica{{ID: replicaID, ExitNodeID: exitNodeID}}, nil
		})
	store.EXPECT().GetAllLiveExitNodeReplicas(gomock.Any(), gomock.Any()).Return(nil, nil)

	sessionCanceled := make(chan struct{})
	registry := newExitNodeReplicaSessionRegistry()
	registry.register(replicaID, func() { close(sessionCanceled) })
	api := &API{
		ctx: ctx,
		Options: &Options{Options: &agplcoderd.Options{
			Database: store,
			Pubsub:   ps,
			Logger:   slogtest.Make(t, nil),
		}},
		exitNodeReplicaSessions: registry,
	}

	events := make(chan string, 1)
	unsubscribe, err := ps.Subscribe(codersdk.ExitNodeReplicasPubsubChannel, func(_ context.Context, payload []byte) {
		events <- string(payload)
	})
	require.NoError(t, err)
	defer unsubscribe()

	api.startExitNodeReplicaReaper(clock)
	call, err := tickerTrap.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, exitNodeReplicaReaperInterval, call.Duration)
	call.MustRelease(ctx)
	waitCtx := testutil.Context(t, testutil.WaitShort)
	clock.Advance(exitNodeReplicaReaperInterval).MustWait(ctx)
	select {
	case <-firstReap:
	case <-waitCtx.Done():
		t.Fatal("first reap did not run")
	}
	clock.Advance(exitNodeReplicaReaperInterval).MustWait(ctx)

	select {
	case <-sessionCanceled:
	case <-waitCtx.Done():
		t.Fatal("replica session was not canceled")
	}
	select {
	case event := <-events:
		require.Equal(t, exitNodeID.String(), event)
	case <-waitCtx.Done():
		t.Fatal("replica staleness was not published")
	}
}
