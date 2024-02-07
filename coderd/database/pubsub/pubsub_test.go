package pubsub_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/postgres"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func TestPGPubsub_Metrics(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	connectionURL, closePg, err := postgres.Open()
	require.NoError(t, err)
	defer closePg()
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
		return testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events") &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_subscribers") &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_connected") &&
			testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_publishes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_subscribes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_messages_total", "normal") &&
			testutil.PromCounterHasValue(t, metrics, 7, "coder_pubsub_received_bytes_total") &&
			testutil.PromCounterHasValue(t, metrics, 7, "coder_pubsub_published_bytes_total")
	}, testutil.WaitShort, testutil.IntervalFast)

	colossalData := make([]byte, 7600)
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
		return testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events") &&
			testutil.PromGaugeHasValue(t, metrics, 2, "coder_pubsub_current_subscribers") &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_connected") &&
			testutil.PromCounterHasValue(t, metrics, 2, "coder_pubsub_publishes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, 2, "coder_pubsub_subscribes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_messages_total", "normal") &&
			testutil.PromCounterHasValue(t, metrics, 1, "coder_pubsub_messages_total", "colossal") &&
			testutil.PromCounterHasValue(t, metrics, 7607, "coder_pubsub_received_bytes_total") &&
			testutil.PromCounterHasValue(t, metrics, 7607, "coder_pubsub_published_bytes_total")
	}, testutil.WaitShort, testutil.IntervalFast)
}
