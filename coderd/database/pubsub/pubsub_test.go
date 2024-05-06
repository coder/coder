package pubsub_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func TestPGPubsub_Metrics(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	connectionURL, closePg, err := dbtestutil.Open()
	require.NoError(t, err)
	defer closePg()
	db, err := sql.Open("postgres", connectionURL)
	require.NoError(t, err)
	defer db.Close()
	registry := prometheus.NewRegistry()
	ctx := testutil.Context(t, testutil.WaitLong)

	latencyMeasureInterval := time.Second
	start := time.Now()
	uut, err := pubsub.New(ctx, logger, db, connectionURL, latencyMeasureInterval)
	require.NoError(t, err)
	defer uut.Close()

	err = registry.Register(uut)
	require.NoError(t, err)

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 0, "coder_pubsub_current_events"))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 0, "coder_pubsub_current_subscribers"))

	event := "test"
	data := "testing"
	messageChannel := make(chan []byte)
	unsub0, err := uut.Subscribe(event, func(ctx context.Context, message []byte) {
		messageChannel <- message
	})
	require.NoError(t, err)
	defer unsub0()
	go func() {
		err = uut.Publish(event, []byte(data))
		assert.NoError(t, err)
	}()
	_ = testutil.RequireRecvCtx(ctx, t, messageChannel)

	require.Eventually(t, func() bool {
		metrics, err = registry.Gather()
		assert.NoError(t, err)

		latencyMeasurements := (time.Since(start).Truncate(latencyMeasureInterval).Seconds() / latencyMeasureInterval.Seconds()) + 1
		t.Log(latencyMeasurements)
		latencyMsgSize := pubsub.LatencyMessageLength * latencyMeasurements

		return testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events") &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_subscribers") &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_connected") &&
			testutil.PromCounterHasValue(t, metrics, 1+latencyMeasurements, "coder_pubsub_publishes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, 1+latencyMeasurements, "coder_pubsub_subscribes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, 1+latencyMeasurements, "coder_pubsub_messages_total", "normal") &&
			testutil.PromCounterHasValue(t, metrics, float64(len(data))+latencyMsgSize, "coder_pubsub_received_bytes_total") &&
			testutil.PromCounterHasValue(t, metrics, float64(len(data))+latencyMsgSize, "coder_pubsub_published_bytes_total") &&
			testutil.PromGaugeAssertion(t, metrics, func(in float64) bool { return in > 0 }, "coder_pubsub_send_latency_seconds") &&
			testutil.PromGaugeAssertion(t, metrics, func(in float64) bool { return in > 0 }, "coder_pubsub_receive_latency_seconds") &&
			testutil.PromCounterHasValue(t, metrics, latencyMeasurements, "coder_pubsub_latency_measures_total") &&
			!testutil.PromCounterGathered(t, metrics, "coder_pubsub_latency_measure_errs_total")
	}, testutil.WaitShort, testutil.IntervalFast)

	colossalSize := 7600
	colossalData := make([]byte, colossalSize)
	for i := range colossalData {
		colossalData[i] = 'q'
	}
	unsub1, err := uut.Subscribe(event, func(ctx context.Context, message []byte) {
		messageChannel <- message
	})
	require.NoError(t, err)
	defer unsub1()
	go func() {
		err = uut.Publish(event, colossalData)
		assert.NoError(t, err)
	}()
	// should get 2 messages because we have 2 subs
	_ = testutil.RequireRecvCtx(ctx, t, messageChannel)
	_ = testutil.RequireRecvCtx(ctx, t, messageChannel)

	require.Eventually(t, func() bool {
		metrics, err = registry.Gather()
		assert.NoError(t, err)

		latencyMeasurements := (time.Since(start).Truncate(latencyMeasureInterval).Seconds() / latencyMeasureInterval.Seconds()) + 1
		latencyMsgSize := pubsub.LatencyMessageLength * latencyMeasurements
		t.Log(latencyMeasurements)

		return testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events") &&
			testutil.PromGaugeHasValue(t, metrics, 2, "coder_pubsub_current_subscribers") &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_connected") &&
			testutil.PromCounterHasValue(t, metrics, 2+latencyMeasurements, "coder_pubsub_publishes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, 2+latencyMeasurements, "coder_pubsub_subscribes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, 1+latencyMeasurements, "coder_pubsub_messages_total", "normal") &&
			testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_messages_total", "colossal") &&
			testutil.PromCounterHasValue(t, metrics, float64(colossalSize+len(data))+latencyMsgSize, "coder_pubsub_received_bytes_total") &&
			testutil.PromCounterHasValue(t, metrics, float64(colossalSize+len(data))+latencyMsgSize, "coder_pubsub_published_bytes_total") &&
			testutil.PromGaugeAssertion(t, metrics, func(in float64) bool { return in > 0 }, "coder_pubsub_send_latency_seconds") &&
			testutil.PromGaugeAssertion(t, metrics, func(in float64) bool { return in > 0 }, "coder_pubsub_receive_latency_seconds") &&
			testutil.PromCounterHasValue(t, metrics, latencyMeasurements, "coder_pubsub_latency_measures_total") &&
			!testutil.PromCounterGathered(t, metrics, "coder_pubsub_latency_measure_errs_total")
	}, testutil.WaitShort, testutil.IntervalFast)
}
