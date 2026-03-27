package agentapi

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// connectionParamsMatcher validates that a BatchUpdateWorkspaceAgentConnectionsParams
// contains exactly the expected agent IDs with corresponding UpdatedAt values.
func TestBatcher_FlushOnInterval(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	clock := quartz.NewMock(t)
	log := slogtest.Make(t, nil)

	// Trap timer resets so we can synchronize with the batcher loop.
	resetTrap := clock.Trap().TimerReset("connectionBatcher", "scheduledFlush")
	defer resetTrap.Close()

	agent1 := uuid.New()
	agent2 := uuid.New()
	now := clock.Now()

	b := NewHeartbeatBatcher(ctx, store,
		WithHeartbeatLogger(log),
		WithHeartbeatClock(clock),
		WithHeartbeatInterval(5*time.Second),
	)
	t.Cleanup(b.Close)

	// Add two updates.
	b.Add(makeUpdate(agent1, now))
	b.Add(makeUpdate(agent2, now.Add(time.Second)))

	// Expect the flush when the timer fires.
	store.EXPECT().
		BatchUpdateWorkspaceAgentConnections(
			gomock.Any(),
			matchConnectionParams(map[uuid.UUID]time.Time{
				agent1: now,
				agent2: now.Add(time.Second),
			}),
		).Return(nil)

	// Advance past the flush interval.
	clock.Advance(5 * time.Second).MustWait(ctx)
	resetTrap.MustWait(ctx).MustRelease(ctx)
}

func TestBatcher_FlushOnCapacity(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	clock := quartz.NewMock(t)
	log := slogtest.Make(t, nil)

	// Trap capacity flush reset.
	capacityResetTrap := clock.Trap().TimerReset("connectionBatcher", "capacityFlush")
	defer capacityResetTrap.Close()

	agent1 := uuid.New()
	agent2 := uuid.New()
	agent3 := uuid.New()
	now := clock.Now()

	// Expect flush when capacity is reached.
	store.EXPECT().
		BatchUpdateWorkspaceAgentConnections(
			gomock.Any(),
			matchConnectionParams(map[uuid.UUID]time.Time{
				agent1: now,
				agent2: now,
				agent3: now,
			}),
		).Return(nil)

	b := NewHeartbeatBatcher(ctx, store,
		WithHeartbeatLogger(log),
		WithHeartbeatClock(clock),
		WithHeartbeatBatchSize(3),
	)
	t.Cleanup(b.Close)

	// Add exactly 3 updates to trigger capacity flush.
	b.Add(makeUpdate(agent1, now))
	b.Add(makeUpdate(agent2, now))
	b.Add(makeUpdate(agent3, now))

	// Wait for the capacity flush reset (no clock advance needed).
	capacityResetTrap.MustWait(ctx).MustRelease(ctx)
}

func TestBatcher_DeduplicatesByAgentID(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	clock := quartz.NewMock(t)
	log := slogtest.Make(t, nil)

	resetTrap := clock.Trap().TimerReset("connectionBatcher", "scheduledFlush")
	defer resetTrap.Close()

	agentID := uuid.New()
	earlier := clock.Now()
	later := earlier.Add(10 * time.Second)

	b := NewHeartbeatBatcher(ctx, store,
		WithHeartbeatLogger(log),
		WithHeartbeatClock(clock),
		WithHeartbeatInterval(5*time.Second),
	)
	t.Cleanup(b.Close)

	// Add older update first, then newer.
	b.Add(makeUpdate(agentID, earlier))
	b.Add(makeUpdate(agentID, later))

	// Expect only the later update.
	store.EXPECT().
		BatchUpdateWorkspaceAgentConnections(
			gomock.Any(),
			matchConnectionParams(map[uuid.UUID]time.Time{
				agentID: later,
			}),
		).Return(nil)

	clock.Advance(5 * time.Second).MustWait(ctx)
	resetTrap.MustWait(ctx).MustRelease(ctx)
}

func TestBatcher_FinalFlushOnClose(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	clock := quartz.NewMock(t)
	log := slogtest.Make(t, nil)

	resetTrap := clock.Trap().TimerReset("connectionBatcher", "scheduledFlush")
	defer resetTrap.Close()

	agentID := uuid.New()
	now := clock.Now()

	// Expect two flushes: one from the timer, one from Close.
	store.EXPECT().
		BatchUpdateWorkspaceAgentConnections(gomock.Any(), gomock.Any()).
		Return(nil).Times(2)

	b := NewHeartbeatBatcher(ctx, store,
		WithHeartbeatLogger(log),
		WithHeartbeatClock(clock),
		WithHeartbeatInterval(5*time.Second),
	)

	// Add an update and flush it via timer to ensure the loop is running.
	b.Add(makeUpdate(agentID, now))
	clock.Advance(5 * time.Second).MustWait(ctx)
	resetTrap.MustWait(ctx).MustRelease(ctx)

	// Now add another update and close — the final flush should pick it up.
	agent2 := uuid.New()
	b.Add(makeUpdate(agent2, now))

	b.Close()
}

func TestBatcher_DropsWhenFull(t *testing.T) {
	t.Parallel()

	// This test verifies the non-blocking Add() behavior directly.
	// We construct a HeartbeatBatcher but only care about the channel semantics,
	// not the flush loop, so we use a real clock with a long interval.
	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	log := slogtest.Make(t, nil)

	// Allow any flushes that happen from the background loop processing.
	store.EXPECT().
		BatchUpdateWorkspaceAgentConnections(gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	// Use a large batch size so the loop doesn't capacity-flush while
	// we fill the channel. Channel buffer = 100 * 5 = 500.
	b := NewHeartbeatBatcher(ctx, store,
		WithHeartbeatLogger(log),
		WithHeartbeatBatchSize(100),
		WithHeartbeatInterval(time.Hour),
	)
	t.Cleanup(b.Close)

	now := time.Now()

	// Fill the channel buffer. The loop may consume some items, so
	// keep adding until the channel is full.
	for i := 0; i < 500; i++ {
		b.updateCh <- makeUpdate(uuid.New(), now)
	}

	// The channel is now full. Add() should not block.
	done := make(chan struct{})
	go func() {
		b.Add(makeUpdate(uuid.New(), now))
		close(done)
	}()

	select {
	case <-done:
		// Success: Add returned immediately.
	case <-time.After(time.Second):
		t.Fatal("Add() blocked when channel was full")
	}
}

type connectionParamsMatcher struct {
	expected map[uuid.UUID]time.Time // agentID -> updatedAt
}

func (m connectionParamsMatcher) Matches(x interface{}) bool {
	params, ok := x.(database.BatchUpdateWorkspaceAgentConnectionsParams)
	if !ok {
		return false
	}
	if len(params.ID) != len(m.expected) {
		return false
	}
	for i, id := range params.ID {
		expectedTime, exists := m.expected[id]
		if !exists {
			return false
		}
		if !params.UpdatedAt[i].Equal(expectedTime) {
			return false
		}
	}
	return true
}

func (connectionParamsMatcher) String() string {
	return "matches expected connection params"
}

func matchConnectionParams(expected map[uuid.UUID]time.Time) gomock.Matcher {
	return connectionParamsMatcher{expected: expected}
}

func makeUpdate(id uuid.UUID, updatedAt time.Time) HeartbeatUpdate {
	return HeartbeatUpdate{
		ID: id,
		LastConnectedAt: sql.NullTime{
			Time:  updatedAt,
			Valid: true,
		},
		UpdatedAt: updatedAt,
	}
}

