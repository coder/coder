package nats_test

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	xnats "github.com/coder/coder/v2/coderd/x/nats"
	"github.com/coder/coder/v2/testutil"
)

// gaugeValue gathers metric family `name` from reg and returns the value
// of the first metric (no labels expected). Returns false when the
// family or metric is missing.
func gaugeValue(t *testing.T, reg *prometheus.Registry, name string) (float64, bool) {
	t.Helper()
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.Metric {
			switch {
			case m.Gauge != nil:
				return m.GetGauge().GetValue(), true
			case m.Counter != nil:
				return m.GetCounter().GetValue(), true
			}
		}
	}
	return 0, false
}

// TestStress_ConcurrentSubscribePublishCancel exercises many goroutines
// that subscribe, publish, and cancel concurrently against a single
// standalone Pubsub. It verifies no panic, no deadlock, that Close
// returns within DrainTimeout, and that the current_subscribers gauge
// returns to 0 after cleanup.
func TestStress_ConcurrentSubscribePublishCancel(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	drainTimeout := 5 * time.Second
	ps, err := xnats.New(ctx, logger, xnats.Options{
		DrainTimeout: drainTimeout,
	})
	require.NoError(t, err)

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(ps))

	const (
		numWorkers = 20
		iterations = 200
		numEvents  = 5
	)
	events := make([]string, numEvents)
	for i := range events {
		events[i] = fmt.Sprintf("stress_event_%d", i)
	}

	var wg sync.WaitGroup
	var dropped atomic.Int64
	workerCtx, workerCancel := context.WithTimeout(ctx, testutil.WaitLong)
	defer workerCancel()

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			//nolint:gosec // deterministic per-worker pseudo-random is fine.
			r := rand.New(rand.NewSource(seed))
			payload := []byte("x")
			for i := 0; i < iterations; i++ {
				if workerCtx.Err() != nil {
					return
				}
				subEvent := events[r.Intn(numEvents)]
				cancelSub, err := ps.SubscribeWithErr(subEvent, func(_ context.Context, _ []byte, errCb error) {
					if errCb != nil {
						dropped.Add(1)
					}
				})
				if err != nil {
					t.Errorf("subscribe: %v", err)
					return
				}
				pubEvent := events[r.Intn(numEvents)]
				if err := ps.Publish(pubEvent, payload); err != nil {
					cancelSub()
					t.Errorf("publish: %v", err)
					return
				}
				cancelSub()
			}
		}(int64(w) + 1)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-workerCtx.Done():
		t.Fatalf("workers did not finish: %v", workerCtx.Err())
	}

	// Close should complete well within DrainTimeout.
	closeStart := time.Now()
	closeDone := make(chan error, 1)
	go func() { closeDone <- ps.Close() }()
	select {
	case err := <-closeDone:
		assert.NoError(t, err)
	case <-time.After(drainTimeout + 2*time.Second):
		t.Fatalf("Close did not return within %s", drainTimeout+2*time.Second)
	}
	t.Logf("close took %s, dropped errors observed: %d", time.Since(closeStart), dropped.Load())

	// After Close, current_subscribers should be 0.
	v, ok := gaugeValue(t, reg, "coder_pubsub_current_subscribers")
	require.True(t, ok, "expected coder_pubsub_current_subscribers to be present")
	assert.Equal(t, float64(0), v, "subscribers gauge should be 0 after Close")

	v, ok = gaugeValue(t, reg, "coder_pubsub_current_events")
	require.True(t, ok)
	assert.Equal(t, float64(0), v, "events gauge should be 0 after Close")
}

// TestStress_ManySubscribersOneEvent verifies that with many
// subscribers on a single event, every subscriber receives every
// published message. Core NATS within a single connection delivers to
// each subscription independently.
func TestStress_ManySubscribersOneEvent(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	ps, err := xnats.New(ctx, logger, xnats.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })

	const (
		numSubs = 100
		numMsgs = 50
	)
	const event = "fanout_event"

	counts := make([]atomic.Int64, numSubs)
	doneChs := make([]chan struct{}, numSubs)
	cancels := make([]func(), 0, numSubs)
	for i := 0; i < numSubs; i++ {
		i := i
		doneChs[i] = make(chan struct{})
		c, err := ps.Subscribe(event, func(_ context.Context, _ []byte) {
			n := counts[i].Add(1)
			if n == numMsgs {
				close(doneChs[i])
			}
		})
		require.NoError(t, err)
		cancels = append(cancels, c)
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	payload := []byte("msg")
	for i := 0; i < numMsgs; i++ {
		require.NoError(t, ps.Publish(event, payload))
	}

	deadline := time.After(testutil.WaitLong)
	var wg sync.WaitGroup
	wg.Add(numSubs)
	for i := 0; i < numSubs; i++ {
		i := i
		go func() {
			defer wg.Done()
			select {
			case <-doneChs[i]:
			case <-deadline:
				t.Errorf("subscriber %d only received %d/%d messages",
					i, counts[i].Load(), numMsgs)
			}
		}()
	}
	wg.Wait()

	for i := 0; i < numSubs; i++ {
		assert.Equal(t, int64(numMsgs), counts[i].Load(),
			"subscriber %d count mismatch", i)
	}
}
