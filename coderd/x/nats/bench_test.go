//nolint:testpackage
package nats

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

// Upstream comparison: NATS server upstream maintains a microbenchmark
// suite at https://github.com/nats-io/nats-server/tree/main/bench and
// https://github.com/nats-io/nats-server/blob/main/server/bench_test.go.
// Those benches report figures on the order of millions of small
// messages per second against a raw *nats.Conn on a single subject /
// single subscriber. They are NOT directly comparable to these
// benchmarks because:
//   - We measure the coderd/x/nats wrapper end-to-end: Publish ->
//     subject mapping -> client flush -> server delivery -> subscriber
//     goroutine -> ListenerWithErr callback.
//   - We exercise fan-out (one publisher, N subscribers) and a
//     clustered topology over loopback routes.
// The upstream numbers are useful only as a sanity floor.
//
// These benches are fire-and-forget: the publisher loop times pure
// publish throughput (b.N publishes in a tight loop) without waiting
// for subscriber acks. A separate atomic counter tallies deliveries
// asynchronously, and the bench drains for up to drainTimeout after
// the publish loop finishes. Earlier revisions of this file used a
// lock-step "publish one, wait for every subscriber to deliver before
// the next publish" approach. That throttled the publisher to the
// slowest subscriber's delivery rate and didn't reflect how real
// callers use Pubsub (which is fire-and-forget). The new shape
// reports both publisher throughput (MB/s, pubs/s) and observed
// delivery throughput / completeness separately, so the inevitable
// gap between them is visible as data rather than hidden behind
// synthetic backpressure.
//
// Metrics reported per leaf:
//
//	MB/s          - publisher ingress (Go built-in via b.SetBytes);
//	                bytes Publish() accepted per second.
//	deliveryMB/s  - aggregate fan-out delivery bandwidth
//	                (deliveries * payload / totalElapsed). Higher than
//	                MB/s because each publish fans out to totalSubs
//	                subscribers.
//	pubs/s        - rate at which Publish() returned successfully
//	                during the publish loop.
//	deliveries/s  - rate at which subscriber callbacks ran (publish +
//	                drain time).
//	delivery_pct  - 100 * delivered / (b.N * totalSubs); <100 means
//	                drain timed out before all deliveries arrived.
//	                The harness fails the leaf in that case rather
//	                than carrying state forward into the next pass.
//	drop_events   - number of ErrDroppedMessages callbacks observed.
//	                NATS coalesces multiple actual drops into a single
//	                callback per slow-consumer event, so this is a
//	                lower bound on lost messages, not an exact count.

const (
	// drainTimeout bounds how long we wait for in-flight deliveries
	// after the publish loop completes.
	drainTimeout = 60 * time.Second
	// benchMaxPayload is the configured NATS MaxPayload so a 512 KiB
	// payload always fits regardless of upstream default drift.
	benchMaxPayload int32 = 1 << 20
	// benchPendingBytes is a generous per-subscription byte limit
	// (512 MiB) chosen so the fire-and-forget loop can flood the
	// subscriber pending queue without immediate drops at the swept
	// fan-out sizes. NATS rejects a non-zero Bytes with a zero Msgs,
	// so PendingLimits.Msgs is set to -1 (unlimited).
	benchPendingBytes = 512 << 20
)

// makePayload returns a deterministic, non-zero byte slice of the
// requested size.
func makePayload(size int) []byte {
	return bytes.Repeat([]byte("x"), size)
}

func benchPendingLimits() PendingLimits {
	return PendingLimits{Msgs: -1, Bytes: benchPendingBytes}
}

// newBenchSingleNode returns a single-node (cluster-of-1) Pubsub with
// bench-specific MaxPayload and per-subscription PendingLimits.
func newBenchSingleNode(b testing.TB) *Pubsub {
	b.Helper()
	logger := slogtest.Make(b, &slogtest.Options{IgnoreErrors: true}).
		Leveled(slog.LevelError)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	p, err := New(ctx, logger, Options{
		MaxPayload:    benchMaxPayload,
		PendingLimits: benchPendingLimits(),
		ReadyTimeout:  testutil.WaitMedium,
	})
	if err != nil {
		b.Fatalf("new single-node pubsub: %v", err)
	}
	b.Cleanup(func() { _ = p.Close() })
	return p
}

// newBenchCluster brings up an N-node full-mesh cluster on loopback
// and waits for routes to converge. The shared buildClusterPubsub
// helper does not let us configure MaxPayload / PendingLimits, so we
// call New directly here instead of modifying the shared helper.
func newBenchCluster(b testing.TB, replicas int) []*Pubsub {
	b.Helper()
	if replicas < 2 {
		b.Fatalf("newBenchCluster requires >= 2 replicas, got %d", replicas)
	}
	ports := make([]int, replicas)
	urls := make([]string, replicas)
	for i := range replicas {
		ports[i] = freePort(b)
		urls[i] = "nats://127.0.0.1:" + strconv.Itoa(ports[i])
	}
	const token = "bench-cluster-token"
	nodes := make([]*Pubsub, replicas)
	for i := range replicas {
		peers := make([]Peer, 0, replicas-1)
		for j := range replicas {
			if i == j {
				continue
			}
			peers = append(peers, Peer{RouteURL: urls[j]})
		}
		name := fmt.Sprintf("bench-node-%d", i)
		logger := slogtest.Make(b, &slogtest.Options{IgnoreErrors: true}).
			Named(name).Leveled(slog.LevelError)
		opts := Options{
			ServerName:       name,
			ClusterName:      "bench-cluster",
			ClusterToken:     token,
			ClusterHost:      "127.0.0.1",
			ClusterPort:      ports[i],
			ClusterAdvertise: "127.0.0.1:" + strconv.Itoa(ports[i]),
			PeerProvider:     StaticPeerProvider(peers),
			MaxPayload:       benchMaxPayload,
			PendingLimits:    benchPendingLimits(),
			ReadyTimeout:     testutil.WaitMedium,
		}
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		p, err := New(ctx, logger, opts)
		cancel()
		if err != nil {
			b.Fatalf("new bench cluster node %d: %v", i, err)
		}
		b.Cleanup(func() { _ = p.Close() })
		nodes[i] = p
	}
	for _, n := range nodes {
		waitForRoutes(b, n, replicas-1)
	}
	return nodes
}

// deliveryCounter is the runtime counter the per-run subscriber
// callback increments. It is swapped via atomic.Pointer between
// priming (waitInterest) and the timed run.
type deliveryCounter struct {
	count      atomic.Int64
	target     int64
	done       chan struct{}
	doneClosed atomic.Bool
}

func (c *deliveryCounter) add() {
	v := c.count.Add(1)
	if v >= c.target && c.doneClosed.CompareAndSwap(false, true) {
		close(c.done)
	}
}

// fanoutHarness owns the long-lived state for a benchmark leaf: the
// nodes, the per-subscriber pending counters, and a passID used to
// generate a fresh subject for each testing.B calibration pass.
//
// testing.B re-enters the leaf function multiple times with growing
// b.N. If we reused the same subject across passes, an earlier pass
// whose drain timed out could leak in-flight deliveries into the next
// pass's counter, producing delivery_pct > 100 or inflated
// deliveries/s. To prevent that, every pass gets a unique subject and
// freshly registered subscriptions; old subscriptions from prior
// passes are torn down before the new ones come up.
//
// drop_events counts the number of ErrDroppedMessages callbacks
// observed across the subscriptions active during the current pass.
// NATS coalesces multiple actual drops into a single callback per
// slow-consumer event, so this is a lower bound on lost messages, not
// an exact count.
type fanoutHarness struct {
	nodes       []*Pubsub
	subsPerNode int
	passID      atomic.Uint64

	// subject, primeSubj, counter, primeCount, drops, primeDrops, and
	// cancels are all per-pass state. They are reset by setupPass at
	// the start of every calibration pass.
	subject    string
	primeSubj  string
	counter    atomic.Pointer[deliveryCounter]
	primeCount atomic.Pointer[deliveryCounter]
	dropEvents atomic.Int64
	primeDrops atomic.Int64
	cancels    []func()
}

func newFanoutHarness(nodes []*Pubsub, subsPerNode int) *fanoutHarness {
	return &fanoutHarness{
		nodes:       nodes,
		subsPerNode: subsPerNode,
	}
}

// setupPass tears down any prior pass's subscriptions, picks a fresh
// per-pass subject, and registers a new set of subscribers on every
// node. The caller is expected to follow this with waitInterest and
// then the timed publish/drain loop.
func (h *fanoutHarness) setupPass(b testing.TB, leafTag string) {
	b.Helper()
	// Tear down prior pass's subscriptions if any.
	for _, c := range h.cancels {
		c()
	}
	h.cancels = h.cancels[:0]
	h.counter.Store(nil)
	h.primeCount.Store(nil)
	h.dropEvents.Store(0)
	h.primeDrops.Store(0)

	id := h.passID.Add(1)
	h.subject = fmt.Sprintf("%s_p%d", leafTag, id)
	h.primeSubj = h.subject + "_prime"

	if cap(h.cancels) < 2*len(h.nodes)*h.subsPerNode {
		h.cancels = make([]func(), 0, 2*len(h.nodes)*h.subsPerNode)
	}
	for _, n := range h.nodes {
		for range h.subsPerNode {
			cancel, err := n.SubscribeWithErr(h.subject, func(_ context.Context, _ []byte, err error) {
				if err != nil {
					if errors.Is(err, pubsub.ErrDroppedMessages) {
						h.dropEvents.Add(1)
					}
					return
				}
				if c := h.counter.Load(); c != nil {
					c.add()
				}
			})
			if err != nil {
				b.Fatalf("subscribe: %v", err)
			}
			h.cancels = append(h.cancels, cancel)

			primeCancel, err := n.SubscribeWithErr(h.primeSubj, func(_ context.Context, _ []byte, err error) {
				if err != nil {
					if errors.Is(err, pubsub.ErrDroppedMessages) {
						h.primeDrops.Add(1)
					}
					return
				}
				if c := h.primeCount.Load(); c != nil {
					c.add()
				}
			})
			if err != nil {
				b.Fatalf("subscribe prime: %v", err)
			}
			h.cancels = append(h.cancels, primeCancel)
		}
	}
}

// waitInterest publishes priming messages on a separate subject and
// waits for every subscriber across all nodes to acknowledge one. Any
// route-propagation churn that emits extra priming deliveries goes to
// the priming counter, not the runtime counter, so it can't pollute
// the timed run's tally.
func (h *fanoutHarness) waitInterest(b testing.TB, publisher *Pubsub, total int, payload []byte) {
	b.Helper()
	deadline := time.Now().Add(testutil.WaitLong)
	for time.Now().Before(deadline) {
		c := &deliveryCounter{target: int64(total), done: make(chan struct{})}
		h.primeCount.Store(c)
		if err := publisher.Publish(h.primeSubj, payload); err != nil {
			b.Fatalf("priming publish: %v", err)
		}
		select {
		case <-c.done:
			// Detach so further straggler primes are dropped on
			// the floor rather than counted next iteration.
			h.primeCount.Store(nil)
			return
		case <-time.After(testutil.IntervalFast):
			h.primeCount.Store(nil)
		}
	}
	b.Fatalf("interest propagation timed out for subject %s", h.subject)
}

// runFanoutBench publishes b.N messages in a tight loop, then drains
// asynchronous deliveries up to drainTimeout. Reports MB/s (via
// b.SetBytes), deliveryMB/s, pubs/s, deliveries/s, delivery_pct, and
// drop_events. See the file header for metric definitions.
//
// runFanoutBench is invoked once per testing.B calibration pass. It
// owns the per-pass setup (subscribe + prime) so that pass N's
// in-flight deliveries cannot leak into pass N+1's counter. Server
// bring-up and payload allocation are done by the caller and reused
// across passes.
func runFanoutBench(b *testing.B, h *fanoutHarness, leafTag string, publisher *Pubsub, totalSubs int, payload []byte) {
	b.Helper()

	// Per-pass setup: new subject, new subscriptions, prime interest.
	// Done OUTSIDE the timed region so the publish loop measures only
	// publisher throughput.
	b.StopTimer()
	h.setupPass(b, leafTag)
	h.waitInterest(b, publisher, totalSubs, payload)

	b.SetBytes(int64(len(payload)))

	target := int64(b.N) * int64(totalSubs)
	counter := &deliveryCounter{target: target, done: make(chan struct{})}
	h.counter.Store(counter)

	b.ResetTimer()
	b.StartTimer()
	start := time.Now()
	for range b.N {
		if err := publisher.Publish(h.subject, payload); err != nil {
			b.Fatalf("publish: %v", err)
		}
	}
	pubElapsed := time.Since(start)
	b.StopTimer()

	drained := false
	select {
	case <-counter.done:
		drained = true
	case <-time.After(drainTimeout):
	}
	totalElapsed := time.Since(start)

	// Detach the counter so any final stragglers don't race with the
	// next pass's setup (setupPass also clears it, but detaching here
	// closes the window between drain return and teardown).
	h.counter.Store(nil)

	finalDelivered := counter.count.Load()
	dropEvents := h.dropEvents.Load()

	pubsPerSec := float64(b.N) / pubElapsed.Seconds()
	delPerSec := float64(finalDelivered) / totalElapsed.Seconds()
	var deliveryPct float64
	if target > 0 {
		deliveryPct = 100.0 * float64(finalDelivered) / float64(target)
	}
	// b.SetBytes reports publisher ingress MB/s (built-in). The
	// deliveryMB/s metric reports aggregate fan-out bandwidth:
	// payload bytes actually delivered to subscriber callbacks per
	// second of wall time (publish + drain). For totalSubs > 1 this
	// is strictly higher than MB/s.
	deliveryMBPerSec := float64(finalDelivered*int64(len(payload))) / totalElapsed.Seconds() / (1 << 20)

	b.ReportMetric(pubsPerSec, "pubs/s")
	b.ReportMetric(delPerSec, "deliveries/s")
	b.ReportMetric(deliveryPct, "delivery_pct")
	b.ReportMetric(deliveryMBPerSec, "deliveryMB/s")
	// drop_events counts ErrDroppedMessages callbacks, not lost
	// messages. NATS coalesces multiple drops into a single callback
	// per slow-consumer event, so this is a lower bound.
	b.ReportMetric(float64(dropEvents), "drop_events")

	// Honest failure: an incomplete drain means deliveries from this
	// pass are still in flight and would otherwise leak into the next
	// calibration pass's counter. Fail loudly rather than report a
	// bogus throughput data point.
	if !drained || finalDelivered < target {
		b.Fatalf("drain incomplete: delivered=%d target=%d delivery_pct=%.2f drop_events=%d (drainTimeout=%s)",
			finalDelivered, target, deliveryPct, dropEvents, drainTimeout)
	}
}

var benchPayloads = []struct {
	name string
	size int
}{
	{"4KiB", 4 * 1024},
	{"512KiB", 512 * 1024},
}

func BenchmarkPubsubFanout_SingleNode(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping NATS pubsub bench in -short mode")
	}
	for _, payload := range benchPayloads {
		for _, n := range []int{1, 4, 16, 64} {
			b.Run(fmt.Sprintf("payload=%s/subs=%d", payload.name, n), func(b *testing.B) {
				// Build the Pubsub and the (subjectless) harness ONCE
				// per leaf, outside of testing.B's N-calibration
				// loop. Subscribe + prime happen per pass inside
				// runFanoutBench so each pass gets a fresh subject
				// and cannot inherit in-flight deliveries from a
				// prior pass.
				b.StopTimer()
				ps := newBenchSingleNode(b)
				h := newFanoutHarness([]*Pubsub{ps}, n)
				body := makePayload(payload.size)
				leafTag := fmt.Sprintf("bench_single_%s_%d_%d", payload.name, n, time.Now().UnixNano())
				runFanoutBench(b, h, leafTag, ps, n, body)
			})
		}
	}
}

func BenchmarkPubsubFanout_Cluster(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping NATS pubsub bench in -short mode")
	}
	for _, payload := range benchPayloads {
		for _, replicas := range []int{3, 10} {
			for _, subsPerNode := range []int{1, 4, 16} {
				b.Run(fmt.Sprintf("payload=%s/replicas=%d/subs_per_node=%d", payload.name, replicas, subsPerNode), func(b *testing.B) {
					b.StopTimer()
					nodes := newBenchCluster(b, replicas)
					h := newFanoutHarness(nodes, subsPerNode)
					total := replicas * subsPerNode
					body := makePayload(payload.size)
					leafTag := fmt.Sprintf("bench_cluster_%s_r%d_s%d_%d", payload.name, replicas, subsPerNode, time.Now().UnixNano())
					runFanoutBench(b, h, leafTag, nodes[0], total, body)
				})
			}
		}
	}
}
