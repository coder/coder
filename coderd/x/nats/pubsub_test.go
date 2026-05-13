package nats_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	xnats "github.com/coder/coder/v2/coderd/x/nats"
	"github.com/coder/coder/v2/testutil"
)

func newTestPubsub(t *testing.T, opts xnats.Options) *xnats.Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := xnats.New(ctx, logger, opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Close()
	})
	return ps
}

func TestStandalone_RoundTrip(t *testing.T) {
	t.Parallel()
	ps := newTestPubsub(t, xnats.Options{})

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
}

func TestStandalone_SubscribeWithErr_NormalMessage(t *testing.T) {
	t.Parallel()
	ps := newTestPubsub(t, xnats.Options{})

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
}

func TestStandalone_Echo_Default(t *testing.T) {
	t.Parallel()
	ps := newTestPubsub(t, xnats.Options{})

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
}

func TestStandalone_Ordering(t *testing.T) {
	t.Parallel()
	ps := newTestPubsub(t, xnats.Options{})

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
}

func TestNewFromConn(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	sopts := &natsserver.Options{
		JetStream:  false,
		DontListen: true,
		ServerName: fmt.Sprintf("ext-%d", time.Now().UnixNano()),
		NoLog:      true,
		NoSigs:     true,
	}
	ns, err := natsserver.NewServer(sopts)
	require.NoError(t, err)
	go ns.Start()
	require.True(t, ns.ReadyForConnections(testutil.WaitShort))
	t.Cleanup(func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	nc, err := natsgo.Connect("", natsgo.InProcessServer(ns))
	require.NoError(t, err)
	t.Cleanup(func() { nc.Close() })

	ps, err := xnats.NewFromConn(logger, nc)
	require.NoError(t, err)

	got := make(chan []byte, 1)
	cancel, err := ps.Subscribe("ext_evt", func(_ context.Context, msg []byte) {
		got <- msg
	})
	require.NoError(t, err)

	require.NoError(t, ps.Publish("ext_evt", []byte("ok")))
	select {
	case msg := <-got:
		assert.Equal(t, "ok", string(msg))
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for ext message")
	}

	cancel()
	require.NoError(t, ps.Close())

	// External server should still be usable.
	assert.False(t, nc.IsClosed(), "NewFromConn must not close external connection")

	nc2, err := natsgo.Connect("", natsgo.InProcessServer(ns))
	require.NoError(t, err)
	defer nc2.Close()

	sub, err := nc2.SubscribeSync("post.close.subj")
	require.NoError(t, err)
	require.NoError(t, nc2.Publish("post.close.subj", []byte("still-alive")))
	require.NoError(t, nc2.FlushTimeout(testutil.WaitShort))
	msg, err := sub.NextMsg(testutil.WaitShort)
	require.NoError(t, err)
	assert.Equal(t, "still-alive", string(msg.Data))
}

func TestClose_Idempotent(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := xnats.New(ctx, logger, xnats.Options{})
	require.NoError(t, err)

	var first, second error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		first = ps.Close()
	}()
	wg.Wait()
	second = ps.Close()
	assert.NoError(t, first)
	assert.NoError(t, second)
}
