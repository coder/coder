//nolint:testpackage
package nats

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

// BenchmarkPubsubFanout_SingleNode is intentionally a no-op: realistic
// Coder topology is the 10-replica cluster (see
// BenchmarkPubsubFanout_Wrapper).
func BenchmarkPubsubFanout_SingleNode(b *testing.B) {
	b.Skip("single-node bench skipped; see BenchmarkPubsubFanout_Wrapper")
}

// BenchmarkPubsubFanout_Wrapper measures the Coder *Pubsub wrapper
// in its production topology: a 10-replica cluster, 100 subscribers
// per node, swept across payload sizes and publisher fanout counts.
// Each node produces one subscriber sample (one nc, many subscriptions
// multiplexed through it). b.N is the total publish count; publishers
// split b.N round-robin across cluster nodes.
//
// Run with: -benchtime=<msgs>x.
func BenchmarkPubsubFanout_Wrapper(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping NATS pubsub bench in -short mode")
	}
	requireFixedBenchtime(b)
	const (
		replicas    = 10
		subsPerNode = 100
	)
	for _, payload := range benchPayloads {
		for _, pubs := range []int{1, 4, 10} {
			name := fmt.Sprintf("payload=%s/replicas=%d/subs_per_node=%d/pub_clients=%d",
				payload.name, replicas, subsPerNode, pubs)
			b.Run(name, func(b *testing.B) {
				runPubsubWrapperFanout(b, wrapperFanoutConfig{
					payloadSize: payload.size,
					replicas:    replicas,
					subsPerNode: subsPerNode,
					pubClients:  pubs,
				})
			})
		}
	}
}

type wrapperFanoutConfig struct {
	payloadSize int
	replicas    int
	subsPerNode int
	pubClients  int
}

// newBenchCluster brings up an N-node full-mesh cluster on loopback
// and waits for routes to converge. Configured for benchmark needs
// (MaxPayload, generous pending limits).
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
		name := fmt.Sprintf("bench-wrapper-node-%d", i)
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

// wrapperSubscriberSample collects observable state for a single node
// in the cluster across all its multiplexed subscriptions.
type wrapperSubscriberSample struct {
	name      string
	expected  int64
	delivered atomic.Int64
	firstOnce sync.Once
	firstNS   atomic.Int64
	lastNS    atomic.Int64
	dropped   atomic.Int64
	done      chan struct{}
	doneOnce  sync.Once
}

func (s *wrapperSubscriberSample) observe(err error) {
	if err != nil {
		if errors.Is(err, pubsub.ErrDroppedMessages) {
			s.dropped.Add(1)
		}
		return
	}
	now := time.Now().UnixNano()
	s.firstOnce.Do(func() { s.firstNS.Store(now) })
	s.lastNS.Store(now)
	if s.delivered.Add(1) >= s.expected {
		s.doneOnce.Do(func() { close(s.done) })
	}
}

func (s *wrapperSubscriberSample) toBenchSample(payloadSize int) *benchSample {
	first := s.firstNS.Load()
	last := s.lastNS.Load()
	got := s.delivered.Load()
	var start, end time.Time
	if first > 0 {
		start = time.Unix(0, first)
		end = time.Unix(0, last)
		if !end.After(start) {
			end = start.Add(time.Nanosecond)
		}
	} else {
		start = time.Now()
		end = start
	}
	return newBenchSample(s.name, got, payloadSize, start, end, nil)
}

func runPubsubWrapperFanout(b *testing.B, cfg wrapperFanoutConfig) {
	b.StopTimer()
	if isBenchWarmup(b) {
		// testing.B always runs the function once with b.N=1 before
		// the real run. Skip the expensive cluster setup so cost is
		// paid only once per leaf.
		return
	}
	nodes := newBenchCluster(b, cfg.replicas)
	payload := makePayload(cfg.payloadSize)

	// The wrapper validates subject tokens; use underscores so the
	// legacy-event -> mapped subject translation accepts the input.
	subject := fmt.Sprintf("bench_wrapper_%d", benchSubjectID.Add(1))
	primeSubject := subject + "_prime"

	// One subscriber sample per node; expected = b.N * subsPerNode
	// because every subscription on the node receives every message.
	expectedPerNode := int64(b.N) * int64(cfg.subsPerNode)
	subSamples := make([]*wrapperSubscriberSample, cfg.replicas)
	for i := range nodes {
		subSamples[i] = &wrapperSubscriberSample{
			name:     fmt.Sprintf("bench-wrapper-node-%d", i),
			expected: expectedPerNode,
			done:     make(chan struct{}),
		}
	}

	cancels := make([]func(), 0, cfg.replicas*cfg.subsPerNode*2)
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	// Register runtime subscriptions.
	for i, n := range nodes {
		ss := subSamples[i]
		for k := 0; k < cfg.subsPerNode; k++ {
			cancel, err := n.SubscribeWithErr(subject, func(_ context.Context, _ []byte, err error) {
				ss.observe(err)
			})
			if err != nil {
				b.Fatalf("subscribe node=%d sub=%d: %v", i, k, err)
			}
			cancels = append(cancels, cancel)
		}
	}

	// Prime interest on a separate subject so route propagation does
	// not affect the timed run.
	primeExpected := int64(cfg.replicas * cfg.subsPerNode)
	var primeCount atomic.Int64
	primeDone := make(chan struct{})
	var primeDoneOnce sync.Once
	for _, n := range nodes {
		for k := 0; k < cfg.subsPerNode; k++ {
			cancel, err := n.SubscribeWithErr(primeSubject, func(_ context.Context, _ []byte, err error) {
				if err != nil {
					return
				}
				if primeCount.Add(1) >= primeExpected {
					primeDoneOnce.Do(func() { close(primeDone) })
				}
			})
			if err != nil {
				b.Fatalf("subscribe prime: %v", err)
			}
			cancels = append(cancels, cancel)
		}
	}
	primeDeadline := time.Now().Add(testutil.WaitLong)
	for {
		primeCount.Store(0)
		if err := nodes[0].Publish(primeSubject, []byte{0}); err != nil {
			b.Fatalf("priming publish: %v", err)
		}
		select {
		case <-primeDone:
			goto primed
		case <-time.After(testutil.IntervalFast):
			if time.Now().After(primeDeadline) {
				b.Fatalf("interest propagation timed out for subject %s", subject)
			}
		}
	}
primed:

	pubCounts := splitCounts(b.N, cfg.pubClients)
	var pubReady sync.WaitGroup
	pubReady.Add(cfg.pubClients)
	trigger := make(chan struct{})
	pubResults := make(chan *benchSample, cfg.pubClients)
	errCh := make(chan error, cfg.pubClients)

	for i := 0; i < cfg.pubClients; i++ {
		p := nodes[i%cfg.replicas]
		name := fmt.Sprintf("pub-%d", i)
		count := pubCounts[i]
		go func() {
			pubReady.Done()
			<-trigger
			latencies := make([]uint64, count)
			start := time.Now()
			for k := 0; k < count; k++ {
				opStart := time.Now()
				if err := p.Publish(subject, payload); err != nil {
					errCh <- fmt.Errorf("publish %s: %w", name, err)
					return
				}
				latencies[k] = uint64(time.Since(opStart).Nanoseconds())
			}
			if err := p.nc.Flush(); err != nil {
				errCh <- fmt.Errorf("flush %s: %w", name, err)
				return
			}
			end := time.Now()
			pubResults <- newBenchSample(name, int64(count), cfg.payloadSize, start, end, latencies)
		}()
	}
	pubReady.Wait()

	b.ResetTimer()
	b.StartTimer()
	close(trigger)

	pubGroup := newBenchSampleGroup()
	for i := 0; i < cfg.pubClients; i++ {
		select {
		case s := <-pubResults:
			pubGroup.AddSample(s)
		case err := <-errCh:
			b.Fatalf("publisher error: %v", err)
		case <-time.After(benchDeliveryDeadline):
			b.Fatalf("publisher %d did not complete within %s", i, benchDeliveryDeadline)
		}
	}

	// Wait for each node's subscriber sample to complete.
	deadline := time.After(benchDeliveryDeadline)
	for i, ss := range subSamples {
		select {
		case <-ss.done:
		case <-deadline:
			b.Fatalf("subscriber node=%d incomplete: delivered=%d expected=%d subject=%s deadline=%s",
				i, ss.delivered.Load(), ss.expected, subject, benchDeliveryDeadline)
		}
	}
	b.StopTimer()

	// Aggregate subscriber samples + drop events.
	subGroup := newBenchSampleGroup()
	var delivered, dropEvents int64
	for _, ss := range subSamples {
		subGroup.AddSample(ss.toBenchSample(cfg.payloadSize))
		delivered += ss.delivered.Load()
		dropEvents += ss.dropped.Load()
	}

	expected := int64(b.N) * int64(cfg.replicas) * int64(cfg.subsPerNode)
	diag := fmt.Sprintf("Wrapper diagnostics: drop_events=%d replicas=%d subs_per_node=%d pub_clients=%d\n",
		dropEvents, cfg.replicas, cfg.subsPerNode, cfg.pubClients)
	logBenchReport(b, benchReportInput{
		name:        "wrapper",
		pubs:        pubGroup,
		subs:        subGroup,
		expected:    expected,
		delivered:   delivered,
		dropEvents:  dropEvents,
		diagnostics: diag,
	})
	reportBenchMetrics(b, pubGroup, subGroup, expected, delivered)
	b.ReportMetric(float64(dropEvents), "drop_events")

	if dropEvents > 0 {
		b.Fatalf("dropped message events observed: %d", dropEvents)
	}
}
