//nolint:testpackage
package nats

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"

	"github.com/coder/coder/v2/testutil"
)

// BenchmarkNATSCoreFanout_TCP measures fanout throughput against a
// raw embedded NATS server over TCP loopback using only nats.go
// primitives. One *nats.Conn per publisher and per subscriber; each
// publisher produces one publisher sample, each subscriber connection
// produces one subscriber sample. b.N is the total publish count;
// publishers split b.N, each subscriber receives all b.N messages.
//
// Run with: -benchtime=<msgs>x. See bench_common_test.go.
func BenchmarkNATSCoreFanout_TCP(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping NATS core TCP bench in -short mode")
	}
	requireFixedBenchtime(b)
	for _, payload := range benchPayloads {
		for _, subs := range []int{100, 1000} {
			for _, pubs := range []int{1, 4, 10} {
				name := fmt.Sprintf("payload=%s/sub_clients=%d/pub_clients=%d", payload.name, subs, pubs)
				b.Run(name, func(b *testing.B) {
					runNATSCoreFanoutTCP(b, coreFanoutConfig{
						payloadSize: payload.size,
						subClients:  subs,
						pubClients:  pubs,
					})
				})
			}
		}
	}
}

type coreFanoutConfig struct {
	payloadSize int
	subClients  int
	pubClients  int
}

// benchClientState tracks observable per-client error / disconnect
// events without altering production wrapper hooks.
type benchClientState struct {
	name         string
	errored      atomic.Bool
	disconnected atomic.Bool
}

func natsBenchOptions(state *benchClientState) []natsgo.Option {
	return []natsgo.Option{
		natsgo.Name(state.name),
		natsgo.MaxReconnects(-1),
		natsgo.IgnoreAuthErrorAbort(),
		natsgo.DisconnectErrHandler(func(_ *natsgo.Conn, err error) {
			if err != nil {
				state.disconnected.Store(true)
			}
		}),
		natsgo.ErrorHandler(func(_ *natsgo.Conn, _ *natsgo.Subscription, err error) {
			if err != nil {
				state.errored.Store(true)
			}
		}),
	}
}

// subscriberRunResult is delivered from a subscriber goroutine once
// it has either observed all expected messages or its deadline fires.
type subscriberRunResult struct {
	sample    *benchSample
	delivered int64
	state     *benchClientState
	subject   string
}

func runNATSCoreFanoutTCP(b *testing.B, cfg coreFanoutConfig) {
	b.StopTimer()
	if isBenchWarmup(b) {
		// testing.B always runs the function once with b.N=1 before
		// the real run. Skip the expensive setup so cost is paid
		// only once per leaf and the --- BENCH: log block reflects
		// the real run.
		return
	}

	// Embedded server on a real TCP loopback port.
	sopts := &natsserver.Options{
		ServerName: fmt.Sprintf("bench-core-tcp-%d", time.Now().UnixNano()),
		Host:       "127.0.0.1",
		Port:       natsserver.RANDOM_PORT,
		MaxPayload: benchMaxPayload,
		NoLog:      true,
		NoSigs:     true,
	}
	ns, err := natsserver.NewServer(sopts)
	if err != nil {
		b.Fatalf("new embedded nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(testutil.WaitMedium) {
		ns.Shutdown()
		ns.WaitForShutdown()
		b.Fatalf("embedded nats server not ready within %s", testutil.WaitMedium)
	}
	b.Cleanup(func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	})
	url := ns.ClientURL()

	payload := makePayload(cfg.payloadSize)
	subject := uniqueBenchSubject("bench.core.tcp")

	// Subscriber connections + states.
	subConns := make([]*natsgo.Conn, cfg.subClients)
	subStates := make([]*benchClientState, cfg.subClients)
	for i := 0; i < cfg.subClients; i++ {
		st := &benchClientState{name: fmt.Sprintf("sub-%d", i)}
		subStates[i] = st
		nc, err := natsgo.Connect(url, natsBenchOptions(st)...)
		if err != nil {
			b.Fatalf("subscriber %d connect: %v", i, err)
		}
		subConns[i] = nc
	}
	b.Cleanup(func() {
		for _, nc := range subConns {
			if nc != nil {
				nc.Close()
			}
		}
	})

	// Publisher connections + states.
	pubConns := make([]*natsgo.Conn, cfg.pubClients)
	pubStates := make([]*benchClientState, cfg.pubClients)
	for i := 0; i < cfg.pubClients; i++ {
		st := &benchClientState{name: fmt.Sprintf("pub-%d", i)}
		pubStates[i] = st
		nc, err := natsgo.Connect(url, natsBenchOptions(st)...)
		if err != nil {
			b.Fatalf("publisher %d connect: %v", i, err)
		}
		pubConns[i] = nc
	}
	b.Cleanup(func() {
		for _, nc := range pubConns {
			if nc != nil {
				nc.Close()
			}
		}
	})

	expectedPerSub := b.N
	pubCounts := splitCounts(b.N, cfg.pubClients)

	var (
		subReady sync.WaitGroup
		pubReady sync.WaitGroup
	)
	subReady.Add(cfg.subClients)
	pubReady.Add(cfg.pubClients)
	trigger := make(chan struct{})
	subResults := make(chan subscriberRunResult, cfg.subClients)
	pubResults := make(chan *benchSample, cfg.pubClients)
	errCh := make(chan error, cfg.pubClients+cfg.subClients)

	// Subscribers first so interest is registered before publishers
	// fire.
	for i := 0; i < cfg.subClients; i++ {
		nc := subConns[i]
		st := subStates[i]
		go runCoreSubscriberSample(b, nc, st, subject, cfg.payloadSize, expectedPerSub, &subReady, subResults, errCh)
	}
	subReady.Wait()

	for i := 0; i < cfg.pubClients; i++ {
		nc := pubConns[i]
		st := pubStates[i]
		count := pubCounts[i]
		go runCorePublisherSample(nc, st, subject, payload, count, &pubReady, trigger, pubResults, errCh)
	}
	pubReady.Wait()

	b.ResetTimer()
	b.StartTimer()
	close(trigger)

	// Collect publisher samples.
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

	// Collect subscriber samples.
	subGroup := newBenchSampleGroup()
	var delivered int64
	deadline := time.After(benchDeliveryDeadline)
	for i := 0; i < cfg.subClients; i++ {
		select {
		case r := <-subResults:
			subGroup.AddSample(r.sample)
			delivered += r.delivered
			if r.delivered < int64(expectedPerSub) {
				b.Fatalf("subscriber %s incomplete: delivered=%d expected=%d subject=%s deadline=%s",
					r.sample.name, r.delivered, expectedPerSub, r.subject, benchDeliveryDeadline)
			}
		case err := <-errCh:
			b.Fatalf("subscriber error: %v", err)
		case <-deadline:
			b.Fatalf("subscriber completion deadline %s exceeded (got %d of %d samples)",
				benchDeliveryDeadline, i, cfg.subClients)
		}
	}
	b.StopTimer()

	// Tally per-client failure flags.
	var errored, disconnected int64
	for _, st := range pubStates {
		if st.errored.Load() {
			errored++
		}
		if st.disconnected.Load() {
			disconnected++
		}
	}
	for _, st := range subStates {
		if st.errored.Load() {
			errored++
		}
		if st.disconnected.Load() {
			disconnected++
		}
	}

	expected := int64(b.N) * int64(cfg.subClients)
	logBenchReport(b, benchReportInput{
		name:         "core_tcp",
		pubs:         pubGroup,
		subs:         subGroup,
		expected:     expected,
		delivered:    delivered,
		errored:      errored,
		disconnected: disconnected,
	})
	reportBenchMetrics(b, pubGroup, subGroup, expected, delivered)

	if errored > 0 || disconnected > 0 {
		b.Fatalf("client failures: errored=%d disconnected=%d", errored, disconnected)
	}
}

// runCoreSubscriberSample subscribes on the connection, primes a flush,
// signals ready, and records first-recv / last-recv timestamps. Once
// the expected count is reached, it emits a subscriberRunResult.
func runCoreSubscriberSample(
	b *testing.B,
	nc *natsgo.Conn,
	state *benchClientState,
	subject string,
	payloadSize int,
	expected int,
	ready *sync.WaitGroup,
	out chan<- subscriberRunResult,
	errCh chan<- error,
) {
	b.Helper()
	var (
		delivered atomic.Int64
		firstOnce sync.Once
		firstNS   atomic.Int64
		lastNS    atomic.Int64
		doneOnce  sync.Once
	)
	done := make(chan struct{})
	sub, err := nc.Subscribe(subject, func(_ *natsgo.Msg) {
		now := time.Now().UnixNano()
		firstOnce.Do(func() { firstNS.Store(now) })
		lastNS.Store(now)
		if delivered.Add(1) >= int64(expected) {
			doneOnce.Do(func() { close(done) })
		}
	})
	if err != nil {
		errCh <- fmt.Errorf("subscribe %s: %w", state.name, err)
		ready.Done()
		return
	}
	if err := sub.SetPendingLimits(-1, -1); err != nil {
		errCh <- fmt.Errorf("set pending limits %s: %w", state.name, err)
		ready.Done()
		return
	}
	if err := nc.Flush(); err != nil {
		errCh <- fmt.Errorf("subscriber flush %s: %w", state.name, err)
		ready.Done()
		return
	}
	ready.Done()

	select {
	case <-done:
	case <-time.After(benchDeliveryDeadline):
	}
	first := firstNS.Load()
	last := lastNS.Load()
	got := delivered.Load()
	var start, end time.Time
	if first > 0 {
		start = time.Unix(0, first)
		end = time.Unix(0, last)
		if !end.After(start) {
			end = start.Add(time.Nanosecond)
		}
	} else {
		// No messages observed. Use a zero-window sample so the group
		// aggregation still sees it as a participating client.
		start = time.Now()
		end = start
	}
	sample := newBenchSample(state.name, got, payloadSize, start, end, nil)
	out <- subscriberRunResult{
		sample:    sample,
		delivered: got,
		state:     state,
		subject:   subject,
	}
}

// runCorePublisherSample waits at the start barrier, then publishes
// count messages reusing one *nats.Msg, recording per-publish latency.
// One end-of-loop Flush is inside the timed region, matching upstream.
func runCorePublisherSample(
	nc *natsgo.Conn,
	state *benchClientState,
	subject string,
	payload []byte,
	count int,
	ready *sync.WaitGroup,
	trigger <-chan struct{},
	out chan<- *benchSample,
	errCh chan<- error,
) {
	ready.Done()
	<-trigger

	msg := &natsgo.Msg{Subject: subject, Data: payload}
	latencies := make([]uint64, count)
	start := time.Now()
	for i := 0; i < count; i++ {
		opStart := time.Now()
		if err := nc.PublishMsg(msg); err != nil {
			errCh <- fmt.Errorf("publish %s: %w", state.name, err)
			return
		}
		latencies[i] = uint64(time.Since(opStart).Nanoseconds())
	}
	if err := nc.Flush(); err != nil {
		errCh <- fmt.Errorf("publisher flush %s: %w", state.name, err)
		return
	}
	end := time.Now()
	out <- newBenchSample(state.name, int64(count), len(payload), start, end, latencies)
}
