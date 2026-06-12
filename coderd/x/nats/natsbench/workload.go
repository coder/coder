package natsbench

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// dropState aggregates dropped-message signals across all subscribers.
// The first signal closes ch so waiting phases can fail fast: any drop
// invalidates the run.
type dropState struct {
	count atomic.Int64
	once  sync.Once
	ch    chan struct{}
}

func newDropState() *dropState {
	return &dropState{ch: make(chan struct{})}
}

func (d *dropState) record() {
	d.count.Add(1)
	d.once.Do(func() { close(d.ch) })
}

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
	drops   *dropState

	published atomic.Int64
	// outstanding counts subscribers that have not yet reached their
	// expected delivery count; allDone closes when it hits zero.
	outstanding atomic.Int64
	allDone     chan struct{}
	doneOnce    sync.Once
}

// runWorkload executes one benchmark run on an already-built topology:
// subscribe everywhere, prove cluster readiness, release all publishers
// together, and account for every expected delivery.
func runWorkload(ctx context.Context, logger slog.Logger, top *topology, pl plan, cfg Config) (*Result, error) {
	w := &workload{
		logger:  logger,
		top:     top,
		pl:      pl,
		cfg:     cfg,
		drops:   newDropState(),
		allDone: make(chan struct{}),
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
		if err := awaitReadiness(ctx, top, pl, cfg.Timeout, trackers); err != nil {
			return nil, xerrors.Errorf("readiness gate: %w", err)
		}
	}

	start := make(chan struct{})
	pubDone, pubErrs := w.startPublishers(start)

	// The hot phase starts when the barrier opens. Both durations are
	// measured from this instant.
	hot := time.Now()
	close(start)

	if err := w.awaitPhase(ctx, "publish", pubDone); err != nil {
		return w.buildResult(time.Since(hot), time.Since(hot)), err
	}
	if err := errors.Join(pubErrs()...); err != nil {
		return w.buildResult(time.Since(hot), time.Since(hot)), xerrors.Errorf("publish: %w", err)
	}
	for _, idx := range uniqueInts(pl.pubNode) {
		if err := top.nodes[idx].Flush(); err != nil {
			return w.buildResult(time.Since(hot), time.Since(hot)), xerrors.Errorf("flush publisher node %d: %w", idx, err)
		}
	}
	publishDur := time.Since(hot)

	if err := w.awaitPhase(ctx, "deliver", w.allDone); err != nil {
		return w.buildResult(publishDur, time.Since(hot)), err
	}
	deliverDur := time.Since(hot)

	res := w.buildResult(publishDur, deliverDur)
	if res.Drops > 0 {
		return res, xerrors.Errorf("invalid run: %d dropped-message signals observed", res.Drops)
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
			return xerrors.Errorf("subscribe subscriber %d: %w", j, err)
		}
		w.cancels = append(w.cancels, cancel)
	}
	if w.outstanding.Load() == 0 {
		w.doneOnce.Do(func() { close(w.allDone) })
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
			w.drops.record()
			return
		}
		if node, ok := probeNode(message); ok {
			st.tracker.observe(node)
			return
		}
		if st.delivered.Add(1) == int64(st.expect) {
			if w.outstanding.Add(-1) == 0 {
				w.doneOnce.Do(func() { close(w.allDone) })
			}
		}
	}
}

func (w *workload) cancelAll() {
	for _, cancel := range w.cancels {
		cancel()
	}
}

// startPublishers launches one goroutine per publisher, all parked on
// the start barrier. It returns a channel closed when every publisher
// finished and an accessor for their errors (valid only after done).
func (w *workload) startPublishers(start <-chan struct{}) (<-chan struct{}, func() []error) {
	payload := make([]byte, w.cfg.PayloadSize)
	errs := make([]error, len(w.pl.perPubMsgs))
	var wg sync.WaitGroup
	for i := range w.pl.perPubMsgs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			node := w.top.nodes[w.pl.pubNode[i]]
			subject := subjectName(w.pl.pubSubject[i])
			for range w.pl.perPubMsgs[i] {
				if err := node.Publish(subject, payload); err != nil {
					errs[i] = xerrors.Errorf("publisher %d on %s: %w", i, subject, err)
					return
				}
				w.published.Add(1)
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	return done, func() []error { return errs }
}

// awaitPhase blocks until the phase signal fires, failing fast on the
// first drop signal, context cancellation, or the per-phase timeout.
// Timeouts carry full diagnostics so a stuck run is debuggable.
func (w *workload) awaitPhase(ctx context.Context, phase string, signal <-chan struct{}) error {
	timer := time.NewTimer(w.cfg.Timeout)
	defer timer.Stop()
	select {
	case <-signal:
		return nil
	case <-w.drops.ch:
		return xerrors.Errorf("invalid run: dropped-message signal during %s phase", phase)
	case <-ctx.Done():
		return xerrors.Errorf("%s phase canceled: %w", phase, ctx.Err())
	case <-timer.C:
		return xerrors.Errorf("%s phase timed out after %s:\n%s", phase, w.cfg.Timeout, w.diagnostics())
	}
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
	_, _ = fmt.Fprintf(&b, "published: %d, drop signals: %d\n", w.published.Load(), w.drops.count.Load())

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

// buildResult snapshots counters into a Result. Callers decide whether
// the run is valid; rates are computed either way for diagnostics.
func (w *workload) buildResult(publishDur, deliverDur time.Duration) *Result {
	var delivered int64
	for _, st := range w.subs {
		delivered += st.delivered.Load()
	}
	res := &Result{
		Config:           w.cfg,
		Published:        w.published.Load(),
		Delivered:        delivered,
		Drops:            w.drops.count.Load(),
		PublishDuration:  publishDur,
		DeliverDuration:  deliverDur,
		PubsPerSec:       ratePerSec(w.published.Load(), publishDur),
		DeliveriesPerSec: ratePerSec(delivered, deliverDur),
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

// uniqueInts returns the sorted unique values of a slice.
func uniqueInts(values []int) []int {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
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
