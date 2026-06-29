package pubsub

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/testutil"
)

func TestMetrics_RecordHelpers(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg).ForBackend(slog.Make(), BackendPostgres)
	const backend = BackendPostgres

	m.RecordPublishSuccess(10)
	m.RecordPublishFailure()
	m.RecordSubscribeSuccess()
	m.RecordSubscribeFailure()
	m.RecordReceived([]byte("hi"))
	m.RecordReceived(make([]byte, ColossalThreshold))
	m.RecordDisconnect()
	m.MarkConnected()

	m.AddEvent("a")
	m.AddSubscriber("a")
	m.AddSubscriber("a")
	m.RemoveSubscriber("a")

	metrics, err := reg.Gather()
	require.NoError(t, err)

	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_publishes_total", backend, "true"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_publishes_total", backend, "false"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 10, "coder_pubsub_published_bytes_total", backend))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_subscribes_total", backend, "true"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_subscribes_total", backend, "false"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_messages_total", backend, "normal"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_messages_total", backend, "colossal"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, float64(2+ColossalThreshold), "coder_pubsub_received_bytes_total", backend))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_disconnections_total", backend))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_connected", backend))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events", backend))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_subscribers", backend))
}

func TestMetrics_GaugesExcludeLatencyChannel(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg).ForBackend(slog.Make(), BackendNATS)
	const backend = BackendNATS

	probe := m.latencyMeasurer.latencyChannelName()
	m.AddEvent(probe)
	m.AddSubscriber(probe)
	m.AddEvent("real")
	m.AddSubscriber("real")

	metrics, err := reg.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events", backend))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_subscribers", backend))
}

func TestMetrics_MeasureOnce(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg).ForBackend(slog.Make(), BackendPostgres)
	const backend = BackendPostgres

	ctx := testutil.Context(t, testutil.WaitShort)
	mem := NewInMemory()
	defer mem.Close()

	m.measureOnce(ctx, mem)

	metrics, err := reg.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_latency_measures_total", backend))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 0, "coder_pubsub_latency_measure_errs_total", backend))
	require.Equal(t, uint64(1), testutil.PromHistogramSampleCount(t, metrics, "coder_pubsub_send_latency_seconds", backend))
	require.Equal(t, uint64(1), testutil.PromHistogramSampleCount(t, metrics, "coder_pubsub_receive_latency_seconds", backend))
}

// failPublishPubsub subscribes like the in-memory pubsub but always fails to
// publish, so a latency measurement deterministically errors.
type failPublishPubsub struct {
	Pubsub
}

func (failPublishPubsub) Publish(string, []byte) error {
	return xerrors.New("boom")
}

func TestMetrics_MeasureOnceError(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg).ForBackend(slog.Make(), BackendPostgres)
	const backend = BackendPostgres

	ctx := testutil.Context(t, testutil.WaitShort)
	mem := NewInMemory()
	defer mem.Close()

	m.measureOnce(ctx, failPublishPubsub{mem})

	metrics, err := reg.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_latency_measures_total", backend))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_latency_measure_errs_total", backend))
	require.Equal(t, uint64(0), testutil.PromHistogramSampleCount(t, metrics, "coder_pubsub_send_latency_seconds", backend))
}
