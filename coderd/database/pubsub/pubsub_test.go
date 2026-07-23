package pubsub_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func TestPGPubsub_Metrics(t *testing.T) {
	t.Parallel()

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
	_ = testutil.TryReceive(ctx, t, messageChannel)

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
	_ = testutil.TryReceive(ctx, t, messageChannel)
	_ = testutil.TryReceive(ctx, t, messageChannel)

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
		select {
		case gotChan <- struct{}{}:
		default:
		}
	})
	require.NoError(t, err)
	defer subCancel()

	// send a message
	err = pubber.Publish("test", []byte("hello"))
	require.NoError(t, err)

	// wait for the message
	_ = testutil.TryReceive(ctx, t, gotChan)

	// read out first connection
	firstConn := testutil.TryReceive(ctx, t, subDriver.Connections)

	// drop the underlying connection being used by the pubsub
	// the pq.Listener should reconnect and repopulate it's listeners
	// so old subscriptions should still work
	err = firstConn.Close()
	require.NoError(t, err)

	// wait for the reconnect
	_ = testutil.TryReceive(ctx, t, subDriver.Connections)

	// The raw connection notification is sent before the
	// pq.Listener re-issues LISTEN on the new connection.
	// Rather than sleeping a fixed duration, retry publishing
	// until the subscriber receives a message, which proves
	// that the LISTEN has been re-established.
	testutil.Eventually(ctx, t, func(_ context.Context) bool {
		// Drain any stale signals before publishing.
		select {
		case <-gotChan:
		default:
		}
		err := pubber.Publish("test", []byte("hello-again"))
		if err != nil {
			return false
		}
		select {
		case <-gotChan:
			return true
		case <-time.After(testutil.IntervalFast):
			return false
		}
	}, testutil.IntervalMedium, "subscriber did not receive message after reconnect")
}

func Test_MsgQueue_ListenerWithError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	m := make(chan string)
	e := make(chan error)
	uut := pubsub.NewMsgQueue(ctx, nil, func(ctx context.Context, msg []byte, err error) {
		m <- string(msg)
		e <- err
	})
	defer uut.Close()

	// We're going to enqueue 4 messages and an error in a loop -- that is, a cycle of 5.
	// PubsubBufferSize is 2048, which is a power of 2, so a pattern of 5 will not be aligned
	// when we wrap around the end of the circular buffer.  This tests that we correctly handle
	// the wrapping and aren't dequeueing misaligned data.
	cycles := (pubsub.BufferSize / 5) * 2 // almost twice around the ring
	for j := 0; j < cycles; j++ {
		for i := 0; i < 4; i++ {
			uut.Enqueue([]byte(fmt.Sprintf("%d%d", j, i)))
		}
		uut.Dropped()
		for i := 0; i < 4; i++ {
			select {
			case <-ctx.Done():
				t.Fatal("timed out")
			case msg := <-m:
				require.Equal(t, fmt.Sprintf("%d%d", j, i), msg)
			}
			select {
			case <-ctx.Done():
				t.Fatal("timed out")
			case err := <-e:
				require.NoError(t, err)
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("timed out")
		case msg := <-m:
			require.Equal(t, "", msg)
		}
		select {
		case <-ctx.Done():
			t.Fatal("timed out")
		case err := <-e:
			require.ErrorIs(t, err, pubsub.ErrDroppedMessages)
		}
	}
}

func Test_MsgQueue_Listener(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	m := make(chan string)
	uut := pubsub.NewMsgQueue(ctx, func(ctx context.Context, msg []byte) {
		m <- string(msg)
	}, nil)
	defer uut.Close()

	// We're going to enqueue 4 messages and an error in a loop -- that is, a cycle of 5.
	// PubsubBufferSize is 2048, which is a power of 2, so a pattern of 5 will not be aligned
	// when we wrap around the end of the circular buffer.  This tests that we correctly handle
	// the wrapping and aren't dequeueing misaligned data.
	cycles := (pubsub.BufferSize / 5) * 2 // almost twice around the ring
	for j := 0; j < cycles; j++ {
		for i := 0; i < 4; i++ {
			uut.Enqueue([]byte(fmt.Sprintf("%d%d", j, i)))
		}
		uut.Dropped()
		for i := 0; i < 4; i++ {
			select {
			case <-ctx.Done():
				t.Fatal("timed out")
			case msg := <-m:
				require.Equal(t, fmt.Sprintf("%d%d", j, i), msg)
			}
		}
		// Listener skips over errors, so we only read out the 4 real messages.
	}
}

func Test_MsgQueue_Full(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	firstDequeue := make(chan struct{})
	allowRead := make(chan struct{})
	n := 0
	errors := make(chan error)
	uut := pubsub.NewMsgQueue(ctx, nil, func(ctx context.Context, msg []byte, err error) {
		if n == 0 {
			close(firstDequeue)
		}
		<-allowRead
		if err == nil {
			require.Equal(t, fmt.Sprintf("%d", n), string(msg))
			n++
			return
		}
		errors <- err
	})
	defer uut.Close()

	// we send 2 more than the capacity.  One extra because the call to the ListenerFunc blocks
	// but only after we've dequeued a message, and then another extra because we want to exceed
	// the capacity, not just reach it.
	for i := 0; i < pubsub.BufferSize+2; i++ {
		uut.Enqueue([]byte(fmt.Sprintf("%d", i)))
		// ensure the first dequeue has happened before proceeding, so that this function isn't racing
		// against the goroutine that dequeues items.
		<-firstDequeue
	}
	close(allowRead)

	select {
	case <-ctx.Done():
		t.Fatal("timed out")
	case err := <-errors:
		require.ErrorIs(t, err, pubsub.ErrDroppedMessages)
	}
	// Ok, so we sent 2 more than capacity, but we only read the capacity, that's because the last
	// message we send doesn't get queued, AND, it bumps a message out of the queue to make room
	// for the error, so we read 2 less than we sent.
	require.Equal(t, pubsub.BufferSize, n)
}
