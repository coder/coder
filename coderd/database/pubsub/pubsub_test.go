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

	logger := testutil.Logger(t)
	connectionURL, err := dbtestutil.Open(t)
	require.NoError(t, err)
	db, err := sql.Open("postgres", connectionURL)
	require.NoError(t, err)
	defer db.Close()
	registry := prometheus.NewRegistry()
	ctx := testutil.Context(t, testutil.WaitLong)

	uut, err := pubsub.New(ctx, logger, db, connectionURL)
	require.NoError(t, err)
	defer uut.Close()

	err = registry.Register(uut)
	require.NoError(t, err)

	// each Gather measures pubsub latency by publishing a message & subscribing to it
	var gatherCount float64

	metrics, err := registry.Gather()
	gatherCount++
	require.NoError(t, err)
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 0, "coder_pubsub_current_events"))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 0, "coder_pubsub_current_subscribers"))

	event := "test"
	data := "testing"
	messageChannel := make(chan []byte)
	unsub0, err := uut.Subscribe(event, func(_ context.Context, message []byte) {
		messageChannel <- message
	})
	require.NoError(t, err)
	defer unsub0()
	go func() {
		err := uut.Publish(event, []byte(data))
		assert.NoError(t, err)
	}()
	_ = testutil.RequireRecvCtx(ctx, t, messageChannel)

	require.Eventually(t, func() bool {
		latencyBytes := gatherCount * pubsub.LatencyMessageLength
		metrics, err = registry.Gather()
		gatherCount++
		assert.NoError(t, err)
		return testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events") &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_subscribers") &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_connected") &&
			testutil.PromCounterHasValue(t, metrics, gatherCount, "coder_pubsub_publishes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, gatherCount, "coder_pubsub_subscribes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, gatherCount, "coder_pubsub_messages_total", "normal") &&
			testutil.PromCounterHasValue(t, metrics, float64(len(data))+latencyBytes, "coder_pubsub_received_bytes_total") &&
			testutil.PromCounterHasValue(t, metrics, float64(len(data))+latencyBytes, "coder_pubsub_published_bytes_total") &&
			testutil.PromGaugeAssertion(t, metrics, func(in float64) bool { return in > 0 }, "coder_pubsub_send_latency_seconds") &&
			testutil.PromGaugeAssertion(t, metrics, func(in float64) bool { return in > 0 }, "coder_pubsub_receive_latency_seconds") &&
			testutil.PromCounterHasValue(t, metrics, gatherCount, "coder_pubsub_latency_measures_total") &&
			!testutil.PromCounterGathered(t, metrics, "coder_pubsub_latency_measure_errs_total")
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
	defer unsub1()
	go func() {
		err := uut.Publish(event, colossalData)
		assert.NoError(t, err)
	}()
	// should get 2 messages because we have 2 subs
	_ = testutil.RequireRecvCtx(ctx, t, messageChannel)
	_ = testutil.RequireRecvCtx(ctx, t, messageChannel)

	require.Eventually(t, func() bool {
		latencyBytes := gatherCount * pubsub.LatencyMessageLength
		metrics, err = registry.Gather()
		gatherCount++
		assert.NoError(t, err)
		return testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events") &&
			testutil.PromGaugeHasValue(t, metrics, 2, "coder_pubsub_current_subscribers") &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_connected") &&
			testutil.PromCounterHasValue(t, metrics, 1+gatherCount, "coder_pubsub_publishes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, 1+gatherCount, "coder_pubsub_subscribes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, gatherCount, "coder_pubsub_messages_total", "normal") &&
			testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_messages_total", "colossal") &&
			testutil.PromCounterHasValue(t, metrics, float64(colossalSize+len(data))+latencyBytes, "coder_pubsub_received_bytes_total") &&
			testutil.PromCounterHasValue(t, metrics, float64(colossalSize+len(data))+latencyBytes, "coder_pubsub_published_bytes_total") &&
			testutil.PromGaugeAssertion(t, metrics, func(in float64) bool { return in > 0 }, "coder_pubsub_send_latency_seconds") &&
			testutil.PromGaugeAssertion(t, metrics, func(in float64) bool { return in > 0 }, "coder_pubsub_receive_latency_seconds") &&
			testutil.PromCounterHasValue(t, metrics, gatherCount, "coder_pubsub_latency_measures_total") &&
			!testutil.PromCounterGathered(t, metrics, "coder_pubsub_latency_measure_errs_total")
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestPGPubsubDriver(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}).Leveled(slog.LevelDebug)

	connectionURL, err := dbtestutil.Open(t)
	require.NoError(t, err)

	// use a separate subber and pubber so we can keep track of listener connections
	db, err := sql.Open("postgres", connectionURL)
	require.NoError(t, err)
	defer db.Close()
	pubber, err := pubsub.New(ctx, logger, db, connectionURL)
	require.NoError(t, err)
	defer pubber.Close()

	// use a connector that sends us the connections for the subber
	subDriver := dbtestutil.NewDriver()
	defer subDriver.Close()
	tconn, err := subDriver.Connector(connectionURL)
	require.NoError(t, err)
	tcdb := sql.OpenDB(tconn)
	defer tcdb.Close()
	subber, err := pubsub.New(ctx, logger, tcdb, connectionURL)
	require.NoError(t, err)
	defer subber.Close()

	// test that we can publish and subscribe
	gotChan := make(chan struct{}, 1)
	defer close(gotChan)
	subCancel, err := subber.Subscribe("test", func(_ context.Context, _ []byte) {
		gotChan <- struct{}{}
	})
	require.NoError(t, err)
	defer subCancel()

	// send a message
	err = pubber.Publish("test", []byte("hello"))
	require.NoError(t, err)

	// wait for the message
	_ = testutil.RequireRecvCtx(ctx, t, gotChan)

	// read out first connection
	firstConn := testutil.RequireRecvCtx(ctx, t, subDriver.Connections)

	// drop the underlying connection being used by the pubsub
	// the pq.Listener should reconnect and repopulate it's listeners
	// so old subscriptions should still work
	err = firstConn.Close()
	require.NoError(t, err)

	// wait for the reconnect
	_ = testutil.RequireRecvCtx(ctx, t, subDriver.Connections)
	// we need to sleep because the raw connection notification
	// is sent before the pq.Listener can reestablish it's listeners
	time.Sleep(1 * time.Second)

	// ensure our old subscription still fires
	err = pubber.Publish("test", []byte("hello-again"))
	require.NoError(t, err)

	// wait for the message on the old subscription
	_ = testutil.RequireRecvCtx(ctx, t, gotChan)
}
