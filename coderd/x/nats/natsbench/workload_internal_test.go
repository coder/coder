package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func TestDropStateRecord(t *testing.T) {
	t.Parallel()

	d := newDropState()
	select {
	case <-d.ch:
		t.Fatal("drop channel closed before any drop")
	default:
	}

	d.record()
	d.record()
	require.EqualValues(t, 2, d.count.Load())
	select {
	case <-d.ch:
	default:
		t.Fatal("drop channel not closed after a drop")
	}
}

func TestListenerRecordsDrops(t *testing.T) {
	t.Parallel()

	w := &workload{
		logger: testutil.Logger(t),
		cfg:    Config{Timeout: testutil.WaitShort},
		drops:  newDropState(),
	}
	st := &subscriberState{expect: 5, tracker: newProbeTracker()}

	// A dropped-message delivery records a drop and does not count
	// toward delivery progress.
	w.listener(st)(context.Background(), nil, pubsub.ErrDroppedMessages)
	require.EqualValues(t, 1, w.drops.count.Load())
	require.EqualValues(t, 0, st.delivered.Load())
}

func TestAwaitPhaseFailsOnDrop(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	w := &workload{
		logger: testutil.Logger(t),
		cfg:    Config{Timeout: testutil.WaitLong},
		drops:  newDropState(),
	}
	w.drops.record()

	// A never-firing phase signal must lose to the already-closed drop
	// channel, so the phase fails fast instead of waiting for Timeout.
	err := w.awaitPhase(ctx, "deliver", make(chan struct{}))
	require.ErrorContains(t, err, "dropped-message signal")
}
