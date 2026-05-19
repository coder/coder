package notifications

import (
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestMetricsSetPendingUpdatesSerializesGaugeWrites(t *testing.T) {
	t.Parallel()

	realGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_pending_updates",
		Help: "test pending updates gauge",
	})
	blockingGauge := &pendingUpdatesBlockingGauge{
		Gauge:      realGauge,
		blockValue: 3,
		entered:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	metrics := &Metrics{
		PendingUpdates:      blockingGauge,
		pendingUpdatesGauge: &pendingUpdatesGauge{Gauge: blockingGauge},
	}

	success := make(chan dispatchResult, 4)
	failure := make(chan dispatchResult, 4)
	success <- dispatchResult{}
	success <- dispatchResult{}

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		failure <- dispatchResult{}
		metrics.pendingUpdatesGauge.set(func() int { return len(success) + len(failure) })
	}()

	testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, blockingGauge.entered)

	success <- dispatchResult{}

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		metrics.pendingUpdatesGauge.set(func() int { return len(success) + len(failure) })
	}()

	close(blockingGauge.release)
	testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, firstDone)
	testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, secondDone)

	require.Equal(t, 4, len(success)+len(failure))
	require.EqualValues(t, 4, promtest.ToFloat64(metrics.PendingUpdates))
}

type pendingUpdatesBlockingGauge struct {
	prometheus.Gauge

	blockValue float64
	entered    chan struct{}
	release    chan struct{}
	once       sync.Once
}

func (g *pendingUpdatesBlockingGauge) Set(value float64) {
	if value == g.blockValue {
		g.once.Do(func() {
			close(g.entered)
			<-g.release
		})
	}
	g.Gauge.Set(value)
}
