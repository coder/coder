package nats_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/nats"
	"github.com/coder/coder/v2/testutil"
)

func newPubsub(t *testing.T, opts nats.Options) *nats.Pubsub {
	t.Helper()

	if opts.ClusterPort == 0 {
		opts.ClusterPort = natsserver.RANDOM_PORT
	}

	logger := slogtest.Make(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	ps, err := nats.New(ctx, logger, opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Close()
	})
	return ps
}

func TestPubsub(t *testing.T) {
	t.Parallel()

	t.Run("RoundTrip", func(t *testing.T) {
		t.Parallel()
		ps := newPubsub(t, nats.Options{})

		got := make(chan []byte, 1)
		cancel, err := ps.Subscribe("test_event", func(_ context.Context, msg []byte) {
			got <- msg
		})
		require.NoError(t, err)
		defer cancel()

		require.NoError(t, ps.Publish("test_event", []byte("hello")))

		select {
		case msg := <-got:
			assert.Equal(t, "hello", string(msg))
		case <-time.After(testutil.WaitShort):
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("SubscribeWithErrNormalMessage", func(t *testing.T) {
		t.Parallel()
		ps := newPubsub(t, nats.Options{})

		got := make(chan []byte, 1)
		cancel, err := ps.SubscribeWithErr("evt", func(_ context.Context, msg []byte, err error) {
			assert.NoError(t, err)
			got <- msg
		})
		require.NoError(t, err)
		defer cancel()

		require.NoError(t, ps.Publish("evt", []byte("payload")))

		select {
		case msg := <-got:
			assert.Equal(t, "payload", string(msg))
		case <-time.After(testutil.WaitShort):
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("EchoDefault", func(t *testing.T) {
		t.Parallel()
		ps := newPubsub(t, nats.Options{})

		got := make(chan []byte, 1)
		cancel, err := ps.Subscribe("echo_evt", func(_ context.Context, msg []byte) {
			got <- msg
		})
		require.NoError(t, err)
		defer cancel()

		require.NoError(t, ps.Publish("echo_evt", []byte("data")))

		select {
		case msg := <-got:
			assert.Equal(t, "data", string(msg))
		case <-time.After(testutil.WaitShort):
			t.Fatal("default should echo own messages")
		}
	})

	t.Run("Ordering", func(t *testing.T) {
		t.Parallel()
		ps := newPubsub(t, nats.Options{})

		const n = 100
		got := make(chan []byte, n)
		cancel, err := ps.Subscribe("ord_evt", func(_ context.Context, msg []byte) {
			got <- msg
		})
		require.NoError(t, err)
		defer cancel()

		for i := 0; i < n; i++ {
			require.NoError(t, ps.Publish("ord_evt", []byte(fmt.Sprintf("%d", i))))
		}

		deadline := time.After(testutil.WaitLong)
		for i := 0; i < n; i++ {
			select {
			case msg := <-got:
				assert.Equal(t, fmt.Sprintf("%d", i), string(msg))
			case <-deadline:
				t.Fatalf("timed out at message %d/%d", i, n)
			}
		}
	})

	t.Run("CloseIdempotent", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		ps, err := nats.New(ctx, logger, nats.Options{})
		require.NoError(t, err)

		var first, second error
		var wg sync.WaitGroup
		wg.Go(func() {
			first = ps.Close()
		})
		wg.Wait()
		second = ps.Close()
		assert.NoError(t, first)
		assert.NoError(t, second)
	})

	t.Run("SubscribeWithErrReceivesDropError", func(t *testing.T) {
		t.Parallel()
		ps := newPubsub(t, nats.Options{
			PendingLimits: nats.PendingLimits{Msgs: 1, Bytes: 1024 * 1024},
		})

		const event = "slow_evt_sync"
		started := make(chan struct{})
		release := make(chan struct{})
		dropped := make(chan error, 1)
		var startedOnce sync.Once
		var releaseOnce sync.Once
		defer releaseOnce.Do(func() { close(release) })

		cancel, err := ps.SubscribeWithErr(event, func(_ context.Context, _ []byte, err error) {
			if err != nil {
				select {
				case dropped <- err:
				default:
				}
				return
			}
			startedOnce.Do(func() {
				close(started)
				<-release
			})
		})
		require.NoError(t, err)
		defer cancel()

		require.NoError(t, ps.Publish(event, []byte("first")))
		require.NoError(t, ps.Flush())
		select {
		case <-started:
		case <-time.After(testutil.WaitShort):
			t.Fatal("timed out waiting for first callback")
		}

		for i := 0; i < 8; i++ {
			require.NoError(t, ps.Publish(event, []byte("burst")))
		}
		require.NoError(t, ps.Flush())
		releaseOnce.Do(func() { close(release) })

		select {
		case err := <-dropped:
			assert.ErrorIs(t, err, pubsub.ErrDroppedMessages)
		case <-time.After(testutil.WaitLong):
			t.Fatal("timed out waiting for drop error")
		}
	})
}
