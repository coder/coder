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

	m.RecordPublishSuccess("a", 10)
	m.RecordPublishFailure("a")
	m.RecordSubscribeSuccess("a")
	m.RecordSubscribeFailure("a")
	m.RecordReceived("a", []byte("hi"))
	m.RecordReceived("a", make([]byte, ColossalThreshold))
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

	// Each measurement uses a unique sequence-suffixed channel name; both
	// must be excluded from the gauges via the shared prefix match.
	probe1 := m.latencyMeasurer.nextChannelName()
	probe2 := m.latencyMeasurer.nextChannelName()
	m.AddEvent(probe1)
	m.AddSubscriber(probe1)
	m.AddEvent(probe2)
	m.AddSubscriber(probe2)
	m.AddEvent("real")
	m.AddSubscriber("real")

	metrics, err := reg.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events", backend))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_subscribers", backend))
}

func TestMetrics_CountersExcludeLatencyChannel(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg).ForBackend(slog.Make(), BackendNATS)
	const backend = BackendNATS

	// Traffic on the latency probe channel must not pollute the
	// publish/subscribe/message/bytes counters, just as it is excluded from
	// the gauges.
	probe := m.latencyMeasurer.nextChannelName()
	m.RecordPublishSuccess(probe, 10)
	m.RecordPublishFailure(probe)
	m.RecordSubscribeSuccess(probe)
	m.RecordSubscribeFailure(probe)
	m.RecordReceived(probe, []byte("hi"))

	// Real traffic is still counted.
	m.RecordPublishSuccess("real", 5)
	m.RecordSubscribeSuccess("real")
	m.RecordReceived("real", []byte("hey"))

	metrics, err := reg.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_publishes_total", backend, "true"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 5, "coder_pubsub_published_bytes_total", backend))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_subscribes_total", backend, "true"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_messages_total", backend, "normal"))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 3, "coder_pubsub_received_bytes_total", backend))
	// The probe's publish/subscribe failures must not register either.
	require.False(t, testutil.PromCounterGathered(t, metrics, "coder_pubsub_publishes_total", backend, "false"))
	require.False(t, testutil.PromCounterGathered(t, metrics, "coder_pubsub_subscribes_total", backend, "false"))
}

func TestMetrics_RecordLatency(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg).ForBackend(slog.Make(), BackendPostgres)
	const backend = BackendPostgres

	ctx := testutil.Context(t, testutil.WaitShort)
	mem := NewInMemory()
	defer mem.Close()

	m.recordLatency(ctx, mem)

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

func TestMetrics_RecordLatencyError(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg).ForBackend(slog.Make(), BackendPostgres)
	const backend = BackendPostgres

	ctx := testutil.Context(t, testutil.WaitShort)
	mem := NewInMemory()
	defer mem.Close()

	m.recordLatency(ctx, failPublishPubsub{mem})

	metrics, err := reg.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_latency_measures_total", backend))
	require.True(t, testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_latency_measure_errs_total", backend))
	require.Equal(t, uint64(0), testutil.PromHistogramSampleCount(t, metrics, "coder_pubsub_send_latency_seconds", backend))
}

func TestMetrics_StopLatencyLoopWithoutStart(t *testing.T) {
	t.Parallel()

	m := NewMetrics(nil).ForBackend(slog.Make(), BackendPostgres)

	// A pubsub built via newWithoutListener never starts the loop but still
	// calls StopLatencyLoop on Close, so Stop must return promptly instead of
	// blocking on a loop that never ran.
	m.StopLatencyLoop()
	// Stop is idempotent.
	m.StopLatencyLoop()
}

func TestMetrics_StartStopLatencyLoop(t *testing.T) {
	t.Parallel()

	// Start the loop, then stop it while it may be mid-measurement. Stop must
	// cancel and wait for the loop to exit without deadlocking or racing. Run
	// under -race to catch regressions.
	m := NewMetrics(nil).ForBackend(slog.Make(), BackendNATS)
	mem := NewInMemory()
	defer mem.Close()

	m.StartLatencyLoop(mem)
	m.StopLatencyLoop()
	// Stop is idempotent.
	m.StopLatencyLoop()
}
