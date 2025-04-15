package pubsub_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestWatchdog_NoTimeout(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fPS := newFakePubsub()

	// trap the ticker and timer.Stop() calls
	pubTrap := mClock.Trap().TickerFunc("publish")
	defer pubTrap.Close()
	subTrap := mClock.Trap().TimerStop("subscribe")
	defer subTrap.Close()

	uut := pubsub.NewWatchdogWithClock(ctx, logger, fPS, mClock)

	// wait for the ticker to be created so that we know it starts from the
	// right baseline time.
	pc, err := pubTrap.Wait(ctx)
	require.NoError(t, err)
	pc.Release()
	require.Equal(t, 15*time.Second, pc.Duration)

	// we subscribe after starting the timer, so we know the timer also starts
	// from the baseline.
	sub := testutil.RequireReceive(ctx, t, fPS.subs)
	require.Equal(t, pubsub.EventPubsubWatchdog, sub.event)

	// 5 min / 15 sec = 20, so do 21 ticks
	for i := 0; i < 21; i++ {
		d, w := mClock.AdvanceNext()
		w.MustWait(ctx)
		require.LessOrEqual(t, d, 15*time.Second)
		p := testutil.RequireReceive(ctx, t, fPS.pubs)
		require.Equal(t, pubsub.EventPubsubWatchdog, p)
		mClock.Advance(30 * time.Millisecond). // reasonable round-trip
							MustWait(ctx)
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- uut.Close()
	}()
	sc, err := subTrap.Wait(ctx) // timer.Stop() called
	require.NoError(t, err)
	sc.Release()
	err = testutil.RequireReceive(ctx, t, errCh)
	require.NoError(t, err)
}

func TestWatchdog_Timeout(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fPS := newFakePubsub()

	// trap the ticker calls
	pubTrap := mClock.Trap().TickerFunc("publish")
	defer pubTrap.Close()

	uut := pubsub.NewWatchdogWithClock(ctx, logger, fPS, mClock)

	// wait for the ticker to be created so that we know it starts from the
	// right baseline time.
	pc, err := pubTrap.Wait(ctx)
	require.NoError(t, err)
	pc.Release()
	require.Equal(t, 15*time.Second, pc.Duration)

	// we subscribe after starting the timer, so we know the timer also starts
	// from the baseline.
	sub := testutil.RequireReceive(ctx, t, fPS.subs)
	require.Equal(t, pubsub.EventPubsubWatchdog, sub.event)

	// 5 min / 15 sec = 20, so do 19 ticks without timing out
	for i := 0; i < 19; i++ {
		d, w := mClock.AdvanceNext()
		w.MustWait(ctx)
		require.LessOrEqual(t, d, 15*time.Second)
		p := testutil.RequireReceive(ctx, t, fPS.pubs)
		require.Equal(t, pubsub.EventPubsubWatchdog, p)
		mClock.Advance(30 * time.Millisecond). // reasonable round-trip
							MustWait(ctx)
		// we DO NOT forward the heartbeat
		// we shouldn't time out
		select {
		case <-uut.Timeout():
			t.Fatal("watchdog tripped")
		default:
			// OK!
		}
	}
	d, w := mClock.AdvanceNext()
	w.MustWait(ctx)
	require.LessOrEqual(t, d, 15*time.Second)
	p := testutil.RequireReceive(ctx, t, fPS.pubs)
	require.Equal(t, pubsub.EventPubsubWatchdog, p)
	testutil.RequireReceive(ctx, t, uut.Timeout())

	err = uut.Close()
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
		pubs: make(chan string, 1),
		subs: make(chan subscribe),
	}
}
