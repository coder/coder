package pubsub_test

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func TestRedisPubsub_Basic(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	redisServer := miniredis.RunT(t)
	ps := newRedisPubsub(ctx, t, redisServer.Addr())

	messageChannel := make(chan []byte, 1)
	cancel, err := ps.Subscribe("test", func(_ context.Context, message []byte) {
		messageChannel <- message
	})
	require.NoError(t, err)
	defer cancel()

	err = ps.Publish("test", []byte("hello"))
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), testutil.TryReceive(ctx, t, messageChannel))
}

func TestRedisPubsub_Ordering(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	redisServer := miniredis.RunT(t)
	ps := newRedisPubsub(ctx, t, redisServer.Addr())

	messageChannel := make(chan []byte, 100)
	cancel, err := ps.Subscribe("test", func(_ context.Context, message []byte) {
		// nolint:gosec
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
		messageChannel <- message
	})
	require.NoError(t, err)
	defer cancel()

	for i := 0; i < 100; i++ {
		err := ps.Publish("test", []byte(fmt.Sprintf("%d", i)))
		require.NoError(t, err)
	}
	for i := 0; i < 100; i++ {
		select {
		case <-time.After(testutil.WaitShort):
			t.Fatalf("timed out waiting for message %d", i)
		case message := <-messageChannel:
			require.Equal(t, fmt.Sprintf("%d", i), string(message))
		}
	}
}

func TestRedisPubsub_Reconnect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	redisServer, err := miniredis.Run()
	require.NoError(t, err)
	addr := redisServer.Addr()
	t.Cleanup(redisServer.Close)

	ps := newRedisPubsub(ctx, t, addr)

	const event = "test"
	messages := make(chan string, pubsub.BufferSize)
	errs := make(chan error, pubsub.BufferSize)
	cancel, err := ps.SubscribeWithErr(event, func(_ context.Context, msg []byte, err error) {
		messages <- string(msg)
		errs <- err
	})
	require.NoError(t, err)
	defer cancel()

	readOne := func() (string, error) {
		t.Helper()
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for message")
			return "", ctx.Err()
		case msg := <-messages:
			select {
			case <-ctx.Done():
				t.Fatal("timed out waiting for error")
				return "", ctx.Err()
			case err := <-errs:
				return msg, err
			}
		}
	}

	require.NoError(t, ps.Publish(event, []byte("0")))
	msg, err := readOne()
	require.NoError(t, err)
	require.Equal(t, "0", msg)

	redisServer.Close()
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return ps.Publish(event, []byte("down")) != nil
	}, testutil.IntervalFast, "redis publish did not fail after shutdown")

	restarted := miniredis.NewMiniRedis()
	require.NoError(t, restarted.StartAddr(addr))
	t.Cleanup(restarted.Close)

	next := 1
	for {
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for publish recovery")
		default:
		}
		err = ps.Publish(event, []byte(fmt.Sprintf("%d", next)))
		if err == nil {
			break
		}
		next++
		time.Sleep(testutil.IntervalFast)
	}
	firstRecovered := next
	require.Less(t, firstRecovered, pubsub.BufferSize)

	go func(start int) {
		current := start
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			_ = ps.Publish(event, []byte(fmt.Sprintf("%d", current)))
			current++
			time.Sleep(testutil.IntervalFast)
		}
	}(next + 1)

	gotDropped := false
	for {
		msg, err := readOne()
		if xerrors.Is(err, pubsub.ErrDroppedMessages) {
			gotDropped = true
			continue
		}
		require.NoError(t, err)
		value, convErr := strconv.Atoi(msg)
		require.NoError(t, convErr)
		if value >= firstRecovered {
			break
		}
	}
	require.True(t, gotDropped)
}

func TestRedisPubsub_Metrics(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	redisServer := miniredis.RunT(t)
	ps := newRedisPubsub(ctx, t, redisServer.Addr())
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(ps))

	messageChannel := make(chan []byte, 1)
	cancel, err := ps.Subscribe("test", func(_ context.Context, message []byte) {
		messageChannel <- message
	})
	require.NoError(t, err)
	defer cancel()

	err = ps.Publish("test", []byte("hello"))
	require.NoError(t, err)
	_ = testutil.TryReceive(ctx, t, messageChannel)

	var gatherCount float64
	require.Eventually(t, func() bool {
		metrics, err := registry.Gather()
		gatherCount++
		assert.NoError(t, err)
		latencyBytes := (gatherCount - 1) * pubsub.LatencyMessageLength
		return testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_events") &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_current_subscribers") &&
			testutil.PromGaugeHasValue(t, metrics, 1, "coder_pubsub_connected") &&
			testutil.PromCounterHasValue(t, metrics, gatherCount, "coder_pubsub_publishes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, gatherCount, "coder_pubsub_subscribes_total", "true") &&
			testutil.PromCounterHasValue(t, metrics, gatherCount, "coder_pubsub_messages_total", "normal") &&
			testutil.PromCounterHasValue(t, metrics, float64(len("hello"))+latencyBytes, "coder_pubsub_published_bytes_total") &&
			testutil.PromCounterHasValue(t, metrics, float64(len("hello"))+latencyBytes, "coder_pubsub_received_bytes_total") &&
			testutil.PromGaugeAssertion(t, metrics, func(in float64) bool { return in > 0 }, "coder_pubsub_send_latency_seconds") &&
			testutil.PromGaugeAssertion(t, metrics, func(in float64) bool { return in > 0 }, "coder_pubsub_receive_latency_seconds") &&
			testutil.PromCounterHasValue(t, metrics, gatherCount, "coder_pubsub_latency_measures_total") &&
			!testutil.PromCounterGathered(t, metrics, "coder_pubsub_latency_measure_errs_total")
	}, testutil.WaitShort, testutil.IntervalFast)
}

func newRedisPubsub(ctx context.Context, t testing.TB, addr string) *pubsub.RedisPubsub {
	t.Helper()
	ps, err := pubsub.NewRedis(ctx, testutil.Logger(t), "redis://"+addr)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Close()
	})
	return ps
}
