package nats //nolint:testpackage // Uses internal slow-consumer helpers.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func newMetricsPubsub(t *testing.T, opts Options) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })
	return ps
}

func gatherFamily(t *testing.T, reg *prometheus.Registry, name string) *dto.MetricFamily {
	t.Helper()
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf
		}
	}
	return nil
}

// metricValueByLabels returns the counter/gauge value for the metric in
// family that matches all of labels (other labels may exist on the
// metric only if labels is empty).
func metricValueByLabels(mf *dto.MetricFamily, labels map[string]string) (float64, bool) {
	if mf == nil {
		return 0, false
	}
	for _, m := range mf.Metric {
		match := true
		for k, v := range labels {
			found := false
			for _, lp := range m.Label {
				if lp.GetName() == k && lp.GetValue() == v {
					found = true
					break
				}
			}
			if !found {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		if c := m.Counter; c != nil {
			return c.GetValue(), true
		}
		if g := m.Gauge; g != nil {
			return g.GetValue(), true
		}
	}
	return 0, false
}

func TestMetrics_Register(t *testing.T) {
	t.Parallel()
	ps := newMetricsPubsub(t, Options{})
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(ps))

	// Subscribe + publish so all explicit counters have at least one
	// observation.
	got := make(chan []byte, 1)
	cancel, err := ps.Subscribe("metrics_evt", func(_ context.Context, msg []byte) {
		got <- msg
	})
	require.NoError(t, err)
	defer cancel()
	require.NoError(t, ps.Publish("metrics_evt", []byte("hello")))
	select {
	case <-got:
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out")
	}

	expected := []string{
		"coder_pubsub_publishes_total",
		"coder_pubsub_subscribes_total",
		"coder_pubsub_messages_total",
		"coder_pubsub_published_bytes_total",
		"coder_pubsub_received_bytes_total",
		"coder_pubsub_nats_slow_consumers_total",
		"coder_pubsub_nats_reconnects_total",
		"coder_pubsub_nats_disconnects_total",
		"coder_pubsub_nats_dropped_msgs_total",
		"coder_pubsub_current_subscribers",
		"coder_pubsub_current_events",
		"coder_pubsub_nats_pending_msgs",
		"coder_pubsub_nats_pending_bytes",
	}
	mfs, err := reg.Gather()
	require.NoError(t, err)
	got2 := make(map[string]bool, len(mfs))
	for _, mf := range mfs {
		got2[mf.GetName()] = true
	}
	for _, name := range expected {
		assert.True(t, got2[name], "missing metric family %s", name)
	}
}

func TestMetrics_PublishCounts(t *testing.T) {
	t.Parallel()
	ps := newMetricsPubsub(t, Options{})
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(ps))

	require.NoError(t, ps.Publish("pub_evt", []byte("data")))

	mf := gatherFamily(t, reg, "coder_pubsub_publishes_total")
	v, ok := metricValueByLabels(mf, map[string]string{"success": "true"})
	require.True(t, ok)
	assert.Equal(t, float64(1), v)

	bytesMf := gatherFamily(t, reg, "coder_pubsub_published_bytes_total")
	v, ok = metricValueByLabels(bytesMf, nil)
	require.True(t, ok)
	assert.Equal(t, float64(4), v)
}

func TestMetrics_MessagesSizeLabel(t *testing.T) {
	t.Parallel()
	// MaxPayload must accommodate colossalThreshold-sized payloads; the
	// embedded server default is well above 7600.
	ps := newMetricsPubsub(t, Options{})
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(ps))

	require.NoError(t, ps.Publish("size_evt", []byte("small")))
	big := make([]byte, colossalThreshold)
	require.NoError(t, ps.Publish("size_evt", big))

	mf := gatherFamily(t, reg, "coder_pubsub_messages_total")
	require.NotNil(t, mf)
	normal, _ := metricValueByLabels(mf, map[string]string{"size": messageSizeNormal})
	colossal, _ := metricValueByLabels(mf, map[string]string{"size": messageSizeColossal})
	assert.Equal(t, float64(1), normal)
	assert.Equal(t, float64(1), colossal)
}

func TestMetrics_CurrentSubscribers(t *testing.T) {
	t.Parallel()
	ps := newMetricsPubsub(t, Options{})
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(ps))

	cancel, err := ps.Subscribe("cur_evt", func(_ context.Context, _ []byte) {})
	require.NoError(t, err)

	mf := gatherFamily(t, reg, "coder_pubsub_current_subscribers")
	v, ok := metricValueByLabels(mf, nil)
	require.True(t, ok)
	assert.Equal(t, float64(1), v)

	cancel()

	ctx := testutil.Context(t, testutil.WaitShort)
	testutil.Eventually(ctx, t, func(_ context.Context) bool {
		mf := gatherFamily(t, reg, "coder_pubsub_current_subscribers")
		v, ok := metricValueByLabels(mf, nil)
		return ok && v == 0
	}, testutil.IntervalFast)
}

func TestMetrics_BoundedLabels(t *testing.T) {
	t.Parallel()
	ps := newMetricsPubsub(t, Options{})
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(ps))

	// Force at least one publish and one subscribe so vec'd counters
	// emit a metric to inspect labels.
	require.NoError(t, ps.Publish("lab_evt", []byte("x")))
	cancel, err := ps.Subscribe("lab_evt", func(_ context.Context, _ []byte) {})
	require.NoError(t, err)
	defer cancel()

	checkLabels := func(name string, allowed map[string][]string) {
		mf := gatherFamily(t, reg, name)
		require.NotNil(t, mf, "%s missing", name)
		for _, m := range mf.Metric {
			gotKeys := map[string]string{}
			for _, lp := range m.Label {
				gotKeys[lp.GetName()] = lp.GetValue()
			}
			assert.Equal(t, len(allowed), len(gotKeys), "%s: unexpected label cardinality: %v", name, gotKeys)
			for k, vs := range allowed {
				v, ok := gotKeys[k]
				assert.True(t, ok, "%s: missing label %s", name, k)
				found := false
				for _, allowedV := range vs {
					if v == allowedV {
						found = true
						break
					}
				}
				assert.True(t, found, "%s: label %s value %q not allowed (allowed=%v)", name, k, v, vs)
			}
		}
	}

	checkLabels("coder_pubsub_publishes_total", map[string][]string{"success": {"true", "false"}})
	checkLabels("coder_pubsub_subscribes_total", map[string][]string{"success": {"true", "false"}})
	checkLabels("coder_pubsub_messages_total", map[string][]string{"size": {messageSizeNormal, messageSizeColossal}})

	for _, name := range []string{
		"coder_pubsub_published_bytes_total",
		"coder_pubsub_received_bytes_total",
		"coder_pubsub_nats_slow_consumers_total",
		"coder_pubsub_nats_reconnects_total",
		"coder_pubsub_nats_disconnects_total",
		"coder_pubsub_nats_dropped_msgs_total",
		"coder_pubsub_current_subscribers",
		"coder_pubsub_current_events",
		"coder_pubsub_nats_pending_msgs",
		"coder_pubsub_nats_pending_bytes",
	} {
		mf := gatherFamily(t, reg, name)
		require.NotNil(t, mf, "%s missing", name)
		for _, m := range mf.Metric {
			assert.Empty(t, m.Label, "%s should have no labels, got %v", name, m.Label)
		}
	}
}

// TestMetrics_NATSSlowConsumer reuses the slow-consumer harness and
// asserts that slow-consumer and dropped-message counters increment.
func TestMetrics_NATSSlowConsumer(t *testing.T) {
	t.Parallel()
	ps := newSlowConsumerPubsub(t)
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(ps))

	const event = "slow_metric_evt"
	release := make(chan struct{})
	var blocked atomic.Bool

	cancel, err := ps.SubscribeWithErr(event, func(_ context.Context, _ []byte, _ error) {
		if !blocked.Swap(true) {
			<-release
		}
	})
	require.NoError(t, err)
	defer cancel()

	for i := 0; i < 100; i++ {
		require.NoError(t, ps.Publish(event, []byte("burst")))
	}
	require.NoError(t, ps.nc.FlushTimeout(testutil.WaitShort))
	close(release)

	ctx := testutil.Context(t, testutil.WaitLong)
	testutil.Eventually(ctx, t, func(_ context.Context) bool {
		mf := gatherFamily(t, reg, "coder_pubsub_nats_slow_consumers_total")
		v, ok := metricValueByLabels(mf, nil)
		if !ok || v == 0 {
			return false
		}
		mf = gatherFamily(t, reg, "coder_pubsub_nats_dropped_msgs_total")
		v, ok = metricValueByLabels(mf, nil)
		return ok && v > 0
	}, testutil.IntervalFast)
}
