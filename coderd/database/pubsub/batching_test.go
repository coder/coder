package pubsub_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func TestBatchingPubsubDedicatedSenderConnection(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	connectionURL, err := dbtestutil.Open(t)
	require.NoError(t, err)

	trackedDriver := dbtestutil.NewDriver()
	defer trackedDriver.Close()
	tconn, err := trackedDriver.Connector(connectionURL)
	require.NoError(t, err)
	trackedDB := sql.OpenDB(tconn)
	defer trackedDB.Close()

	base, err := pubsub.New(ctx, logger.Named("base"), trackedDB, connectionURL)
	require.NoError(t, err)
	defer base.Close()

	listenerConn := testutil.TryReceive(ctx, t, trackedDriver.Connections)
	batched, err := pubsub.NewBatching(ctx, logger.Named("batched"), base, trackedDB, connectionURL, pubsub.BatchingConfig{
		FlushInterval: time.Hour,
		BatchSize:     1,
		QueueSize:     8,
	})
	require.NoError(t, err)
	defer batched.Close()

	senderConn := testutil.TryReceive(ctx, t, trackedDriver.Connections)
	require.NotEqual(t, fmt.Sprintf("%p", listenerConn), fmt.Sprintf("%p", senderConn))

	event := t.Name()
	messageCh := make(chan []byte, 1)
	cancel, err := batched.Subscribe(event, func(_ context.Context, message []byte) {
		messageCh <- message
	})
	require.NoError(t, err)
	defer cancel()

	require.NoError(t, batched.Publish(event, []byte("hello")))
	require.Equal(t, []byte("hello"), testutil.TryReceive(ctx, t, messageCh))
}

func TestBatchingPubsubReconnectsAfterSenderDisconnect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	connectionURL, err := dbtestutil.Open(t)
	require.NoError(t, err)

	trackedDriver := dbtestutil.NewDriver()
	defer trackedDriver.Close()
	tconn, err := trackedDriver.Connector(connectionURL)
	require.NoError(t, err)
	trackedDB := sql.OpenDB(tconn)
	defer trackedDB.Close()

	base, err := pubsub.New(ctx, logger.Named("base"), trackedDB, connectionURL)
	require.NoError(t, err)
	defer base.Close()

	_ = testutil.TryReceive(ctx, t, trackedDriver.Connections) // listener connection
	batched, err := pubsub.NewBatching(ctx, logger.Named("batched"), base, trackedDB, connectionURL, pubsub.BatchingConfig{
		FlushInterval: time.Hour,
		BatchSize:     1,
		QueueSize:     8,
	})
	require.NoError(t, err)
	defer batched.Close()

	senderConn := testutil.TryReceive(ctx, t, trackedDriver.Connections)
	event := t.Name()
	messageCh := make(chan []byte, 4)
	cancel, err := batched.Subscribe(event, func(_ context.Context, message []byte) {
		messageCh <- message
	})
	require.NoError(t, err)
	defer cancel()

	require.NoError(t, batched.Publish(event, []byte("before-disconnect")))
	require.Equal(t, []byte("before-disconnect"), testutil.TryReceive(ctx, t, messageCh))
	require.NoError(t, senderConn.Close())

	reconnected := false
	delivered := false
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		if !reconnected {
			select {
			case conn := <-trackedDriver.Connections:
				reconnected = conn != nil
			default:
			}
		}
		select {
		case <-messageCh:
		default:
		}
		if err := batched.Publish(event, []byte("after-disconnect")); err != nil {
			return false
		}
		select {
		case msg := <-messageCh:
			delivered = string(msg) == "after-disconnect"
		case <-time.After(testutil.IntervalFast):
			delivered = false
		}
		return reconnected && delivered
	}, testutil.IntervalMedium, "batched sender did not recover after disconnect")
}
