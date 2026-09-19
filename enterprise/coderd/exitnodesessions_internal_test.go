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

func TestExitNodeReplicaSessionRegistry(t *testing.T) {
	t.Parallel()

	t.Run("Stopped", func(t *testing.T) {
		t.Parallel()
		registry := newExitNodeReplicaSessionRegistry(nil)
		replicaID := uuid.New()
		registry.stop(replicaID)
		canceled := false
		registry.register(replicaID, func() { canceled = true })
		require.True(t, canceled)
	})
	t.Run("TombstoneExpiry", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t)
		registry := newExitNodeReplicaSessionRegistry(clock)
		old := uuid.New()
		registry.stop(old)
		clock.Advance(exitNodeStoppedTombstoneTTL + time.Second)
		registry.stop(uuid.New())
		require.NotContains(t, registry.stopped, old)
		require.Len(t, registry.stopped, 1)
	})
	t.Run("HeartbeatWins", func(t *testing.T) {
		t.Parallel()
		registry := newExitNodeReplicaSessionRegistry(nil)
		replicaID, exitNodeID := uuid.New(), uuid.New()
		canceled := false
		registry.register(replicaID, func() { canceled = true })
		registry.markLive(replicaID, exitNodeID)
		generation := registry.live[replicaID].generation
		registry.markLive(replicaID, exitNodeID)
		require.False(t, registry.cancelUnlessHeartbeat(replicaID, generation))
		require.False(t, canceled)
		require.True(t, registry.cancelUnlessHeartbeat(replicaID, registry.live[replicaID].generation))
		require.True(t, canceled)
	})
}

type exitNodeReaperTest struct {
	api                   *API
	store                 *dbmock.MockStore
	registry              *exitNodeReplicaSessionRegistry
	exitNodeID, replicaID uuid.UUID
}

func newExitNodeReaperTest(t *testing.T) exitNodeReaperTest {
	t.Helper()
	store := dbmock.NewMockStore(gomock.NewController(t))
	ps := pubsub.NewInMemory()
	t.Cleanup(func() { require.NoError(t, ps.Close()) })
	registry := newExitNodeReplicaSessionRegistry(nil)
	return exitNodeReaperTest{
		api: &API{
			ctx: t.Context(),
			Options: &Options{Options: &agplcoderd.Options{
				Database: store, Pubsub: ps, Logger: slogtest.Make(t, nil),
			}},
			exitNodeReplicaSessions: registry,
		},
		store: store, registry: registry, exitNodeID: uuid.New(), replicaID: uuid.New(),
	}
}

func TestExitNodeReplicaReaper(t *testing.T) {
	t.Parallel()

	f := newExitNodeReaperTest(t)
	api, store, registry, exitNodeID, replicaID := f.api, f.store, f.registry, f.exitNodeID, f.replicaID
	ctx, cancel := context.WithCancel(api.ctx)
	t.Cleanup(cancel)
	api.ctx = ctx
	clock := quartz.NewMock(t)
	tickerTrap := clock.Trap().NewTicker("exit_node_replica_reaper")
	defer tickerTrap.Close()
	firstReap := make(chan struct{})
	store.EXPECT().GetAllLiveExitNodeReplicas(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, time.Time) ([]database.ExitNodeReplica, error) {
			close(firstReap)
			return []database.ExitNodeReplica{{ID: replicaID, ExitNodeID: exitNodeID}}, nil
		})
	store.EXPECT().GetAllLiveExitNodeReplicas(gomock.Any(), gomock.Any()).Return(nil, nil)
	store.EXPECT().GetExitNodeReplicaByID(gomock.Any(), replicaID).Return(database.ExitNodeReplica{
		ID: replicaID, ExitNodeID: exitNodeID,
	}, nil)
	sessionCanceled := make(chan struct{})
	registry.register(replicaID, func() { close(sessionCanceled) })
	events := make(chan string, 1)
	unsubscribe, err := api.Pubsub.Subscribe(codersdk.ExitNodeReplicasPubsubChannel, func(_ context.Context, payload []byte) {
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

func TestExitNodeReplicaReaperRechecksHeartbeat(t *testing.T) {
	t.Parallel()

	f := newExitNodeReaperTest(t)
	api, store, registry, exitNodeID, replicaID := f.api, f.store, f.registry, f.exitNodeID, f.replicaID
	now := time.Now().UTC()
	store.EXPECT().GetAllLiveExitNodeReplicas(gomock.Any(), gomock.Any()).Return(nil, nil)
	store.EXPECT().GetExitNodeReplicaByID(gomock.Any(), replicaID).Return(database.ExitNodeReplica{
		ID: replicaID, ExitNodeID: exitNodeID, UpdatedAt: now,
	}, nil)
	canceled := false
	registry.live[replicaID] = exitNodeReplicaState{exitNodeID: exitNodeID}
	registry.register(replicaID, func() { canceled = true })

	api.reapExitNodeReplicas(now)
	require.False(t, canceled)
	require.Equal(t, exitNodeID, registry.live[replicaID].exitNodeID)
}
