package nats_test

// Capacity-planning benchmarks for NATS Core pub/sub.
//
// This bench answers: "how many publishes per second can NATS absorb in
// Coder-shaped workloads, with 100% delivery?".
//
// Each benchmark can run against one of two backends, selected by the
// package-level -bench.type flag:
//
//   - native: raw nats-server + nats.go connections. Measures NATS
//     capacity, not wrapper overhead.
//   - coder:  the coderd/x/nats.Pubsub wrapper. Measures end-to-end
//     capacity through the wrapper that production code actually uses
//     (subject mapping, metrics, slow-consumer accounting, etc.).
//
// Matrix (8 leaves per backend): topology={standalone,cluster10} x
// subjects={1,10} x payload={8KiB,512KiB}.
//
// Operator contract: REQUIRES -benchtime=Nx (e.g. -benchtime=1000x).
// Time-based -benchtime (default 1s) is rejected with a clear error so
// nobody silently runs a 1-message bench.
//
// Run examples:
//   go test -run x -bench BenchmarkPubsub -benchtime=1000x \
//     -bench.type=native ./coderd/x/nats/ -timeout 30m
//   go test -run x -bench BenchmarkPubsub/coder/standalone \
//     -benchtime=500x -bench.type=coder ./coderd/x/nats/ -timeout 10m

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	xnats "github.com/coder/coder/v2/coderd/x/nats"
	"github.com/coder/coder/v2/testutil"
)

// benchType selects the backend for every BenchmarkPubsub* benchmark.
// "native" uses raw *nats.Conn; "coder" uses the coderd/x/nats.Pubsub
// wrapper. Validated by requireIterBenchtime.
var benchType = flag.String("bench.type", "native",
	"benchmark backend: native (raw nats) or coder (coderd/x/nats.Pubsub wrapper)")

// ---------- IEC byte formatter (no external dep) ----------

func iecBytes(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := int64(n) / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%d%ciB", int64(n)/div, "KMGTPE"[exp])
}

// ---------- benchtime fast-fail ----------

// requireIterBenchtime fails the bench fast if -benchtime is not in Nx
// form. We require an explicit message count so capacity numbers are
// reproducible.
func requireIterBenchtime(b *testing.B) {
	b.Helper()
	f := flag.Lookup("test.benchtime")
	if f == nil {
		b.Fatal("benchmark requires -benchtime=Nx (test.benchtime flag missing)")
	}
	v := f.Value.String()
	if !strings.HasSuffix(v, "x") {
		b.Fatalf("benchmark requires -benchtime=Nx (got %q); time-based benchtime is not supported", v)
	}
	if _, err := strconv.ParseInt(strings.TrimSuffix(v, "x"), 10, 64); err != nil {
		b.Fatalf("benchmark requires -benchtime=Nx (got %q): %v", v, err)
	}
	switch *benchType {
	case "native", "coder":
	default:
		b.Fatalf("invalid -bench.type=%q; allowed: native, coder", *benchType)
	}
}

// benchDiscardLogger returns a slog.Logger that drops everything. Used
// by Coder-mode benchmarks where we don't want server/client log spew
// to pollute the benchmark output.
func benchDiscardLogger() slog.Logger {
	return slog.Make(sloghuman.Sink(io.Discard))
}

// ---------- runtime overhead instrumentation ----------

// runtimeProbe captures process-wide goroutine count and HeapAlloc so a
// benchmark leaf can report deltas attributable to "cost of N
// subscriptions setup". Use captureBaseline before any subscriptions
// are created and reportSubsCost just before the publisher window
// starts. Calls runtime.GC to make HeapAlloc meaningful.
type runtimeProbe struct {
	baseGoroutines int
	baseHeapAlloc  uint64
	deltaGor       int
	deltaHeapMB    float64
}

func (p *runtimeProbe) captureBaseline() {
	runtime.GC() //nolint:revive // benchmark instrumentation needs deterministic heap snapshots
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	p.baseGoroutines = runtime.NumGoroutine()
	p.baseHeapAlloc = ms.HeapAlloc
}

// captureAfterSetup records the goroutine and heap delta attributable
// to the subscription setup phase. Call after all Subscribe calls and
// any flush/settle delay, just before the publisher window starts.
func (p *runtimeProbe) captureAfterSetup() {
	runtime.GC() //nolint:revive // benchmark instrumentation needs deterministic heap snapshots
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	p.deltaGor = runtime.NumGoroutine() - p.baseGoroutines
	// HeapAlloc is uint64 but in benchmarks always fits in int64; the
	// subtraction may be negative if GC reclaimed more than we
	// allocated (rare; e.g., NewFromConn child paths).
	delta := int64(ms.HeapAlloc) - int64(p.baseHeapAlloc) //nolint:gosec // bounded by process heap
	p.deltaHeapMB = float64(delta) / (1024 * 1024)
}

func (p *runtimeProbe) report(b *testing.B) {
	b.Helper()
	b.ReportMetric(float64(p.deltaGor), "goroutines_delta")
	b.ReportMetric(p.deltaHeapMB, "heap_alloc_delta_mb")
}

// ---------- backend abstraction ----------

// harness hides the difference between raw-NATS and Pubsub-wrapper
// benchmarks. A leaf runner stands up a harness with the desired number
// of replicas and total subscriber/publisher counts, then drives it via
// publish/subscribe closures. The harness is responsible for its own
// cleanup via b.Cleanup.
//
// Conceptually:
//
//   - numReplicas is the number of "logical" NATS servers (always 1 in
//     standalone, N in cluster).
//   - publishers is a function table indexed by publisher index. Each
//     publisher is bound to exactly one replica (publisher i -> replica
//     i % numReplicas). For native mode this is a *nats.Conn-bound
//     PublishMsg; for coder mode it is the *Pubsub-bound Publish.
//   - subscribe registers a single subscription on replica replicaIdx
//     for the given subject and invokes onMsg for each delivery.
//   - flushPubs blocks until every publisher's outbound buffer has been
//     drained. For coder mode (which doesn't expose Flush on *Pubsub)
//     a small fixed sleep is used; the delivery-completeness check is
//     what proves correctness.
//   - errored / disconnected are counters incremented by underlying
//     client-side error/disconnect callbacks. Reported as a Logf at
//     leaf end.
type harness struct {
	numReplicas int

	// subjectName returns the per-leaf subject string for subject
	// index i. Native and coder modes use different naming because
	// the wrapper validates event tokens (no dots allowed inside one
	// token).
	subjectName func(i int) string

	// publish publishes payload from publisher pubIdx to subject
	// subjects[subjIdx]. Returns nil on success.
	publish func(pubIdx, subjIdx int, payload []byte) error

	// subscribe registers a subscription on replica replicaIdx for
	// subject subjects[subjIdx], invoking onMsg for every delivery.
	subscribe func(replicaIdx, subjIdx int, onMsg func()) error

	// flushPubs ensures all in-flight publishes are on the wire.
	flushPubs func() error

	errored, disconnected *atomic.Int64
}

// setupNative builds a raw-NATS harness: one *nats.Conn per publisher,
// one *nats.Conn per subscriber, against either a standalone server or
// a 10-node embedded cluster.
func setupNative(b *testing.B, topology string, numPubs, numSubs int) *harness {
	b.Helper()
	var servers []*natsserver.Server
	switch topology {
	case "standalone":
		servers = []*natsserver.Server{startStandaloneServer(b)}
	case "cluster10":
		servers = startClusterServers(b, 10)
	default:
		b.Fatalf("unknown topology %q", topology)
	}

	var errored, disconnected atomic.Int64

	// Pre-allocate subscriber connections. Subscriber index s binds
	// to servers[s % len(servers)]; the actual Subscribe call happens
	// inside h.subscribe. Connections are established in parallel
	// because numSubs can be as large as 10000 (high-cardinality) or
	// 5000 (hot-subject); serial connects would burn many seconds of
	// wall time before the publisher window opens.
	subConns := make([]*natsgo.Conn, numSubs)
	connectErrs := make([]error, numSubs)
	var cwg sync.WaitGroup
	cwg.Add(numSubs)
	// Cap parallelism; the embedded server's accept(2) serializes
	// anyway and we don't want to exhaust file descriptors.
	sem := make(chan struct{}, 256)
	for s := 0; s < numSubs; s++ {
		s := s
		serverIdx := s % len(servers)
		sem <- struct{}{}
		go func() {
			defer cwg.Done()
			defer func() { <-sem }()
			nc, err := natsgo.Connect(servers[serverIdx].ClientURL(),
				natsgo.MaxReconnects(-1),
				natsgo.IgnoreAuthErrorAbort(),
				natsgo.ErrorHandler(func(_ *natsgo.Conn, _ *natsgo.Subscription, _ error) {
					errored.Add(1)
				}),
				natsgo.DisconnectErrHandler(func(_ *natsgo.Conn, _ error) {
					disconnected.Add(1)
				}),
			)
			if err != nil {
				connectErrs[s] = err
				return
			}
			subConns[s] = nc
		}()
	}
	cwg.Wait()
	for i, err := range connectErrs {
		if err != nil {
			b.Fatalf("subscriber %d connect: %v", i, err)
		}
	}
	for _, nc := range subConns {
		nc := nc
		b.Cleanup(func() { nc.Close() })
	}

	pubConns := make([]*natsgo.Conn, numPubs)
	for i := 0; i < numPubs; i++ {
		serverIdx := i % len(servers)
		pubConns[i] = benchConnect(b, servers[serverIdx].ClientURL(), &errored, &disconnected)
	}

	// In native mode "replicaIdx" == server index. The subscribe
	// closure maps replicaIdx -> any subscriber connection attached
	// to that replica. We round-robin assign subscriber indexes from
	// each replica's pool. nextSubOnReplica tracks how many we've
	// handed out per replica.
	nextSubOnReplica := make([]int, len(servers))

	return &harness{
		numReplicas:  len(servers),
		subjectName:  func(i int) string { return fmt.Sprintf("bench.subj.%d", i) },
		errored:      &errored,
		disconnected: &disconnected,
		publish: func(pubIdx, subjIdx int, payload []byte) error {
			subj := fmt.Sprintf("bench.subj.%d", subjIdx)
			return pubConns[pubIdx].PublishMsg(&natsgo.Msg{Subject: subj, Data: payload})
		},
		subscribe: func(replicaIdx, subjIdx int, onMsg func()) error {
			// Pick the next subscriber connection attached to this
			// replica. Subscriber index s with s%len(servers)==replicaIdx
			// is at position nextSubOnReplica[replicaIdx] within that
			// replica's pool.
			n := nextSubOnReplica[replicaIdx]
			s := n*len(servers) + replicaIdx
			if s >= numSubs {
				return xerrors.Errorf("native harness: no more subscribers on replica %d (assigned %d)", replicaIdx, n)
			}
			nextSubOnReplica[replicaIdx] = n + 1
			subj := fmt.Sprintf("bench.subj.%d", subjIdx)
			sub, err := subConns[s].Subscribe(subj, func(_ *natsgo.Msg) {
				onMsg()
			})
			if err != nil {
				return err
			}
			sub.SetPendingLimits(-1, -1)
			return nil
		},
		flushPubs: func() error {
			for _, nc := range pubConns {
				if err := nc.Flush(); err != nil {
					return err
				}
			}
			// Also flush subscriber connections so subscription
			// registrations are visible to the server before
			// publishing starts. This is invoked once before the
			// publisher window opens; calling it again after the
			// window is harmless.
			for _, nc := range subConns {
				if err := nc.Flush(); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// setupCoder builds a coderd/x/nats.Pubsub-backed harness: one *Pubsub
// per replica, with the wrapper's own embedded server inside each
// instance. In standalone mode there is one *Pubsub (cluster-of-1); in
// cluster10 mode there are ten *Pubsub instances in a full mesh.
func setupCoder(b *testing.B, topology string, numPubs, numSubs int) *harness {
	b.Helper()

	var numReplicas int
	switch topology {
	case "standalone":
		numReplicas = 1
	case "cluster10":
		numReplicas = 10
	default:
		b.Fatalf("unknown topology %q", topology)
	}

	logger := benchDiscardLogger()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	pubsubs := make([]*xnats.Pubsub, numReplicas)

	if numReplicas == 1 {
		// Cluster-of-1; no peers needed.
		p, err := xnats.New(ctx, logger, xnats.Options{})
		if err != nil {
			b.Fatalf("coder pubsub New (standalone): %v", err)
		}
		pubsubs[0] = p
	} else {
		// Pre-allocate route ports and a shared cluster token, then
		// build a full mesh of peers per replica.
		ports := make([]int, numReplicas)
		for i := range ports {
			ports[i] = freeBenchPort(b)
		}
		// Shared route auth secret used by all replicas. Not a
		// credential; the bench cluster is loopback-only and torn
		// down at the end of each leaf.
		token := "bench-coder-cluster-token" //nolint:gosec // G101: see comment
		for i := 0; i < numReplicas; i++ {
			peers := make([]xnats.Peer, 0, numReplicas-1)
			for j := 0; j < numReplicas; j++ {
				if j == i {
					continue
				}
				peers = append(peers, xnats.Peer{
					Name:     fmt.Sprintf("bench-coder-%d", j),
					RouteURL: fmt.Sprintf("nats://127.0.0.1:%d", ports[j]),
				})
			}
			opts := xnats.Options{
				ServerName:       fmt.Sprintf("bench-coder-%d", i),
				ClusterName:      "bench-coder-cluster",
				ClusterToken:     token,
				ClusterHost:      "127.0.0.1",
				ClusterPort:      ports[i],
				ClusterAdvertise: net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[i])),
				PeerProvider:     xnats.StaticPeerProvider(peers),
				ReadyTimeout:     30 * time.Second,
			}
			p, err := xnats.New(ctx, logger, opts)
			if err != nil {
				// Tear down anything we've already started so b.Cleanup
				// doesn't trip on a partially-constructed cluster.
				for k := 0; k < i; k++ {
					_ = pubsubs[k].Close()
				}
				b.Fatalf("coder pubsub New (cluster replica %d): %v", i, err)
			}
			pubsubs[i] = p
		}
	}

	for _, p := range pubsubs {
		p := p
		b.Cleanup(func() { _ = p.Close() })
	}

	// Subscription cancels accumulate here so b.Cleanup can drain
	// them in reverse before the wrapper's Close.
	var cancelMu sync.Mutex
	var subCancels []func()
	b.Cleanup(func() {
		cancelMu.Lock()
		defer cancelMu.Unlock()
		for i := len(subCancels) - 1; i >= 0; i-- {
			subCancels[i]()
		}
	})

	var errored, disconnected atomic.Int64

	// Subject naming for the wrapper: event tokens must be
	// [A-Za-z0-9_-]+. We use underscores instead of dots and pass
	// the event name through pubsub.Publish/Subscribe; the wrapper
	// maps it to "coder.v1.pubsub.bench_subj_<n>".
	subjectName := func(i int) string { return fmt.Sprintf("bench_subj_%d", i) }

	return &harness{
		numReplicas:  numReplicas,
		subjectName:  subjectName,
		errored:      &errored,
		disconnected: &disconnected,
		publish: func(pubIdx, subjIdx int, payload []byte) error {
			// Publisher pubIdx is bound to replica pubIdx %
			// numReplicas. Matches the "one publisher per replica"
			// pattern used by the native harness.
			return pubsubs[pubIdx%numReplicas].Publish(subjectName(subjIdx), payload)
		},
		subscribe: func(replicaIdx, subjIdx int, onMsg func()) error {
			cancelFn, err := pubsubs[replicaIdx].Subscribe(
				subjectName(subjIdx),
				func(_ context.Context, _ []byte) { onMsg() },
			)
			if err != nil {
				return err
			}
			cancelMu.Lock()
			subCancels = append(subCancels, cancelFn)
			cancelMu.Unlock()
			return nil
		},
		flushPubs: func() error {
			// *Pubsub does not expose Flush. The wrapper's
			// Publish hits nc.Publish synchronously, so by the
			// time the worker pool's wg.Wait returns the calls
			// are at least enqueued on the client; a short sleep
			// gives the client a chance to drain to the server.
			// The delivery-completeness loop is what actually
			// proves messages landed.
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	}
}

// newHarness dispatches on *benchType. It is the only entry point that
// leaf runners need.
func newHarness(b *testing.B, topology string, numPubs, numSubs int) *harness {
	b.Helper()
	switch *benchType {
	case "native":
		return setupNative(b, topology, numPubs, numSubs)
	case "coder":
		return setupCoder(b, topology, numPubs, numSubs)
	default:
		b.Fatalf("invalid -bench.type=%q", *benchType)
		return nil
	}
}

// ---------- embedded server helpers ----------

// freeBenchPort returns a TCP port that was bindable on 127.0.0.1 at the
// moment of the call.
func freeBenchPort(b *testing.B) int {
	b.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen 127.0.0.1:0: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// startStandaloneServer brings up a single embedded nats-server on TCP
// loopback. JetStream/log/sigs are disabled. Cluster mode is off.
func startStandaloneServer(b *testing.B) *natsserver.Server {
	b.Helper()
	opts := &natsserver.Options{
		Host:       "127.0.0.1",
		Port:       natsserver.RANDOM_PORT,
		JetStream:  false,
		NoLog:      true,
		NoSigs:     true,
		ServerName: fmt.Sprintf("bench-solo-%d", time.Now().UnixNano()),
	}
	ns, err := natsserver.NewServer(opts)
	if err != nil {
		b.Fatalf("new standalone server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		ns.WaitForShutdown()
		b.Fatal("standalone server not ready")
	}
	b.Cleanup(func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	})
	return ns
}

// startClusterServers brings up `n` embedded nats-servers in a full-mesh
// cluster. Every server lists every other server's route URL. Returns
// the servers in creation order. All client URLs are TCP loopback.
func startClusterServers(b *testing.B, n int) []*natsserver.Server {
	b.Helper()
	ports := make([]int, n)
	routes := make([]string, n)
	for i := 0; i < n; i++ {
		ports[i] = freeBenchPort(b)
		routes[i] = "nats://127.0.0.1:" + strconv.Itoa(ports[i])
	}
	// Build a full mesh of route URLs (excluding self) for each server.
	parseURLs := func(self int) []*url.URL {
		urls := make([]*url.URL, 0, n-1)
		for i, r := range routes {
			if i == self {
				continue
			}
			u, err := url.Parse(r)
			if err != nil {
				b.Fatalf("parse route %q: %v", r, err)
			}
			urls = append(urls, u)
		}
		return urls
	}

	servers := make([]*natsserver.Server, n)
	for i := 0; i < n; i++ {
		opts := &natsserver.Options{
			Host:       "127.0.0.1",
			Port:       natsserver.RANDOM_PORT,
			JetStream:  false,
			NoLog:      true,
			NoSigs:     true,
			ServerName: fmt.Sprintf("bench-c10-%d-%d", i, time.Now().UnixNano()),
			Cluster: natsserver.ClusterOpts{
				Name: "bench-cluster",
				Host: "127.0.0.1",
				Port: ports[i],
			},
			Routes: parseURLs(i),
		}
		ns, err := natsserver.NewServer(opts)
		if err != nil {
			b.Fatalf("new cluster server %d: %v", i, err)
		}
		go ns.Start()
		if !ns.ReadyForConnections(15 * time.Second) {
			ns.Shutdown()
			ns.WaitForShutdown()
			b.Fatalf("cluster server %d not ready", i)
		}
		servers[i] = ns
		b.Cleanup(func() {
			ns.Shutdown()
			ns.WaitForShutdown()
		})
	}

	// Wait for full mesh of routes (each server should see n-1 peers).
	deadline := time.Now().Add(20 * time.Second)
	for _, ns := range servers {
		for ns.NumRoutes() < n-1 {
			if time.Now().After(deadline) {
				b.Fatalf("cluster routes did not converge: %s has %d routes (want %d)",
					ns.Name(), ns.NumRoutes(), n-1)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return servers
}

// ---------- connection helpers ----------

func benchConnect(b *testing.B, clientURL string, errored, disconnected *atomic.Int64) *natsgo.Conn {
	b.Helper()
	nc, err := natsgo.Connect(clientURL,
		natsgo.MaxReconnects(-1),
		natsgo.IgnoreAuthErrorAbort(),
		natsgo.ErrorHandler(func(_ *natsgo.Conn, _ *natsgo.Subscription, _ error) {
			errored.Add(1)
		}),
		natsgo.DisconnectErrHandler(func(_ *natsgo.Conn, _ error) {
			disconnected.Add(1)
		}),
	)
	if err != nil {
		b.Fatalf("connect %s: %v", clientURL, err)
	}
	b.Cleanup(func() { nc.Close() })
	return nc
}

// ---------- latency percentile helper ----------

func percentileMicros(durs []time.Duration, p float64) float64 {
	if len(durs) == 0 {
		return 0
	}
	idx := int(float64(len(durs)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(durs) {
		idx = len(durs) - 1
	}
	return float64(durs[idx].Microseconds())
}

// ---------- the benchmark ----------

type leafCfg struct {
	name      string
	topology  string // "standalone" | "cluster10"
	subjects  int
	payload   int
	subsTotal int // total subscribers across all servers
	pubs      int // total publishers across all servers
}

func BenchmarkPubsub(b *testing.B) {
	leaves := []leafCfg{}
	for _, topo := range []string{"standalone", "cluster10"} {
		for _, ns := range []int{1, 10} {
			for _, pl := range []int{8 * 1024, 512 * 1024} {
				subs := 100
				pubs := 1
				if topo == "cluster10" {
					subs = 100 * 10
					pubs = 10
				}
				leaves = append(leaves, leafCfg{
					name:      fmt.Sprintf("%s/%s/subj%d/%s", *benchType, topo, ns, iecBytes(pl)),
					topology:  topo,
					subjects:  ns,
					payload:   pl,
					subsTotal: subs,
					pubs:      pubs,
				})
			}
		}
	}

	for _, cfg := range leaves {
		cfg := cfg
		b.Run(cfg.name, func(b *testing.B) {
			runLeaf(b, cfg)
		})
	}
}

func runLeaf(b *testing.B, cfg leafCfg) {
	requireIterBenchtime(b)

	h := newHarness(b, cfg.topology, cfg.pubs, cfg.subsTotal)

	// Capture runtime baseline before any subscriptions are created.
	// The delta reported below isolates "cost of N subscriptions" from
	// "cost of publishing".
	var probe runtimeProbe
	probe.captureBaseline()

	// --- subscriber wiring via the harness ---
	// Distribute subscribers across replicas (round-robin), and across
	// subjects (each subscriber listens on exactly one subject).
	// expectedPerSubject[i] = count of subscribers listening on
	// subjects[i].
	expectedPerSubject := make([]int, cfg.subjects)
	delivered := make([]atomic.Int64, cfg.subjects)

	for s := 0; s < cfg.subsTotal; s++ {
		replicaIdx := s % h.numReplicas
		subjIdx := s % cfg.subjects
		idx := subjIdx
		if err := h.subscribe(replicaIdx, subjIdx, func() {
			delivered[idx].Add(1)
		}); err != nil {
			b.Fatalf("subscribe: %v", err)
		}
		expectedPerSubject[subjIdx]++
	}

	// Flush all subscriber connections so subscriptions are registered
	// at the server before publishers start. For coder mode this is
	// also when we want a brief pause for interest gossip to settle in
	// cluster topologies; see harness.flushPubs.
	if err := h.flushPubs(); err != nil {
		b.Fatalf("flush before publish window: %v", err)
	}
	// For cluster mode, give interest propagation a moment to converge
	// across routes. We use a fixed sleep here rather than poking at
	// the underlying server because the coder harness doesn't expose
	// per-server NumSubscriptions.
	if cfg.topology == "cluster10" {
		time.Sleep(500 * time.Millisecond)
	}

	// --- payload (reusable across publishers) ---
	payload := make([]byte, cfg.payload)
	for i := range payload {
		payload[i] = byte(i)
	}

	// --- worker pool driven by b.Loop() in the main goroutine ---
	// b.Loop() advances "message slots". Each slot is dispatched to a
	// publisher worker via a buffered channel. Workers publish and
	// record per-call latency. After b.Loop() returns we close the
	// work channel, wait for workers, then flush. The publisher window
	// covers from start-barrier release to final flush returning.

	work := make(chan int, cfg.pubs*8)
	latencies := make([][]time.Duration, cfg.pubs)
	publishedPerPub := make([]int64, cfg.pubs)
	startBarrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(cfg.pubs)
	for i := 0; i < cfg.pubs; i++ {
		i := i
		latencies[i] = make([]time.Duration, 0, 1024)
		// Each publisher rotates through subjects on each publish: it
		// always targets subjects[i mod numSubjects] where i is the
		// publisher index.
		subjIdx := i % cfg.subjects
		go func() {
			defer wg.Done()
			<-startBarrier
			for range work {
				start := time.Now()
				if err := h.publish(i, subjIdx, payload); err != nil {
					// Don't fatal from goroutine; surface via error handler counter.
					h.errored.Add(1)
					continue
				}
				latencies[i] = append(latencies[i], time.Since(start))
				publishedPerPub[i]++
			}
		}()
	}

	// All wiring done. Snapshot per-subscription runtime cost just
	// before the publisher window opens.
	probe.captureAfterSetup()

	// All wiring done. Reset timer and start the publisher window.
	b.ResetTimer()
	pubStart := time.Now()
	close(startBarrier)

	loops := int64(0)
	for b.Loop() {
		work <- int(loops)
		loops++
	}
	close(work)
	wg.Wait()
	// Final flush of every publisher to ensure all published bytes
	// have left the client. Counted in the publisher window.
	if err := h.flushPubs(); err != nil {
		b.Fatalf("pub flush: %v", err)
	}
	pubEnd := time.Now()
	b.StopTimer()

	// --- verify total publishes ---
	var totalPublished int64
	for _, n := range publishedPerPub {
		totalPublished += n
	}
	if totalPublished != loops {
		b.Fatalf("published count mismatch: got %d, expected %d", totalPublished, loops)
	}

	// --- wait for delivery to converge ---
	// expected total = sum over publishers of (msgs_pub * subs_on_that_subj)
	publishedPerSubject := make([]int64, cfg.subjects)
	for i := 0; i < cfg.pubs; i++ {
		publishedPerSubject[i%cfg.subjects] += publishedPerPub[i]
	}
	var expectedTotal int64
	for s := 0; s < cfg.subjects; s++ {
		expectedTotal += publishedPerSubject[s] * int64(expectedPerSubject[s])
	}

	// Poll for delivery. Allow generous time scaled by message count
	// and topology (cluster routes add latency).
	settle := 30 * time.Second
	if cfg.topology == "cluster10" {
		settle = 60 * time.Second
	}
	deadline := time.Now().Add(settle)
	deliveryEnd := time.Now()
	for time.Now().Before(deadline) {
		var got int64
		for s := 0; s < cfg.subjects; s++ {
			got += delivered[s].Load()
		}
		if got >= expectedTotal {
			deliveryEnd = time.Now()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// --- compute delivery and report ---
	var gotTotal int64
	shortfalls := make([]string, 0)
	for s := 0; s < cfg.subjects; s++ {
		want := publishedPerSubject[s] * int64(expectedPerSubject[s])
		got := delivered[s].Load()
		gotTotal += got
		if got < want {
			shortfalls = append(shortfalls,
				fmt.Sprintf("subj=%s subs=%d want=%d got=%d short=%d",
					h.subjectName(s), expectedPerSubject[s], want, got, want-got))
		}
	}

	deliveryPct := 0.0
	if expectedTotal > 0 {
		deliveryPct = 100.0 * float64(gotTotal) / float64(expectedTotal)
	}

	// Aggregate latencies across publishers.
	var allLats []time.Duration
	for _, ls := range latencies {
		allLats = append(allLats, ls...)
	}
	sort.Slice(allLats, func(i, j int) bool { return allLats[i] < allLats[j] })

	pubWindow := pubEnd.Sub(pubStart).Seconds()
	deliveryWindow := deliveryEnd.Sub(pubStart).Seconds()
	pubThroughput := float64(totalPublished) / pubWindow
	deliveryThroughput := float64(gotTotal) / deliveryWindow
	deliveryDrain := deliveryEnd.Sub(pubEnd).Seconds()

	b.ReportMetric(pubThroughput, "pub_throughput_per_sec")
	b.ReportMetric(deliveryThroughput, "delivery_throughput_per_sec")
	b.ReportMetric(deliveryDrain, "delivery_drain_sec")
	b.ReportMetric(deliveryPct, "delivery_pct")
	b.ReportMetric(percentileMicros(allLats, 0.50), "pub_p50_us")
	b.ReportMetric(percentileMicros(allLats, 0.99), "pub_p99_us")
	b.ReportMetric(percentileMicros(allLats, 0.999), "pub_p999_us")
	probe.report(b)
	// Suppress default ns/op which is misleading for this multi-worker design.
	b.ReportMetric(0, "ns/op")

	if h.errored.Load() > 0 || h.disconnected.Load() > 0 {
		b.Logf("nats client events: errored=%d disconnected=%d",
			h.errored.Load(), h.disconnected.Load())
	}

	if deliveryPct < 100.0 {
		b.Fatalf("delivery shortfall: %.4f%% (got %d / want %d); %s",
			deliveryPct, gotTotal, expectedTotal, strings.Join(shortfalls, "; "))
	}
}

// ---------- high-cardinality thin fan-out benchmark ----------

// BenchmarkPubsubHighCardinality stresses NATS's subject-routing table
// rather than fan-out width. The matrix mirrors per-workspace /
// per-agent / per-job subject patterns (e.g. workspace_owner:<uuid>,
// agent-logs:<uuid>, chat:stream:<chat>): 1000 or 10000 distinct
// subjects, each with exactly one subscriber. Publishers round-robin
// through the entire subject ring, so every subject is exercised and
// per-publish fan-out is exactly 1.
//
// Operator contract matches BenchmarkPubsub: requires -benchtime=Nx.
func BenchmarkPubsubHighCardinality(b *testing.B) {
	type hcCfg struct {
		name     string
		topology string
		subjects int
		payload  int
		pubs     int
	}
	leaves := []hcCfg{}
	for _, topo := range []string{"standalone", "cluster10"} {
		for _, ns := range []int{1000, 10000} {
			pubs := 1
			if topo == "cluster10" {
				pubs = 10
			}
			leaves = append(leaves, hcCfg{
				name:     fmt.Sprintf("%s/%s/subj%d/%s", *benchType, topo, ns, iecBytes(8*1024)),
				topology: topo,
				subjects: ns,
				payload:  8 * 1024,
				pubs:     pubs,
			})
		}
	}

	for _, cfg := range leaves {
		cfg := cfg
		b.Run(cfg.name, func(b *testing.B) {
			runHighCardinalityLeaf(b, cfg.topology, cfg.subjects, cfg.payload, cfg.pubs)
		})
	}
}

// runHighCardinalityLeaf wires N distinct subjects, one subscriber per
// subject (round-robin across replicas in cluster mode), and `pubs`
// publishers that rotate through the full subject ring per publish.
func runHighCardinalityLeaf(b *testing.B, topology string, numSubjects, payloadBytes, numPubs int) {
	requireIterBenchtime(b)

	// One subscriber per subject => numSubs == numSubjects.
	h := newHarness(b, topology, numPubs, numSubjects)

	var probe runtimeProbe
	probe.captureBaseline()

	// One subscriber per subject, on its own subscription, round-robin
	// across replicas. Each subject thus has exactly one subscriber and
	// per-publish fan-out is 1.
	delivered := make([]atomic.Int64, numSubjects)
	for s := 0; s < numSubjects; s++ {
		replicaIdx := s % h.numReplicas
		idx := s
		if err := h.subscribe(replicaIdx, s, func() {
			delivered[idx].Add(1)
		}); err != nil {
			b.Fatalf("subscribe: %v", err)
		}
	}
	if err := h.flushPubs(); err != nil {
		b.Fatalf("flush before publish window: %v", err)
	}
	if topology == "cluster10" {
		// Interest gossip for 10k subjects takes longer than the
		// small-cardinality case.
		time.Sleep(2 * time.Second)
	}

	payload := make([]byte, payloadBytes)
	for i := range payload {
		payload[i] = byte(i)
	}

	// Each work slot carries the subject index for that publish, so
	// the dispatcher picks the subject and workers just publish.
	type slot struct {
		subjIdx int
	}
	work := make(chan slot, numPubs*8)
	latencies := make([][]time.Duration, numPubs)
	publishedPerPub := make([]int64, numPubs)
	publishedPerSubject := make([]int64, numSubjects)
	var publishedPerSubjectMu sync.Mutex
	startBarrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(numPubs)
	for i := 0; i < numPubs; i++ {
		i := i
		latencies[i] = make([]time.Duration, 0, 1024)
		go func() {
			defer wg.Done()
			localCounts := make(map[int]int64)
			<-startBarrier
			for sl := range work {
				start := time.Now()
				if err := h.publish(i, sl.subjIdx, payload); err != nil {
					h.errored.Add(1)
					continue
				}
				latencies[i] = append(latencies[i], time.Since(start))
				publishedPerPub[i]++
				localCounts[sl.subjIdx]++
			}
			publishedPerSubjectMu.Lock()
			for k, v := range localCounts {
				publishedPerSubject[k] += v
			}
			publishedPerSubjectMu.Unlock()
		}()
	}

	probe.captureAfterSetup()

	b.ResetTimer()
	pubStart := time.Now()
	close(startBarrier)

	loops := int64(0)
	for b.Loop() {
		work <- slot{subjIdx: int(loops % int64(numSubjects))}
		loops++
	}
	close(work)
	wg.Wait()
	if err := h.flushPubs(); err != nil {
		b.Fatalf("pub flush: %v", err)
	}
	pubEnd := time.Now()
	b.StopTimer()

	var totalPublished int64
	for _, n := range publishedPerPub {
		totalPublished += n
	}
	if totalPublished != loops {
		b.Fatalf("published count mismatch: got %d, expected %d", totalPublished, loops)
	}

	// Fan-out is exactly 1 per publish, so expected delivery == total
	// publishes. Per-subject expected = publishedPerSubject[s].
	expectedTotal := totalPublished

	settle := 30 * time.Second
	if topology == "cluster10" {
		settle = 90 * time.Second
	}
	deadline := time.Now().Add(settle)
	deliveryEnd := time.Now()
	for time.Now().Before(deadline) {
		var got int64
		for s := 0; s < numSubjects; s++ {
			got += delivered[s].Load()
		}
		if got >= expectedTotal {
			deliveryEnd = time.Now()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	var gotTotal int64
	shortfalls := make([]string, 0)
	for s := 0; s < numSubjects; s++ {
		want := publishedPerSubject[s]
		got := delivered[s].Load()
		gotTotal += got
		if got < want {
			shortfalls = append(shortfalls,
				fmt.Sprintf("subj=%s want=%d got=%d short=%d",
					h.subjectName(s), want, got, want-got))
			if len(shortfalls) >= 10 {
				shortfalls = append(shortfalls, "...(truncated)")
				break
			}
		}
	}

	deliveryPct := 0.0
	if expectedTotal > 0 {
		deliveryPct = 100.0 * float64(gotTotal) / float64(expectedTotal)
	}

	var allLats []time.Duration
	for _, ls := range latencies {
		allLats = append(allLats, ls...)
	}
	sort.Slice(allLats, func(i, j int) bool { return allLats[i] < allLats[j] })

	pubWindow := pubEnd.Sub(pubStart).Seconds()
	deliveryWindow := deliveryEnd.Sub(pubStart).Seconds()
	pubThroughput := float64(totalPublished) / pubWindow
	deliveryThroughput := float64(gotTotal) / deliveryWindow
	deliveryDrain := deliveryEnd.Sub(pubEnd).Seconds()

	b.ReportMetric(pubThroughput, "pub_throughput_per_sec")
	b.ReportMetric(deliveryThroughput, "delivery_throughput_per_sec")
	b.ReportMetric(deliveryDrain, "delivery_drain_sec")
	b.ReportMetric(deliveryPct, "delivery_pct")
	b.ReportMetric(percentileMicros(allLats, 0.50), "pub_p50_us")
	b.ReportMetric(percentileMicros(allLats, 0.99), "pub_p99_us")
	b.ReportMetric(percentileMicros(allLats, 0.999), "pub_p999_us")
	probe.report(b)
	b.ReportMetric(0, "ns/op")

	if h.errored.Load() > 0 || h.disconnected.Load() > 0 {
		b.Logf("nats client events: errored=%d disconnected=%d",
			h.errored.Load(), h.disconnected.Load())
	}

	if deliveryPct < 100.0 {
		b.Fatalf("delivery shortfall: %.4f%% (got %d / want %d); %s",
			deliveryPct, gotTotal, expectedTotal, strings.Join(shortfalls, "; "))
	}
}

// ---------- hot-subject concentrated fan-out benchmark ----------

// BenchmarkPubsubHotSubjectConcentrated stresses NATS's per-replica
// outbound fan-out for one hot subject. This represents the
// workspace_agent_metadata_batch worst case: one global subject with
// many UI sessions all attached to a single replica, every batch
// delivered to every subscriber. Standalone-only by design: the shape
// is about concentration, not distribution.
//
// At 5000 subscribers / 512 KiB this leaf may approach NATS's default
// MaxPending slow-consumer threshold; if it does, the delivery_pct
// metric will reflect it and the leaf fails loudly rather than masking
// the drop.
func BenchmarkPubsubHotSubjectConcentrated(b *testing.B) {
	type hsCfg struct {
		name    string
		subs    int
		payload int
	}
	leaves := []hsCfg{}
	for _, subs := range []int{1000, 5000} {
		for _, pl := range []int{8 * 1024, 512 * 1024} {
			leaves = append(leaves, hsCfg{
				name:    fmt.Sprintf("%s/standalone/subj1/subs%d/%s", *benchType, subs, iecBytes(pl)),
				subs:    subs,
				payload: pl,
			})
		}
	}
	for _, cfg := range leaves {
		cfg := cfg
		b.Run(cfg.name, func(b *testing.B) {
			runHotSubjectLeaf(b, cfg.subs, cfg.payload)
		})
	}
}

// runHotSubjectLeaf wires one global subject with `numSubs` subscribers
// and a single publisher, then publishes `b.N` messages. Per-publish
// fan-out is numSubs.
func runHotSubjectLeaf(b *testing.B, numSubs, payloadBytes int) {
	requireIterBenchtime(b)

	h := newHarness(b, "standalone", 1, numSubs)

	var probe runtimeProbe
	probe.captureBaseline()

	var delivered atomic.Int64
	for i := 0; i < numSubs; i++ {
		if err := h.subscribe(0, 0, func() { delivered.Add(1) }); err != nil {
			b.Fatalf("subscriber %d subscribe: %v", i, err)
		}
	}
	if err := h.flushPubs(); err != nil {
		b.Fatalf("flush before publish window: %v", err)
	}

	payload := make([]byte, payloadBytes)
	for i := range payload {
		payload[i] = byte(i)
	}

	latencies := make([]time.Duration, 0, 1024)

	probe.captureAfterSetup()

	b.ResetTimer()
	pubStart := time.Now()

	var published int64
	for b.Loop() {
		start := time.Now()
		if err := h.publish(0, 0, payload); err != nil {
			h.errored.Add(1)
			continue
		}
		latencies = append(latencies, time.Since(start))
		published++
	}
	if err := h.flushPubs(); err != nil {
		b.Fatalf("pub flush: %v", err)
	}
	pubEnd := time.Now()
	b.StopTimer()

	expectedTotal := published * int64(numSubs)

	// Generous settle: 512 KiB * 5000 subs = 2.5 GiB to push per
	// publish. Even on loopback, large delivery windows take time.
	settle := 60 * time.Second
	if payloadBytes >= 512*1024 && numSubs >= 5000 {
		settle = 180 * time.Second
	}
	deadline := time.Now().Add(settle)
	deliveryEnd := time.Now()
	for time.Now().Before(deadline) {
		if delivered.Load() >= expectedTotal {
			deliveryEnd = time.Now()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	gotTotal := delivered.Load()
	deliveryPct := 0.0
	if expectedTotal > 0 {
		deliveryPct = 100.0 * float64(gotTotal) / float64(expectedTotal)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	pubWindow := pubEnd.Sub(pubStart).Seconds()
	deliveryWindow := deliveryEnd.Sub(pubStart).Seconds()
	pubThroughput := float64(published) / pubWindow
	deliveryThroughput := float64(gotTotal) / deliveryWindow
	deliveryDrain := deliveryEnd.Sub(pubEnd).Seconds()

	b.ReportMetric(pubThroughput, "pub_throughput_per_sec")
	b.ReportMetric(deliveryThroughput, "delivery_throughput_per_sec")
	b.ReportMetric(deliveryDrain, "delivery_drain_sec")
	b.ReportMetric(deliveryPct, "delivery_pct")
	b.ReportMetric(percentileMicros(latencies, 0.50), "pub_p50_us")
	b.ReportMetric(percentileMicros(latencies, 0.99), "pub_p99_us")
	b.ReportMetric(percentileMicros(latencies, 0.999), "pub_p999_us")
	probe.report(b)
	b.ReportMetric(0, "ns/op")

	if h.errored.Load() > 0 || h.disconnected.Load() > 0 {
		b.Logf("nats client events: errored=%d disconnected=%d",
			h.errored.Load(), h.disconnected.Load())
	}

	if deliveryPct < 100.0 {
		b.Fatalf("delivery shortfall: %.4f%% (got %d / want %d); subs=%d payload=%s",
			deliveryPct, gotTotal, expectedTotal, numSubs, iecBytes(payloadBytes))
	}
}

// ---------- thin-fanout (pgcoord / replicasync) benchmark ----------

// BenchmarkPubsubThinFanout models the "one subscriber per replica"
// pattern used by replicasync.Manager and the pgcoord coordinator: a
// single global subject that every replica subscribes to, with every
// replica also publishing into it. Per-publish fan-out equals the
// cluster size minus the originator's local echo (NATS Core delivers
// to every interested subscriber, including the local one, by
// default).
//
// Matrix (4 leaves): cluster10 only x payload={8KiB,512KiB} x
// -bench.type={native,coder}. Standalone is intentionally excluded:
// the shape is "thin fanout across replicas via routes", and a
// single-replica reduction is the same as native single-sub which the
// existing BenchmarkPubsub already covers.
func BenchmarkPubsubThinFanout(b *testing.B) {
	type tfCfg struct {
		name    string
		payload int
	}
	leaves := []tfCfg{}
	for _, pl := range []int{8 * 1024, 512 * 1024} {
		leaves = append(leaves, tfCfg{
			name:    fmt.Sprintf("%s/cluster10/subj1/%s", *benchType, iecBytes(pl)),
			payload: pl,
		})
	}
	for _, cfg := range leaves {
		cfg := cfg
		b.Run(cfg.name, func(b *testing.B) {
			runThinFanoutLeaf(b, cfg.payload)
		})
	}
}

// runThinFanoutLeaf wires a 10-replica cluster with exactly one
// subscriber per replica on a single global subject, plus one
// publisher per replica. All publishers publish into the same subject;
// every subscriber should receive every message regardless of which
// replica published it (per-publish fan-out is 10).
func runThinFanoutLeaf(b *testing.B, payloadBytes int) {
	requireIterBenchtime(b)

	const numReplicas = 10
	const numSubs = numReplicas // one subscriber per replica
	const numPubs = numReplicas // one publisher per replica
	const subjIdx = 0

	h := newHarness(b, "cluster10", numPubs, numSubs)
	if h.numReplicas != numReplicas {
		b.Fatalf("harness numReplicas = %d; want %d", h.numReplicas, numReplicas)
	}

	var probe runtimeProbe
	probe.captureBaseline()

	// Per-subscriber delivery counters so we can spot one-sided
	// shortfalls.
	delivered := make([]atomic.Int64, numReplicas)
	for r := 0; r < numReplicas; r++ {
		r := r
		if err := h.subscribe(r, subjIdx, func() {
			delivered[r].Add(1)
		}); err != nil {
			b.Fatalf("subscribe replica %d: %v", r, err)
		}
	}
	if err := h.flushPubs(); err != nil {
		b.Fatalf("flush before publish window: %v", err)
	}
	// Settle for route interest gossip across all replicas.
	time.Sleep(500 * time.Millisecond)

	payload := make([]byte, payloadBytes)
	for i := range payload {
		payload[i] = byte(i)
	}

	work := make(chan int, numPubs*8)
	latencies := make([][]time.Duration, numPubs)
	publishedPerPub := make([]int64, numPubs)
	startBarrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(numPubs)
	for i := 0; i < numPubs; i++ {
		i := i
		latencies[i] = make([]time.Duration, 0, 1024)
		go func() {
			defer wg.Done()
			<-startBarrier
			for range work {
				start := time.Now()
				if err := h.publish(i, subjIdx, payload); err != nil {
					h.errored.Add(1)
					continue
				}
				latencies[i] = append(latencies[i], time.Since(start))
				publishedPerPub[i]++
			}
		}()
	}

	probe.captureAfterSetup()

	b.ResetTimer()
	pubStart := time.Now()
	close(startBarrier)

	loops := int64(0)
	for b.Loop() {
		work <- int(loops)
		loops++
	}
	close(work)
	wg.Wait()
	if err := h.flushPubs(); err != nil {
		b.Fatalf("pub flush: %v", err)
	}
	pubEnd := time.Now()
	b.StopTimer()

	var totalPublished int64
	for _, n := range publishedPerPub {
		totalPublished += n
	}
	if totalPublished != loops {
		b.Fatalf("published count mismatch: got %d, expected %d", totalPublished, loops)
	}

	// Each subscriber sees every published message (fan-out =
	// numReplicas).
	expectedPerSub := totalPublished
	expectedTotal := expectedPerSub * int64(numReplicas)

	settle := 60 * time.Second
	deadline := time.Now().Add(settle)
	deliveryEnd := time.Now()
	for time.Now().Before(deadline) {
		var got int64
		for r := 0; r < numReplicas; r++ {
			got += delivered[r].Load()
		}
		if got >= expectedTotal {
			deliveryEnd = time.Now()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	var gotTotal int64
	shortfalls := make([]string, 0)
	for r := 0; r < numReplicas; r++ {
		got := delivered[r].Load()
		gotTotal += got
		if got < expectedPerSub {
			shortfalls = append(shortfalls,
				fmt.Sprintf("replica=%d want=%d got=%d short=%d",
					r, expectedPerSub, got, expectedPerSub-got))
		}
	}

	deliveryPct := 0.0
	if expectedTotal > 0 {
		deliveryPct = 100.0 * float64(gotTotal) / float64(expectedTotal)
	}

	var allLats []time.Duration
	for _, ls := range latencies {
		allLats = append(allLats, ls...)
	}
	sort.Slice(allLats, func(i, j int) bool { return allLats[i] < allLats[j] })

	pubWindow := pubEnd.Sub(pubStart).Seconds()
	deliveryWindow := deliveryEnd.Sub(pubStart).Seconds()
	pubThroughput := float64(totalPublished) / pubWindow
	deliveryThroughput := float64(gotTotal) / deliveryWindow
	deliveryDrain := deliveryEnd.Sub(pubEnd).Seconds()

	b.ReportMetric(pubThroughput, "pub_throughput_per_sec")
	b.ReportMetric(deliveryThroughput, "delivery_throughput_per_sec")
	b.ReportMetric(deliveryDrain, "delivery_drain_sec")
	b.ReportMetric(deliveryPct, "delivery_pct")
	b.ReportMetric(percentileMicros(allLats, 0.50), "pub_p50_us")
	b.ReportMetric(percentileMicros(allLats, 0.99), "pub_p99_us")
	b.ReportMetric(percentileMicros(allLats, 0.999), "pub_p999_us")
	probe.report(b)
	b.ReportMetric(0, "ns/op")

	if h.errored.Load() > 0 || h.disconnected.Load() > 0 {
		b.Logf("nats client events: errored=%d disconnected=%d",
			h.errored.Load(), h.disconnected.Load())
	}

	if deliveryPct < 100.0 {
		b.Fatalf("delivery shortfall: %.4f%% (got %d / want %d); %s",
			deliveryPct, gotTotal, expectedTotal, strings.Join(shortfalls, "; "))
	}
}

// BenchmarkPubsubAllRemote places every subscriber on a replica other
// than the publisher's. A single publisher is bound to replica 0 (which
// has zero local subscribers); N subscribers are distributed round-robin
// across replicas 1-9. This isolates pure route-only fan-out: replica 0
// does no local delivery work and merely forwards each message to its 9
// route peers, which then fan out to their local subscribers.
//
// Contrast with BenchmarkPubsubHotSubjectConcentrated, where every
// subscriber lives on the publisher's replica and the publisher pays the
// full local fan-out cost (suspected appendBufs/WriteBufferSize=32KiB
// bottleneck). If AllRemote's pub_throughput is dramatically higher for
// the same (subs, payload) pair, local fan-out dominates; if similar,
// the cost is elsewhere (e.g., per-publisher-conn flush behavior).
//
// Matrix (4 leaves): cluster10 only x subs={100,1000} x
// payload={8KiB,512KiB} x -bench.type={native,coder}.
func BenchmarkPubsubAllRemote(b *testing.B) {
	type arCfg struct {
		name    string
		numSubs int
		payload int
	}
	leaves := []arCfg{}
	for _, ns := range []int{100, 1000} {
		for _, pl := range []int{8 * 1024, 512 * 1024} {
			leaves = append(leaves, arCfg{
				name:    fmt.Sprintf("%s/cluster10/subj1/subs%d/%s", *benchType, ns, iecBytes(pl)),
				numSubs: ns,
				payload: pl,
			})
		}
	}
	for _, cfg := range leaves {
		cfg := cfg
		b.Run(cfg.name, func(b *testing.B) {
			runAllRemoteLeaf(b, cfg.numSubs, cfg.payload)
		})
	}
}

// runAllRemoteLeaf wires a 10-replica cluster with one publisher on
// replica 0 and numSubs subscribers distributed round-robin across
// replicas 1-9 (replica 0 has zero subscribers). Every subscriber should
// receive every published message via route delivery only.
func runAllRemoteLeaf(b *testing.B, numSubs, payloadBytes int) {
	requireIterBenchtime(b)

	const numReplicas = 10
	const numPubs = 1
	const subjIdx = 0
	const pubReplica = 0

	// The native harness allocates one *nats.Conn per subscriber slot
	// with slot s pinned to replica s%numReplicas. To place numSubs
	// subscribers on replicas 1-9 we need at least
	// ceil(numSubs/9) conns mapped to each remote replica; allocate
	// perRemote*numReplicas slots so each replica (including the
	// unused replica 0) has perRemote conns available.
	perRemote := (numSubs + 8) / 9
	harnessSubs := perRemote * numReplicas

	h := newHarness(b, "cluster10", numPubs, harnessSubs)
	if h.numReplicas != numReplicas {
		b.Fatalf("harness numReplicas = %d; want %d", h.numReplicas, numReplicas)
	}

	var probe runtimeProbe
	probe.captureBaseline()

	// Per-replica delivery counters so a one-sided shortfall is
	// visible. Replica 0 has no subscribers; its counter stays at 0.
	delivered := make([]atomic.Int64, numReplicas)
	subsPerReplica := make([]int, numReplicas)
	for i := 0; i < numSubs; i++ {
		r := 1 + (i % 9)
		subsPerReplica[r]++
		rr := r
		if err := h.subscribe(rr, subjIdx, func() {
			delivered[rr].Add(1)
		}); err != nil {
			b.Fatalf("subscribe replica %d (sub %d): %v", rr, i, err)
		}
	}
	if err := h.flushPubs(); err != nil {
		b.Fatalf("flush before publish window: %v", err)
	}
	// Settle for route interest gossip across all replicas.
	time.Sleep(500 * time.Millisecond)

	payload := make([]byte, payloadBytes)
	for i := range payload {
		payload[i] = byte(i)
	}

	work := make(chan struct{}, 8)
	latencies := make([]time.Duration, 0, 1024)
	var published int64
	startBarrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-startBarrier
		for range work {
			start := time.Now()
			if err := h.publish(pubReplica, subjIdx, payload); err != nil {
				h.errored.Add(1)
				continue
			}
			latencies = append(latencies, time.Since(start))
			published++
		}
	}()

	probe.captureAfterSetup()

	b.ResetTimer()
	pubStart := time.Now()
	close(startBarrier)

	loops := int64(0)
	for b.Loop() {
		work <- struct{}{}
		loops++
	}
	close(work)
	wg.Wait()
	if err := h.flushPubs(); err != nil {
		b.Fatalf("pub flush: %v", err)
	}
	pubEnd := time.Now()
	b.StopTimer()

	if published != loops {
		b.Fatalf("published count mismatch: got %d, expected %d", published, loops)
	}

	// Each subscriber sees every published message; total = pub*numSubs.
	expectedTotal := published * int64(numSubs)

	settle := 30 * time.Second
	deadline := time.Now().Add(settle)
	deliveryEnd := time.Now()
	for time.Now().Before(deadline) {
		var got int64
		for r := 0; r < numReplicas; r++ {
			got += delivered[r].Load()
		}
		if got >= expectedTotal {
			deliveryEnd = time.Now()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	var gotTotal int64
	shortfalls := make([]string, 0)
	for r := 0; r < numReplicas; r++ {
		got := delivered[r].Load()
		gotTotal += got
		want := int64(subsPerReplica[r]) * published
		if got < want {
			shortfalls = append(shortfalls,
				fmt.Sprintf("replica=%d subs=%d want=%d got=%d short=%d",
					r, subsPerReplica[r], want, got, want-got))
		}
	}

	deliveryPct := 0.0
	if expectedTotal > 0 {
		deliveryPct = 100.0 * float64(gotTotal) / float64(expectedTotal)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	pubWindow := pubEnd.Sub(pubStart).Seconds()
	deliveryWindow := deliveryEnd.Sub(pubStart).Seconds()
	pubThroughput := float64(published) / pubWindow
	deliveryThroughput := float64(gotTotal) / deliveryWindow
	deliveryDrain := deliveryEnd.Sub(pubEnd).Seconds()

	b.ReportMetric(pubThroughput, "pub_throughput_per_sec")
	b.ReportMetric(deliveryThroughput, "delivery_throughput_per_sec")
	b.ReportMetric(deliveryDrain, "delivery_drain_sec")
	b.ReportMetric(deliveryPct, "delivery_pct")
	b.ReportMetric(percentileMicros(latencies, 0.50), "pub_p50_us")
	b.ReportMetric(percentileMicros(latencies, 0.99), "pub_p99_us")
	b.ReportMetric(percentileMicros(latencies, 0.999), "pub_p999_us")
	probe.report(b)
	b.ReportMetric(0, "ns/op")

	if h.errored.Load() > 0 || h.disconnected.Load() > 0 {
		b.Logf("nats client events: errored=%d disconnected=%d",
			h.errored.Load(), h.disconnected.Load())
	}

	if deliveryPct < 100.0 {
		b.Fatalf("delivery shortfall: %.4f%% (got %d / want %d); %s",
			deliveryPct, gotTotal, expectedTotal, strings.Join(shortfalls, "; "))
	}
}
