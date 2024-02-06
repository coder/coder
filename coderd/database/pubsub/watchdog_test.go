package pubsub_test

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func TestWatchdog_NoTimeout(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, time.Hour)
	mClock := clock.NewMock()
	start := time.Date(2024, 2, 5, 8, 7, 6, 5, time.UTC)
	mClock.Set(start)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fPS := newFakePubsub()
	uut := pubsub.NewWatchdogWithClock(ctx, logger, fPS, mClock)

	sub := testutil.RequireRecvCtx(ctx, t, fPS.subs)
	require.Equal(t, pubsub.EventPubsubWatchdog, sub.event)
	p := testutil.RequireRecvCtx(ctx, t, fPS.pubs)
	require.Equal(t, pubsub.EventPubsubWatchdog, p)

	// 5 min / 15 sec = 20, so do 21 ticks
	for i := 0; i < 21; i++ {
		mClock.Add(15 * time.Second)
		p = testutil.RequireRecvCtx(ctx, t, fPS.pubs)
		require.Equal(t, pubsub.EventPubsubWatchdog, p)
		mClock.Add(30 * time.Millisecond) // reasonable round-trip
		// forward the beat
		sub.listener(ctx, []byte{})
		// we shouldn't time out
		select {
		case <-uut.Timeout():
			t.Fatal("watchdog tripped")
		default:
			// OK!
		}
	}

	err := uut.Close()
	require.NoError(t, err)
}

func TestWatchdog_Timeout(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := clock.NewMock()
	start := time.Date(2024, 2, 5, 8, 7, 6, 5, time.UTC)
	mClock.Set(start)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fPS := newFakePubsub()
	uut := pubsub.NewWatchdogWithClock(ctx, logger, fPS, mClock)

	sub := testutil.RequireRecvCtx(ctx, t, fPS.subs)
	require.Equal(t, pubsub.EventPubsubWatchdog, sub.event)
	p := testutil.RequireRecvCtx(ctx, t, fPS.pubs)
	require.Equal(t, pubsub.EventPubsubWatchdog, p)

	// 5 min / 15 sec = 20, so do 19 ticks without timing out
	for i := 0; i < 19; i++ {
		mClock.Add(15 * time.Second)
		p = testutil.RequireRecvCtx(ctx, t, fPS.pubs)
		require.Equal(t, pubsub.EventPubsubWatchdog, p)
		mClock.Add(30 * time.Millisecond) // reasonable round-trip
		// we DO NOT forward the heartbeat
		// we shouldn't time out
		select {
		case <-uut.Timeout():
			t.Fatal("watchdog tripped")
		default:
			// OK!
		}
	}
	mClock.Add(15 * time.Second)
	p = testutil.RequireRecvCtx(ctx, t, fPS.pubs)
	require.Equal(t, pubsub.EventPubsubWatchdog, p)
	testutil.RequireRecvCtx(ctx, t, uut.Timeout())

	err := uut.Close()
	require.NoError(t, err)
}

type subscribe struct {
	event    string
	listener pubsub.Listener
}

type fakePubsub struct {
	pubs chan string
	subs chan subscribe
}

func (f *fakePubsub) Subscribe(event string, listener pubsub.Listener) (func(), error) {
	f.subs <- subscribe{event, listener}
	return func() {}, nil
}

func (*fakePubsub) SubscribeWithErr(string, pubsub.ListenerWithErr) (func(), error) {
	panic("should not be called")
}

func (*fakePubsub) Close() error {
	panic("should not be called")
}

func (f *fakePubsub) Publish(event string, _ []byte) error {
	f.pubs <- event
	return nil
}

func newFakePubsub() *fakePubsub {
	return &fakePubsub{
		pubs: make(chan string),
		subs: make(chan subscribe),
	}
}
