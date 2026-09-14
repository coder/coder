package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// subscriberState tracks one subscriber's exact delivery accounting.
type subscriberState struct {
	expect    int
	delivered atomic.Int64
	tracker   *probeTracker
}

// workload wires subscribers and publishers onto a topology and runs
// the measured phase with exact accounting and bounded waits.
type workload struct {
	logger slog.Logger
	top    *topology
	pl     plan
	cfg    Config

	subs    []*subscriberState
	cancels []func()
	// dropSignals counts ErrDroppedMessages deliveries. It is a
	// diagnostic signal only: the authoritative loss count is Expected
	// minus Delivered, because these signals coalesce and cross-node
	// routed loss never signals at all.
	dropSignals atomic.Int64

	published atomic.Int64
	// outstanding counts subscribers that have not yet reached their
	// expected delivery count; allDone closes when it hits zero.
	outstanding atomic.Int64
	allDone     chan struct{}
	// convergence is how long the readiness gate took to propagate
	// interest across the cluster; zero for single-node runs.
	convergence time.Duration
	// settleWindow is the delivery quiescence window used when a run
	// dropped messages and can never reach its exact count. It defaults
	// to defaultSettleWindow; tests override it to stay fast.
	settleWindow time.Duration
}

// runWorkload executes one benchmark run on an already-built topology:
// subscribe everywhere, prove cluster readiness, release all publishers
// together, and account for every expected delivery.
func runWorkload(ctx context.Context, logger slog.Logger, top *topology, pl plan, cfg Config) (*Result, error) {
	w := &workload{
		logger:       logger,
		top:          top,
		pl:           pl,
		cfg:          cfg,
		allDone:      make(chan struct{}),
		settleWindow: defaultSettleWindow,
	}
	defer w.cancelAll()
	if err := w.subscribe(); err != nil {
		return nil, err
	}

	if len(top.nodes) > 1 {
		trackers := make([]*probeTracker, len(w.subs))
		for j, st := range w.subs {
			trackers[j] = st.tracker
		}
		convergence, err := awaitTopologyReady(ctx, top, pl, cfg.Timeout, trackers)
		if err != nil {
			return nil, xerrors.Errorf("readiness gate: %w", err)
		}
		w.convergence = convergence
		logger.Debug(ctx, "cluster converged", slog.F("duration", convergence))
	}

	// Both durations are measured from this instant. Goroutine spawn
	// cost is negligible against the publish phase, so the publishers
	// start as they are launched rather than behind a barrier.
	hot := time.Now()
	pubDone, pubErrCh := w.startPublishers()

	if err := w.awaitPhase(ctx, "publish", pubDone); err != nil {
		return w.buildResult(time.Since(hot), time.Since(hot)), err
	}
	// pubDone closing implies pubErrCh is already closed (both happen
	// after the publishers' WaitGroup), so this drain terminates.
	var pubErrs []error
	for err := range pubErrCh {
		pubErrs = append(pubErrs, err)
	}
	if err := errors.Join(pubErrs...); err != nil {
		return w.buildResult(time.Since(hot), time.Since(hot)), xerrors.Errorf("publish: %w", err)
	}
	for _, idx := range pl.pubNodes {
		if err := top.nodes[idx].Flush(); err != nil {
			return w.buildResult(time.Since(hot), time.Since(hot)), xerrors.Errorf("flush publisher node %d: %w", idx, err)
		}
	}
	publishDur := time.Since(hot)

	deliverDur, err := w.awaitDelivery(ctx, hot)
	if err != nil {
		return w.buildResult(publishDur, deliverDur), err
	}

	res := w.buildResult(publishDur, deliverDur)
	if res.Drops > 0 {
		logger.Warn(ctx, "run dropped messages",
			slog.F("expected", res.Expected),
			slog.F("delivered", res.Delivered),
			slog.F("drops", res.Drops),
			slog.F("drop_signals", w.dropSignals.Load()),
		)
	}
	return res, nil
}

// subscribe registers every subscriber. SubscribeWithErr flushes the
// SUB on its subscribe connection before returning, so once this
// returns every subscriber's interest is registered with its local
// server; cross-node interest propagation is proven separately by the
// readiness gate.
func (w *workload) subscribe() error {
	for j := range w.pl.subSubject {
		st := &subscriberState{
			expect:  w.pl.expectPerSub[j],
			tracker: newProbeTracker(),
		}
		w.subs = append(w.subs, st)
		if st.expect > 0 {
			w.outstanding.Add(1)
		}
		node := w.top.nodes[w.pl.subNode[j]]
		cancel, err := node.SubscribeWithErr(subjectName(w.pl.subSubject[j]), w.listener(st))
		if err != nil {
			return xerrors.Errorf("register subscriber %d: %w", j, err)
		}
		w.cancels = append(w.cancels, cancel)
	}
	return nil
}

// listener builds the delivery callback for one subscriber: probes feed
// the readiness tracker, errors poison the run, and benchmark payloads
// count toward the exact expected total.
func (w *workload) listener(st *subscriberState) pubsub.ListenerWithErr {
	return func(ctx context.Context, message []byte, err error) {
		if err != nil {
			if !xerrors.Is(err, pubsub.ErrDroppedMessages) {
				w.logger.Error(ctx, "unexpected subscriber error", slog.Error(err))
			}
			w.dropSignals.Add(1)
			return
		}
		if node, ok := probeNode(message); ok {
			st.tracker.observe(node)
			return
		}
		// Each subscriber decrements outstanding exactly once, when its
		// delivered count first equals its expectation, so the atomic
		// reaches zero exactly once and closes allDone exactly once.
		if st.delivered.Add(1) == int64(st.expect) {
			if w.outstanding.Add(-1) == 0 {
				close(w.allDone)
			}
		}
	}
}

func (w *workload) cancelAll() {
	for _, cancel := range w.cancels {
		cancel()
	}
}

// startPublishers launches one goroutine per publisher and returns a
// channel closed when every publisher finished and a channel of publish
// errors that is closed at the same time. The error channel is buffered
// to the publisher count so a failing publisher never blocks, and each
// publisher sends at most one error.
func (w *workload) startPublishers() (<-chan struct{}, <-chan error) {
	payload := make([]byte, w.cfg.PayloadSize)
	errCh := make(chan error, len(w.pl.perPubMsgs))
	var wg sync.WaitGroup
	for i := range w.pl.perPubMsgs {
		wg.Go(func() {
			node := w.top.nodes[w.pl.pubNode[i]]
			subject := subjectName(w.pl.pubSubject[i])
			for range w.pl.perPubMsgs[i] {
				if err := node.Publish(subject, payload); err != nil {
					errCh <- xerrors.Errorf("publisher %d on %s: %w", i, subject, err)
					return
				}
				w.published.Add(1)
			}
		})
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(errCh)
		close(done)
	}()
	return done, errCh
}

// awaitPhase blocks until the phase signal fires, failing on context
// cancellation or the per-phase timeout. Timeouts carry full
// diagnostics so a stuck run is debuggable. Dropped messages do not
// fail a phase: they are accounted as a metric, not a failure.
func (w *workload) awaitPhase(ctx context.Context, phase string, signal <-chan struct{}) error {
	timer := time.NewTimer(w.cfg.Timeout)
	defer timer.Stop()
	select {
	case <-signal:
		return nil
	case <-ctx.Done():
		return xerrors.Errorf("%s phase canceled: %w", phase, ctx.Err())
	case <-timer.C:
		return xerrors.Errorf("%s phase timed out after %s:\n%s", phase, w.cfg.Timeout, w.diagnostics())
	}
}

// deliveryPollInterval is how often awaitDelivery samples the delivery
// counter to detect quiescence. It only bounds the precision of the
// quiescence path; the exact-count fast path fires immediately on
// allDone regardless of this cadence.
const deliveryPollInterval = 100 * time.Millisecond

// defaultSettleWindow is how long the delivery counter must stay flat
// before a run that dropped messages is declared complete. It is a fixed
// internal constant rather than a knob: only a run that drops messages
// ever waits it out, and on loopback a backlogged subscriber catches up
// in milliseconds, so five seconds is ample headroom. Too short would
// overcount drops; too long only slows runs that already dropped.
const defaultSettleWindow = 5 * time.Second

// awaitDelivery waits for the deliver phase to finish and returns its
// duration measured from hot.
//
// A zero-drop run reaches its exact expected count, closing allDone, and
// finishes precisely with no settle delay. A run that dropped messages
// can never reach that count, so it instead completes by quiescence: the
// total delivered counter is polled, and once it has not advanced for
// settleWindow the phase is declared complete. The duration is measured
// to the last observed progress, not to the end of the settle window, so
// the idle wait never inflates the delivery rate.
//
// The counter is read by summing the per-subscriber delivery atomics, so
// no global counter or per-delivery timestamp is added to the hot
// delivery path.
func (w *workload) awaitDelivery(ctx context.Context, hot time.Time) (time.Duration, error) {
	timer := time.NewTimer(w.cfg.Timeout)
	defer timer.Stop()
	poll := time.NewTicker(deliveryPollInterval)
	defer poll.Stop()

	lastCount := w.totalDelivered()
	lastProgress := time.Now()
	for {
		select {
		case <-w.allDone:
			return time.Since(hot), nil
		case <-ctx.Done():
			return time.Since(hot), xerrors.Errorf("deliver phase canceled: %w", ctx.Err())
		case <-timer.C:
			return time.Since(hot), xerrors.Errorf("deliver phase timed out after %s:\n%s", w.cfg.Timeout, w.diagnostics())
		case now := <-poll.C:
			count := w.totalDelivered()
			if count != lastCount {
				lastCount = count
				lastProgress = now
				continue
			}
			if now.Sub(lastProgress) >= w.settleWindow {
				return lastProgress.Sub(hot), nil
			}
		}
	}
}

// totalDelivered sums every subscriber's delivered count. It is called
// from the quiescence poller, off the hot delivery path.
func (w *workload) totalDelivered() int64 {
	var total int64
	for _, st := range w.subs {
		total += st.delivered.Load()
	}
	return total
}

// diagnostics renders subscriber shortfalls, per-node server stats, and
// a goroutine dump for timeout errors.
func (w *workload) diagnostics() string {
	var b strings.Builder
	const maxShortfalls = 20
	short := 0
	for j, st := range w.subs {
		got := st.delivered.Load()
		if got >= int64(st.expect) {
			continue
		}
		short++
		if short <= maxShortfalls {
			_, _ = fmt.Fprintf(&b, "subscriber %d (subject %s, node %d): delivered %d of %d\n",
				j, subjectName(w.pl.subSubject[j]), w.pl.subNode[j], got, st.expect)
		}
	}
	if short > maxShortfalls {
		_, _ = fmt.Fprintf(&b, "... and %d more subscribers short\n", short-maxShortfalls)
	}
	_, _ = fmt.Fprintf(&b, "published: %d, drop signals: %d\n", w.published.Load(), w.dropSignals.Load())

	for i, node := range w.top.nodes {
		varz, err := node.Server.Varz(&natsserver.VarzOptions{})
		if err != nil {
			_, _ = fmt.Fprintf(&b, "node %d: varz error: %v\n", i, err)
			continue
		}
		_, _ = fmt.Fprintf(&b, "node %d: connections=%d routes=%d subscriptions=%d slow_consumers=%d in_msgs=%d out_msgs=%d\n",
			i, varz.Connections, varz.Routes, varz.Subscriptions, varz.SlowConsumers, varz.InMsgs, varz.OutMsgs)
	}

	_, _ = b.WriteString("goroutine dump:\n")
	_, _ = b.WriteString(goroutineDump())
	return b.String()
}

// buildResult snapshots counters into a Result. Drops is the exact
// shortfall between expected and observed deliveries, the authoritative
// loss count. Rates are computed for valid and dropping runs alike,
// since a dropping run still has a meaningful throughput for what it
// delivered.
func (w *workload) buildResult(publishDur, deliverDur time.Duration) *Result {
	delivered := w.totalDelivered()
	expected := int64(w.pl.totalExpected)
	res := &Result{
		Config:              w.cfg,
		Expected:            expected,
		Published:           w.published.Load(),
		Delivered:           delivered,
		Drops:               max(0, expected-delivered),
		ConvergenceDuration: w.convergence,
		PublishDuration:     publishDur,
		DeliverDuration:     deliverDur,
		PubsPerSec:          ratePerSec(w.published.Load(), publishDur),
		DeliveriesPerSec:    ratePerSec(delivered, deliverDur),
	}
	return res
}

// ratePerSec computes count/dur, returning 0 for non-positive
// durations.
func ratePerSec(count int64, dur time.Duration) float64 {
	if dur <= 0 {
		return 0
	}
	return float64(count) / dur.Seconds()
}

// goroutineDump captures all goroutine stacks, growing the buffer until
// the dump fits or a hard cap is reached.
func goroutineDump() string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) || len(buf) >= 16<<20 {
			return string(buf[:n])
		}
		buf = make([]byte, len(buf)*2)
	}
}
