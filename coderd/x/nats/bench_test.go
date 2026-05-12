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
//   - Our payload is 512 KiB (524288 B) per message, which is a
//     fundamentally different regime from upstream's tiny-payload
//     micro-bench (typically <=256 B). At 512 KiB, throughput is
//     dominated by memory bandwidth, socket buffering, and route
//     forwarding rather than per-message overhead.
//   - We exercise fan-out (one publisher, N subscribers) and a
//     clustered topology over loopback routes, which upstream does not
//     emphasize in its single-subject micro-benches.
// The upstream numbers are useful only as a sanity floor: if a tiny
// publish through our wrapper took milliseconds, something would be
// badly wrong.

const benchPayloadLen = 512 * 1024 // 512 KiB, the bench payload size.

// benchPayload returns a deterministic, non-zero 512 KiB byte slice.
func benchPayload() []byte {
	return bytes.Repeat([]byte("x"), benchPayloadLen)
}

// newBenchSingleNode returns a single-node (cluster-of-1) Pubsub with
// the wrapper's defaults plus an explicit MaxPayload of 1 MiB so the
// 512 KiB payload always fits regardless of upstream default drift.
//
// PendingLimits is left at the NATS default (64 MiB / 65536 msgs per
// subscription); empirically that is sufficient at 512 KiB for the
// fan-out counts swept here because the bench loop waits for delivery
// from every subscriber before publishing the next message. If a run
// ever reports ErrDroppedMessages, raise PendingLimits.Bytes for the
// affected subscription(s) to e.g. 512 MiB.
func newBenchSingleNode(b testing.TB) *Pubsub {
	b.Helper()
	logger := slogtest.Make(b, &slogtest.Options{IgnoreErrors: true}).
		Leveled(slog.LevelError)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	p, err := New(ctx, logger, Options{
		MaxPayload:   1 << 20,
		ReadyTimeout: testutil.WaitMedium,
	})
	if err != nil {
		b.Fatalf("new single-node pubsub: %v", err)
	}
	b.Cleanup(func() { _ = p.Close() })
	return p
}

// newBenchCluster brings up an N-node full-mesh cluster on loopback
// and waits for routes to converge. Reuses the buildClusterPubsub /
// freePort / waitForRoutes test helpers.
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
		nodes[i] = buildClusterPubsub(b, fmt.Sprintf("bench-node-%d", i),
			ports[i], peers, token, nil)
	}
	for _, n := range nodes {
		waitForRoutes(b, n, replicas-1)
	}
	return nodes
}

// fanoutHarness subscribes `subsPerNode` listeners on each Pubsub in
// `nodes` against `subject`. Every delivery sends one token into
// `delivered`. Any ErrDroppedMessages observation increments `drops`.
type fanoutHarness struct {
	subject   string
	delivered chan struct{}
	drops     atomic.Int64
	cancels   []func()
}

func newFanoutHarness(b testing.TB, nodes []*Pubsub, subsPerNode int, subject string) *fanoutHarness {
	b.Helper()
	total := len(nodes) * subsPerNode
	// Buffer big enough to absorb a single round of fan-out without
	// blocking the subscriber goroutine. The publisher drains this
	// channel before publishing the next message.
	h := &fanoutHarness{
		subject:   subject,
		delivered: make(chan struct{}, total),
		cancels:   make([]func(), 0, total),
	}
	for _, n := range nodes {
		for range subsPerNode {
			cancel, err := n.SubscribeWithErr(subject, func(_ context.Context, _ []byte, err error) {
				if err != nil {
					if errors.Is(err, pubsub.ErrDroppedMessages) {
						h.drops.Add(1)
					}
					return
				}
				h.delivered <- struct{}{}
			})
			if err != nil {
				b.Fatalf("subscribe: %v", err)
			}
			h.cancels = append(h.cancels, cancel)
		}
	}
	b.Cleanup(func() {
		for _, c := range h.cancels {
			c()
		}
	})
	return h
}

// waitInterest publishes one priming message and waits for every
// subscriber across all nodes to deliver it. This ensures cluster
// route interest propagation is complete before the timed loop. The
// priming message is not counted towards b.N.
func (h *fanoutHarness) waitInterest(b testing.TB, publisher *Pubsub, total int, payload []byte) {
	b.Helper()
	deadline := time.Now().Add(testutil.WaitLong)
	for time.Now().Before(deadline) {
		if err := publisher.Publish(h.subject, payload); err != nil {
			b.Fatalf("priming publish: %v", err)
		}
		got := 0
		ok := true
		for got < total && ok {
			select {
			case <-h.delivered:
				got++
			case <-time.After(testutil.IntervalFast):
				ok = false
			}
		}
		if got == total {
			// Drain any stragglers from earlier priming publishes.
			drainUntil := time.Now().Add(testutil.IntervalFast)
			for time.Now().Before(drainUntil) {
				select {
				case <-h.delivered:
				default:
					return
				}
			}
			return
		}
		// Drain partial deliveries before retrying.
		for {
			select {
			case <-h.delivered:
			default:
				goto retry
			}
		}
	retry:
	}
	b.Fatalf("interest propagation timed out for subject %s", h.subject)
}

// runFanoutBench is the timed loop. The publisher publishes a single
// message, then waits for `total` deliveries (one per subscriber)
// before publishing the next. This measures end-to-end fan-out
// throughput with natural backpressure.
func runFanoutBench(b *testing.B, h *fanoutHarness, publisher *Pubsub, total int, payload []byte) {
	b.Helper()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	start := time.Now()
	for range b.N {
		if err := publisher.Publish(h.subject, payload); err != nil {
			b.Fatalf("publish: %v", err)
		}
		got := 0
		for got < total {
			<-h.delivered
			got++
		}
	}
	b.StopTimer()
	elapsed := time.Since(start)

	if d := h.drops.Load(); d > 0 {
		b.Fatalf("subscriber observed %d dropped-message events; "+
			"raise PendingLimits.Bytes for this configuration", d)
	}
	totalDeliveries := int64(b.N) * int64(total)
	secs := elapsed.Seconds()
	if secs > 0 {
		b.ReportMetric(float64(totalDeliveries)/secs, "deliveries/s")
		b.ReportMetric(float64(b.N)/secs, "pubs/s")
	}
}

func BenchmarkPubsubFanout_SingleNode(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping NATS pubsub bench in -short mode")
	}
	for _, n := range []int{1, 4, 16, 64} {
		b.Run(fmt.Sprintf("subs=%d", n), func(b *testing.B) {
			// Build the Pubsub and subscriber harness ONCE per leaf
			// sub-benchmark, outside of testing.B's N-calibration
			// loop. testing.B calls the inner func repeatedly with
			// growing N values; doing setup inside that func would
			// spin up a new NATS server (and leak the prior one,
			// since b.Cleanup only fires at end of test) on every
			// calibration pass.
			b.StopTimer()
			ps := newBenchSingleNode(b)
			subject := fmt.Sprintf("bench_single_%d_%d", n, time.Now().UnixNano())
			h := newFanoutHarness(b, []*Pubsub{ps}, n, subject)
			payload := benchPayload()
			h.waitInterest(b, ps, n, payload)

			b.Run("run", func(b *testing.B) {
				runFanoutBench(b, h, ps, n, payload)
			})
		})
	}
}

func BenchmarkPubsubFanout_Cluster(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping NATS pubsub bench in -short mode")
	}
	for _, replicas := range []int{3, 10} {
		for _, subsPerNode := range []int{1, 4, 16} {
			b.Run(fmt.Sprintf("replicas=%d/subs_per_node=%d", replicas, subsPerNode), func(b *testing.B) {
				// Setup happens in the outer b.Run so it runs once
				// per configuration; the inner b.Run is what
				// testing.B's N-calibration drives.
				b.StopTimer()
				nodes := newBenchCluster(b, replicas)
				subject := fmt.Sprintf("bench_cluster_r%d_s%d_%d", replicas, subsPerNode, time.Now().UnixNano())
				h := newFanoutHarness(b, nodes, subsPerNode, subject)
				total := replicas * subsPerNode
				payload := benchPayload()
				h.waitInterest(b, nodes[0], total, payload)

				b.Run("run", func(b *testing.B) {
					runFanoutBench(b, h, nodes[0], total, payload)
				})
			})
		}
	}
}
