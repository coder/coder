package pubsub

import (
	"context"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/clock"
)

const (
	EventPubsubWatchdog = "pubsub_watchdog"
	periodHeartbeat     = 15 * time.Second
	// periodTimeout is the time without receiving a heartbeat (from any publisher) before we
	// consider the watchdog to have timed out.  There is a tradeoff here between avoiding
	// disruption due to a short-lived issue connecting to the postgres database, and restarting
	// before the consequences of a non-working pubsub are noticed by end users (e.g. being unable
	// to connect to their workspaces).
	periodTimeout = 5 * time.Minute
)

type Watchdog struct {
	ctx     context.Context
	cancel  context.CancelFunc
	logger  slog.Logger
	ps      Pubsub
	wg      sync.WaitGroup
	timeout chan struct{}

	// for testing
	clock clock.Clock
}

func NewWatchdog(ctx context.Context, logger slog.Logger, ps Pubsub) *Watchdog {
	return NewWatchdogWithClock(ctx, logger, ps, clock.NewReal())
}

// NewWatchdogWithClock returns a watchdog with the given clock.  Product code should always call NewWatchDog.
func NewWatchdogWithClock(ctx context.Context, logger slog.Logger, ps Pubsub, c clock.Clock) *Watchdog {
	ctx, cancel := context.WithCancel(ctx)
	w := &Watchdog{
		ctx:     ctx,
		cancel:  cancel,
		logger:  logger,
		ps:      ps,
		timeout: make(chan struct{}),
		clock:   c,
	}
	w.wg.Add(2)
	go w.publishLoop()
	go w.subscribeMonitor()
	return w
}

func (w *Watchdog) Close() error {
	w.cancel()
	w.wg.Wait()
	return nil
}

// Timeout returns a channel that is closed if the watchdog times out.  Note that the Timeout() chan
// will NOT be closed if the Watchdog is Close'd or its context expires, so it is important to read
// from the Timeout() chan in a select e.g.
//
// w := NewWatchDog(ctx, logger, ps)
// select {
// case <-ctx.Done():
// case <-w.Timeout():
//
//	   FreakOut()
//	}
func (w *Watchdog) Timeout() <-chan struct{} {
	return w.timeout
}

func (w *Watchdog) publishLoop() {
	defer w.wg.Done()
	tkr := w.clock.TickerFunc(w.ctx, periodHeartbeat, func() error {
		err := w.ps.Publish(EventPubsubWatchdog, []byte{})
		if err != nil {
			w.logger.Warn(w.ctx, "failed to publish heartbeat on pubsub watchdog", slog.Error(err))
		} else {
			w.logger.Debug(w.ctx, "published heartbeat on pubsub watchdog")
		}
		return err
	}, "publish")
	// ignore the error, since we log before returning the error
	_ = tkr.Wait()
}

func (w *Watchdog) subscribeMonitor() {
	defer w.wg.Done()
	tmr := w.clock.NewTimer(periodTimeout)
	defer tmr.Stop("subscribe")
	beats := make(chan struct{})
	unsub, err := w.ps.Subscribe(EventPubsubWatchdog, func(context.Context, []byte) {
		w.logger.Debug(w.ctx, "got heartbeat for pubsub watchdog")
		select {
		case <-w.ctx.Done():
		case beats <- struct{}{}:
		}
	})
	if err != nil {
		w.logger.Critical(w.ctx, "watchdog failed to subscribe", slog.Error(err))
		close(w.timeout)
		return
	}
	defer unsub()
	for {
		select {
		case <-w.ctx.Done():
			w.logger.Debug(w.ctx, "context done; exiting subscribeMonitor")
			return
		case <-beats:
			// c.f. https://pkg.go.dev/time#Timer.Reset
			if !tmr.Stop() {
				<-tmr.C
			}
			tmr.Reset(periodTimeout)
		case <-tmr.C:
			buf := new(strings.Builder)
			_ = pprof.Lookup("goroutine").WriteTo(buf, 1)
			w.logger.Critical(w.ctx, "pubsub watchdog timeout", slog.F("goroutines", buf.String()))
			close(w.timeout)
			return
		}
	}
}
