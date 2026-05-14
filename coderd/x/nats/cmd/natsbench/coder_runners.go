package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	codernats "github.com/coder/coder/v2/coderd/x/nats"
)

// coderSubState is the per-subscriber state shared by every Coder
// runner (non-cluster + cluster + cluster-symmetric). It exposes the
// hot-phase delivery counter, drop-signal counter, optional warmup
// state for cluster runs, and the cancel function returned by
// SubscribeWithErr so cleanup is uniform across modes.
//
// The done channel closes when count reaches expect. Subscribers with
// expect==0 (no publishers on their subject) have done pre-closed by
// newCoderSubState so they do not block the delivery wait.
type coderSubState struct {
	count    atomic.Int64
	drops    atomic.Int64
	done     chan struct{}
	expect   int64
	cancel   func()
	warmup   *warmupState // non-nil only for cluster runners
	doneOnce atomic.Bool
}

// newCoderSubState builds a coderSubState for a subscriber that expects
// `expect` deliveries on its subject. expect==0 (subject has no
// publishers) pre-closes done so the delivery wait passes it
// immediately.
func newCoderSubState(expect int64) *coderSubState {
	st := &coderSubState{
		done:   make(chan struct{}),
		expect: expect,
	}
	if expect == 0 {
		st.markDone()
	}
	return st
}

// newClusterCoderSubState is the same as newCoderSubState but attaches
// a warmupState so the callback can record per-replica warmup arrivals.
func newClusterCoderSubState(expect int64) *coderSubState {
	st := newCoderSubState(expect)
	st.warmup = &warmupState{}
	return st
}

// markDone closes done at most once; safe for callbacks racing on the
// final delivery.
func (s *coderSubState) markDone() {
	if s.doneOnce.CompareAndSwap(false, true) {
		close(s.done)
	}
}

// coderSubCallback returns a SubscribeWithErr callback that:
//   - Routes warmup-tagged payloads to st.warmup (if present); they do
//     NOT increment the hot-phase counter.
//   - Counts ErrDroppedMessages to st.drops.
//   - Records the first non-drop error in firstSubErr.
//   - Increments st.count for hot payloads and closes st.done when
//     count == expect.
func coderSubCallback(st *coderSubState, firstSubErr *atomic.Value) pubsub.ListenerWithErr {
	return func(_ context.Context, data []byte, cberr error) {
		if cberr != nil {
			if errors.Is(cberr, pubsub.ErrDroppedMessages) {
				st.drops.Add(1)
				return
			}
			firstSubErr.CompareAndSwap(nil, cberr)
			return
		}
		if isWarmupPayload(data) {
			st.warmup.mark(warmupReplicaIdx(data))
			return
		}
		n := st.count.Add(1)
		if n == st.expect {
			st.markDone()
		}
	}
}

// awaitCoderDeliveryDone blocks until every coderSubState reports done
// or timeout elapses. On timeout it builds a formatBenchTimeoutError
// with delivered/expected/drops, the first subscriber error if any,
// and emits a goroutine stack dump to stderr via awaitOrTimeout.
func awaitCoderDeliveryDone(phase string, timeout time.Duration, states []*coderSubState, firstSubErr *atomic.Value) error {
	if len(states) == 0 {
		return nil
	}
	// Build an aggregate "all done" channel by spawning a fan-in
	// goroutine. We cannot reuse a shared done channel because every
	// subscriber has its own. The goroutine exits as soon as the
	// aggregate condition holds OR the timeout has fired (the caller's
	// awaitOrTimeout fires the cancel by returning, but to make sure
	// the goroutine actually exits we use a stop channel).
	allDone := make(chan struct{})
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for _, st := range states {
			select {
			case <-st.done:
			case <-stop:
				return
			}
		}
		close(allDone)
	}()
	diag := func() string {
		var delivered, expected, drops int64
		for _, s := range states {
			delivered += s.count.Load()
			expected += s.expect
			drops += s.drops.Load()
		}
		var firstErr error
		if v := firstSubErr.Load(); v != nil {
			firstErr, _ = v.(error)
		}
		return formatBenchTimeoutError(delivered, expected, len(states), drops, firstErr).Error()
	}
	if err := awaitOrTimeout(phase, timeout, allDone, diag); err != nil {
		// Materialize the standard formatBenchTimeoutError so callers
		// can keep matching on its shape while still benefiting from
		// the phase-timeout goroutine dump (already written to stderr
		// by awaitOrTimeout).
		var delivered, expected, drops int64
		for _, s := range states {
			delivered += s.count.Load()
			expected += s.expect
			drops += s.drops.Load()
		}
		var firstErr error
		if v := firstSubErr.Load(); v != nil {
			firstErr, _ = v.(error)
		}
		return formatBenchTimeoutError(delivered, expected, len(states), drops, firstErr)
	}
	return nil
}

// publishPhaseDiagFromCoderStates builds a compact publishPhaseDiag
// snapshot used as the on-timeout diag for the publish wg.Wait phase.
// The publishedSoFar argument is the per-publisher publish total from
// the plan; we cannot measure "actually published so far" without
// adding atomic counters to every publisher goroutine, so the diag
// reports the planned total alongside delivered-so-far. The goroutine
// stacks (written by awaitOrTimeout) are the authoritative source for
// "where is Publish stuck right now".
func publishPhaseDiagFromCoderStates(states []*coderSubState, expectPublished int64, publishErr *atomic.Value, firstSubErr *atomic.Value) publishPhaseDiag {
	var delivered, expected, drops int64
	for _, s := range states {
		delivered += s.count.Load()
		expected += s.expect
		drops += s.drops.Load()
	}
	d := publishPhaseDiag{
		published:       -1, // not tracked; planned total is below
		expectPublished: expectPublished,
		delivered:       delivered,
		expectDelivered: expected,
		drops:           drops,
	}
	if v := publishErr.Load(); v != nil {
		d.firstPubErr, _ = v.(error)
	}
	if v := firstSubErr.Load(); v != nil {
		d.firstSubErr, _ = v.(error)
	}
	return d
}

// benchmarkDrainTimeout returns the Pubsub Options.DrainTimeout to
// install for a benchmark run. Caps it at 30s so an extreme -timeout
// value (e.g. -timeout=1h) does not extend Close-drain unboundedly.
// Returns a non-zero value so the caller's runBoundedCleanup is the
// authoritative bound, not the Pubsub drain default.
func benchmarkDrainTimeout(timeout time.Duration) time.Duration {
	const ceiling = 30 * time.Second
	if timeout <= 0 || timeout > ceiling {
		return ceiling
	}
	return timeout
}

// cleanupTimeout returns the per-cleanup-phase deadline. Bounded at
// 60s so cleanup never blocks the result print path for too long even
// if -timeout is set generously.
func cleanupTimeout(timeout time.Duration) time.Duration {
	const ceiling = 60 * time.Second
	if timeout <= 0 || timeout > ceiling {
		return ceiling
	}
	return timeout
}

// coderWarmupRunner is a warmupRunner backed by a slice of *Pubsub
// (one per replica). It uses publishWarmup to push the warmup payload
// through the publishing replica's Pubsub.Publish, and flushReplica to
// drive the wrapper's pool flush before each round.
type coderWarmupRunner struct {
	pubsubs []interface {
		Publish(subject string, message []byte) error
		Flush() error
	}
}

func (c *coderWarmupRunner) publishWarmup(subject string, replica int) error {
	if replica < 0 || replica >= len(c.pubsubs) {
		return xerrors.Errorf("warmup: replica %d out of range [0,%d)", replica, len(c.pubsubs))
	}
	return c.pubsubs[replica].Publish(subject, warmupPayload(replica))
}

func (c *coderWarmupRunner) flushReplica(replica int) error {
	if replica < 0 || replica >= len(c.pubsubs) {
		return xerrors.Errorf("warmup: replica %d out of range [0,%d)", replica, len(c.pubsubs))
	}
	return c.pubsubs[replica].Flush()
}

// warmupTimeout slices a sub-budget out of the per-phase timeout for
// the cluster warmup phase. We allow up to 1/3 of the per-phase
// timeout, capped at 30s, with a 250ms floor so the loop has at least
// a few rounds to settle.
func warmupTimeout(timeout time.Duration) time.Duration {
	const (
		ceiling = 30 * time.Second
		floor   = 250 * time.Millisecond
	)
	if timeout <= 0 {
		return ceiling
	}
	w := timeout / 3
	if w > ceiling {
		w = ceiling
	}
	if w < floor {
		w = floor
	}
	return w
}

// pollWarmup is the inter-round wait used by warmupSubjectsBlocking
// in the cluster runners. Chosen empirically: small enough that a
// loopback full mesh of <=10 replicas converges in a single round
// most of the time, large enough that we do not burn CPU spinning.
const pollWarmup = 20 * time.Millisecond

// runCoderClusterWarmup drives warmupSubjectsBlocking for a coder
// cluster runner. It builds the per-subject publishing-replica list
// from plan + pubReplicaOf, builds a coderWarmupRunner wrapping the
// per-replica Pubsubs, and runs the warmup with a deadline derived
// from the per-phase timeout via warmupTimeout. If the warmup soft cap
// fires, no error is returned (the loop returns nil and the hot phase
// proceeds; any actual interest-propagation failure will surface as a
// delivery shortfall under the normal -timeout path).
func runCoderClusterWarmup(
	subjects []string,
	plan subjectPlan,
	subStates []*coderSubState,
	pubsubs []*codernats.Pubsub,
	pubReplicaOf func(int) int,
	timeout time.Duration,
) error {
	if len(subStates) == 0 || len(plan.PubSubject) == 0 {
		return nil
	}
	expected := expectedWarmupMask(plan, pubReplicaOf)
	pubReplicas := pubReplicasPerSubject(plan, pubReplicaOf)
	warms := make([]*warmupState, len(subStates))
	for i, st := range subStates {
		// runCoderClusterWarmup is only called from cluster runners,
		// which use newClusterCoderSubState. Defensive: synthesize a
		// state if a runner forgot to attach one rather than panic.
		if st.warmup == nil {
			st.warmup = &warmupState{}
		}
		warms[i] = st.warmup
	}
	pp := make([]interface {
		Publish(subject string, message []byte) error
		Flush() error
	}, len(pubsubs))
	for i, p := range pubsubs {
		pp[i] = p
	}
	runner := &coderWarmupRunner{pubsubs: pp}
	deadline := time.Now().Add(warmupTimeout(timeout))
	return warmupSubjectsBlocking(subjects, expected, plan.SubSubject, warms, pubReplicas, runner, deadline, pollWarmup)
}

// closeCoderClusterConcurrent invokes Close on each *codernats.Pubsub
// concurrently and returns the first error. Concurrent Close avoids a
// serial N*30s worst-case cleanup time for a 10-replica cluster when
// every replica's drain hits its DrainTimeout simultaneously.
func closeCoderClusterConcurrent(pubsubs []*codernats.Pubsub) error {
	if len(pubsubs) == 0 {
		return nil
	}
	var (
		wg       sync.WaitGroup
		firstErr atomic.Value
	)
	for _, p := range pubsubs {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.Close(); err != nil {
				firstErr.CompareAndSwap(nil, err)
			}
		}()
	}
	wg.Wait()
	if v := firstErr.Load(); v != nil {
		err, _ := v.(error)
		return xerrors.Errorf("close coder cluster: %w", err)
	}
	return nil
}

// flushPubsubsConcurrent calls Flush on each *codernats.Pubsub in the
// set concurrently and returns the first error. Used after the
// publish phase in runCoderClusterSymmetric so a slow replica does
// not block flushing the others.
func flushPubsubsConcurrent(set map[*codernats.Pubsub]struct{}) error {
	if len(set) == 0 {
		return nil
	}
	var (
		wg       sync.WaitGroup
		firstErr atomic.Value
	)
	for p := range set {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.Flush(); err != nil {
				firstErr.CompareAndSwap(nil, err)
			}
		}()
	}
	wg.Wait()
	if v := firstErr.Load(); v != nil {
		err, _ := v.(error)
		return xerrors.Errorf("flush coder pubsubs: %w", err)
	}
	return nil
}
