package nats_test

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/nats"
	"github.com/coder/coder/v2/testutil"
)

func TestPubsub_Metrics(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	registry := prometheus.NewRegistry()
	uut := newPubsub(t, nats.Options{Metrics: pubsub.NewMetrics(registry)})

	// The send/receive byte and call counters are perturbed by the
	// background latency loop, so we assert lower bounds for those. The
	// current_events/current_subscribers gauges exclude the latency probe
	// channel, so they remain exact.
	const backend = "nats"
	positive := func(in float64) bool { return in > 0 }

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 0, "coder_pubsub_current_events", backend))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 0, "coder_pubsub_current_subscribers", backend))

	event := "test"
	data := "testing"
	messageChannel := make(chan []byte)
	unsub0, err := uut.Subscribe(event, func(_ context.Context, message []byte) {
		messageChannel <- message
	})
	require.NoError(t, err)
	go func() {
		err := uut.Publish(event, []byte(data))
		assert.NoError(t, err)
	}()
	_ = testutil.TryReceive(ctx, t, messageChannel)

	require.Eventually(t, func() bool {
		metrics, err = registry.Gather()
		assert.NoError(t, err)
		return testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events", backend) &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_subscribers", backend) &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_connected", backend) &&
			testutil.PromCounterAssertion(t, metrics, positive, "coder_pubsub_publishes_total", backend, "true") &&
			testutil.PromCounterAssertion(t, metrics, positive, "coder_pubsub_subscribes_total", backend, "true") &&
			testutil.PromCounterAssertion(t, metrics, positive, "coder_pubsub_messages_total", backend, "normal") &&
			testutil.PromCounterAssertion(t, metrics, positive, "coder_pubsub_received_bytes_total", backend) &&
			testutil.PromCounterAssertion(t, metrics, positive, "coder_pubsub_published_bytes_total", backend) &&
			testutil.PromHistogramSampleCount(t, metrics, "coder_pubsub_send_latency_seconds", backend) > 0 &&
			testutil.PromHistogramSampleCount(t, metrics, "coder_pubsub_receive_latency_seconds", backend) > 0 &&
			testutil.PromCounterAssertion(t, metrics, positive, "coder_pubsub_latency_measures_total", backend) &&
			testutil.PromCounterHasValue(t, metrics, 0, "coder_pubsub_latency_measure_errs_total", backend)
	}, testutil.WaitShort, testutil.IntervalFast)

	colossalSize := 7600
	colossalData := make([]byte, colossalSize)
	for i := range colossalData {
		colossalData[i] = 'q'
	}
	unsub1, err := uut.Subscribe(event, func(_ context.Context, message []byte) {
		messageChannel <- message
	})
	require.NoError(t, err)
	go func() {
		err := uut.Publish(event, colossalData)
		assert.NoError(t, err)
	}()
	// should get 2 messages because we have 2 subs
	_ = testutil.TryReceive(ctx, t, messageChannel)
	_ = testutil.TryReceive(ctx, t, messageChannel)

	require.Eventually(t, func() bool {
		metrics, err = registry.Gather()
		assert.NoError(t, err)
		return testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events", backend) &&
			testutil.PromGaugeHasValue(t, metrics, 2, "coder_pubsub_current_subscribers", backend) &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_connected", backend) &&
			testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_messages_total", backend, "colossal")
	}, testutil.WaitShort, testutil.IntervalFast)

	// Unsubscribing both local subscribers should decrement the subscriber
	// gauge back to 0, and removing the last subscriber on the event should
	// decrement the event gauge back to 0.
	unsub0()
	unsub1()

	require.Eventually(t, func() bool {
		metrics, err = registry.Gather()
		assert.NoError(t, err)
		return testutil.PromGaugeHasValue(t, metrics, 0, "coder_pubsub_current_events", backend) &&
			testutil.PromGaugeHasValue(t, metrics, 0, "coder_pubsub_current_subscribers", backend)
	}, testutil.WaitShort, testutil.IntervalFast)
}
