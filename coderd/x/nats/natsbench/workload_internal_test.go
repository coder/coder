package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func TestListenerRecordsDropsAsMetric(t *testing.T) {
	t.Parallel()

	w := &workload{
		logger: testutil.Logger(t),
		cfg:    Config{Timeout: testutil.WaitShort},
	}
	st := &subscriberState{expect: 5, tracker: newProbeTracker()}

	// A dropped-message delivery is counted as a signal but does not
	// advance delivery progress; drops no longer fail the run.
	w.listener(st)(context.Background(), nil, pubsub.ErrDroppedMessages)
	require.EqualValues(t, 1, w.dropSignals.Load())
	require.EqualValues(t, 0, st.delivered.Load())
}

func TestAwaitDeliveryExactCount(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	w := &workload{
		logger:       testutil.Logger(t),
		cfg:          Config{Timeout: testutil.WaitShort},
		allDone:      make(chan struct{}),
		settleWindow: testutil.WaitShort,
	}
	st := &subscriberState{expect: 3, tracker: newProbeTracker()}
	w.subs = []*subscriberState{st}
	w.outstanding.Store(1)

	// Deliver exactly the expected count: allDone fires and the deliver
	// phase finishes precisely, without waiting out the settle window.
	listener := w.listener(st)
	go func() {
		for range 3 {
			listener(ctx, []byte("x"), nil)
		}
	}()

	dur, err := w.awaitDelivery(ctx, time.Now())
	require.NoError(t, err)
	// Windows doesn't have high resolution timers, so dur can occasionally be 0 on that platform.
	require.GreaterOrEqual(t, dur, time.Duration(0))
	require.EqualValues(t, 3, w.totalDelivered())
}

func TestAwaitDeliveryQuiescence(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	w := &workload{
		logger: testutil.Logger(t),
		cfg:    Config{Timeout: testutil.WaitShort},
		// A short settle window keeps the test fast; the timeout is far
		// larger so quiescence, not the hard timeout, ends the phase.
		allDone:      make(chan struct{}),
		settleWindow: deliveryPollInterval / 2,
	}
	// A subscriber that never reaches its expectation models a dropped
	// message: allDone can never close, so the phase must complete by
	// quiescence once the delivery counter stays flat.
	w.subs = []*subscriberState{{expect: 100}}

	dur, err := w.awaitDelivery(ctx, time.Now())
	require.NoError(t, err)
	// Windows doesn't have high resolution timers, so dur can occasionally be 0 on that platform.
	require.GreaterOrEqual(t, dur, time.Duration(0))

	select {
	case <-w.allDone:
		t.Fatal("allDone closed despite a permanent shortfall")
	default:
	}
}
