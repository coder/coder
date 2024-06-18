package clock_test

import (
	"context"
	"testing"
	"time"

	"github.com/coder/coder/v2/clock"
)

func TestTimer_NegativeDuration(t *testing.T) {
	t.Parallel()
	// nolint:gocritic // trying to avoid Coder-specific stuff with an eye toward spinning this out
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mClock := clock.NewMock(t)
	start := mClock.Now()
	trap := mClock.Trap().NewTimer()
	defer trap.Close()

	timers := make(chan *clock.Timer, 1)
	go func() {
		timers <- mClock.NewTimer(-time.Second)
	}()
	c := trap.MustWait(ctx)
	c.Release()
	// trap returns the actual passed value
	if c.Duration != -time.Second {
		t.Fatalf("expected -time.Second, got: %v", c.Duration)
	}

	tmr := <-timers
	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for timer")
	case tme := <-tmr.C:
		// the tick is the current time, not the past
		if !tme.Equal(start) {
			t.Fatalf("expected time %v, got %v", start, tme)
		}
	}
	if tmr.Stop() {
		t.Fatal("timer still running")
	}
}

func TestAfterFunc_NegativeDuration(t *testing.T) {
	t.Parallel()
	// nolint:gocritic // trying to avoid Coder-specific stuff with an eye toward spinning this out
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mClock := clock.NewMock(t)
	trap := mClock.Trap().AfterFunc()
	defer trap.Close()

	timers := make(chan *clock.Timer, 1)
	done := make(chan struct{})
	go func() {
		timers <- mClock.AfterFunc(-time.Second, func() {
			close(done)
		})
	}()
	c := trap.MustWait(ctx)
	c.Release()
	// trap returns the actual passed value
	if c.Duration != -time.Second {
		t.Fatalf("expected -time.Second, got: %v", c.Duration)
	}

	tmr := <-timers
	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for timer")
	case <-done:
		// OK!
	}
	if tmr.Stop() {
		t.Fatal("timer still running")
	}
}

func TestNewTicker(t *testing.T) {
	t.Parallel()
	// nolint:gocritic // trying to avoid Coder-specific stuff with an eye toward spinning this out
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)
	defer cancel()

	mClock := clock.NewMock(t)
	start := mClock.Now()
	trapNT := mClock.Trap().NewTicker("new")
	defer trapNT.Close()
	trapStop := mClock.Trap().TickerStop("stop")
	defer trapStop.Close()
	trapReset := mClock.Trap().TickerReset("reset")
	defer trapReset.Close()

	tickers := make(chan *clock.Ticker, 1)
	go func() {
		tickers <- mClock.NewTicker(time.Hour, "new")
	}()
	c := trapNT.MustWait(ctx)
	c.Release()
	if c.Duration != time.Hour {
		t.Fatalf("expected time.Hour, got: %v", c.Duration)
	}
	tkr := <-tickers

	for i := 0; i < 3; i++ {
		mClock.Advance(time.Hour).MustWait(ctx)
	}

	// should get first tick, rest dropped
	tTime := start.Add(time.Hour)
	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for ticker")
	case tick := <-tkr.C:
		if !tick.Equal(tTime) {
			t.Fatalf("expected time %v, got %v", tTime, tick)
		}
	}

	go tkr.Reset(time.Minute, "reset")
	c = trapReset.MustWait(ctx)
	mClock.Advance(time.Second).MustWait(ctx)
	c.Release()
	if c.Duration != time.Minute {
		t.Fatalf("expected time.Minute, got: %v", c.Duration)
	}
	mClock.Advance(time.Minute).MustWait(ctx)

	// tick should show present time, ensuring the 2 hour ticks got dropped when
	// we didn't read from the channel.
	tTime = mClock.Now()
	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for ticker")
	case tick := <-tkr.C:
		if !tick.Equal(tTime) {
			t.Fatalf("expected time %v, got %v", tTime, tick)
		}
	}

	go tkr.Stop("stop")
	trapStop.MustWait(ctx).Release()
	mClock.Advance(time.Hour).MustWait(ctx)
	select {
	case <-tkr.C:
		t.Fatal("ticker still running")
	default:
		// OK
	}

	// Resetting after stop
	go tkr.Reset(time.Minute, "reset")
	trapReset.MustWait(ctx).Release()
	mClock.Advance(time.Minute).MustWait(ctx)
	tTime = mClock.Now()
	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for ticker")
	case tick := <-tkr.C:
		if !tick.Equal(tTime) {
			t.Fatalf("expected time %v, got %v", tTime, tick)
		}
	}
}
