package nats_test

// Capacity-planning benchmarks for raw NATS Core pub/sub.
//
// This bench answers: "how many publishes per second can NATS absorb in
// Coder-shaped workloads, with 100% delivery?". It deliberately avoids
// the coderd/x/nats.Pubsub wrapper and goes straight to natsserver +
// natsgo so it measures NATS capacity, not wrapper overhead.
//
// Matrix (8 leaves): topology={standalone,cluster10} x subjects={1,10}
// x payload={8KiB,512KiB}.
//
// Operator contract: REQUIRES -benchtime=Nx (e.g. -benchtime=1000x).
// Time-based -benchtime (default 1s) is rejected with a clear error so
// nobody silently runs a 1-message bench.
//
// Run examples:
//   go test -run x -bench BenchmarkPubsub -benchtime=1000x \
//     ./coderd/x/nats/ -timeout 30m
//   go test -run x -bench BenchmarkPubsub/standalone -benchtime=500x \
//     ./coderd/x/nats/ -timeout 10m

import (
	"flag"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
)

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
					name:      fmt.Sprintf("%s/subj%d/%s", topo, ns, iecBytes(pl)),
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

	// --- bring up servers (untimed) ---
	var servers []*natsserver.Server
	switch cfg.topology {
	case "standalone":
		servers = []*natsserver.Server{startStandaloneServer(b)}
	case "cluster10":
		servers = startClusterServers(b, 10)
	default:
		b.Fatalf("unknown topology %q", cfg.topology)
	}

	var errored, disconnected atomic.Int64

	// --- subjects ---
	subjects := make([]string, cfg.subjects)
	for i := range subjects {
		subjects[i] = fmt.Sprintf("bench.subj.%d", i)
	}

	// --- subscriber wiring ---
	// Distribute subscribers across servers (round-robin), and across
	// subjects (each subscriber listens on exactly one subject).
	// expectedPerSubject[i] = count of subscribers listening on subjects[i].
	expectedPerSubject := make([]int, cfg.subjects)
	delivered := make([]atomic.Int64, cfg.subjects)

	subConns := make([]*natsgo.Conn, 0, cfg.subsTotal)
	for s := 0; s < cfg.subsTotal; s++ {
		serverIdx := s % len(servers)
		subjIdx := s % cfg.subjects
		nc := benchConnect(b, servers[serverIdx].ClientURL(), &errored, &disconnected)
		subConns = append(subConns, nc)
		idx := subjIdx
		sub, err := nc.Subscribe(subjects[subjIdx], func(_ *natsgo.Msg) {
			delivered[idx].Add(1)
		})
		if err != nil {
			b.Fatalf("subscribe: %v", err)
		}
		sub.SetPendingLimits(-1, -1)
		expectedPerSubject[subjIdx]++
	}
	// Flush all subscriber connections so subscriptions are registered
	// at the server before publishers start.
	for _, nc := range subConns {
		if err := nc.Flush(); err != nil {
			b.Fatalf("sub flush: %v", err)
		}
	}
	// For cluster mode, give interest propagation a moment to converge
	// across routes.
	if cfg.topology == "cluster10" {
		// Wait until every server reports at least one subscription
		// for each subject we'll publish to. We use NumSubscriptions
		// as a proxy for interest. We allow time for route gossip.
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			ok := true
			for _, ns := range servers {
				// Each server has its directly-attached subs plus
				// routed interest; total should be >= number of
				// subjects covered locally + remote interest count.
				if ns.NumSubscriptions() == 0 {
					ok = false
					break
				}
			}
			if ok {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		// Brief extra settle for interest gossip.
		time.Sleep(200 * time.Millisecond)
	}

	// --- publisher wiring ---
	// One publisher per (server, slot). For standalone: 1 pub on the
	// one server. For cluster10: 10 pubs, one per server.
	pubConns := make([]*natsgo.Conn, cfg.pubs)
	for i := 0; i < cfg.pubs; i++ {
		serverIdx := i % len(servers)
		pubConns[i] = benchConnect(b, servers[serverIdx].ClientURL(), &errored, &disconnected)
	}

	// Pre-build one reusable *nats.Msg per publisher. Each publisher
	// rotates through subjects on each publish: it always targets
	// subjects[i mod numSubjects] where i is the publisher index.
	payload := make([]byte, cfg.payload)
	for i := range payload {
		payload[i] = byte(i)
	}
	pubMsgs := make([]*natsgo.Msg, cfg.pubs)
	for i := 0; i < cfg.pubs; i++ {
		pubMsgs[i] = &natsgo.Msg{Subject: subjects[i%cfg.subjects], Data: payload}
	}

	// --- worker pool driven by b.Loop() in the main goroutine ---
	// b.Loop() advances "message slots". Each slot is dispatched to a
	// publisher worker via a buffered channel. Workers publish and
	// record per-call latency. After b.Loop() returns we close the
	// work channel, wait for workers, then flush all publisher
	// connections. The publisher window covers from start-barrier
	// release to final Flush returning.

	work := make(chan int, cfg.pubs*8)
	latencies := make([][]time.Duration, cfg.pubs)
	publishedPerPub := make([]int64, cfg.pubs)
	startBarrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(cfg.pubs)
	for i := 0; i < cfg.pubs; i++ {
		i := i
		latencies[i] = make([]time.Duration, 0, 1024)
		go func() {
			defer wg.Done()
			nc := pubConns[i]
			msg := pubMsgs[i]
			<-startBarrier
			for range work {
				start := time.Now()
				if err := nc.PublishMsg(msg); err != nil {
					// Don't fatal from goroutine; surface via error handler counter.
					errored.Add(1)
					continue
				}
				latencies[i] = append(latencies[i], time.Since(start))
				publishedPerPub[i]++
			}
		}()
	}

	// All wiring done. Reset timer and start the publisher window.
	b.ResetTimer()
	// Round-robin slots across publishers via the shared channel.
	// Capture the window start = barrier release.
	windowStart := time.Now()
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
	for _, nc := range pubConns {
		if err := nc.Flush(); err != nil {
			b.Fatalf("pub flush: %v", err)
		}
	}
	windowElapsed := time.Since(windowStart)
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
	for time.Now().Before(deadline) {
		var got int64
		for s := 0; s < cfg.subjects; s++ {
			got += delivered[s].Load()
		}
		if got >= expectedTotal {
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
					subjects[s], expectedPerSubject[s], want, got, want-got))
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

	pubsPerSec := float64(totalPublished) / windowElapsed.Seconds()

	b.ReportMetric(pubsPerSec, "pubs/s")
	b.ReportMetric(deliveryPct, "delivery_pct")
	b.ReportMetric(percentileMicros(allLats, 0.50), "pub_p50_us")
	b.ReportMetric(percentileMicros(allLats, 0.99), "pub_p99_us")
	b.ReportMetric(percentileMicros(allLats, 0.999), "pub_p999_us")
	// Suppress default ns/op which is misleading for this multi-worker design.
	b.ReportMetric(0, "ns/op")

	if errored.Load() > 0 || disconnected.Load() > 0 {
		b.Logf("nats client events: errored=%d disconnected=%d",
			errored.Load(), disconnected.Load())
	}

	if deliveryPct < 100.0 {
		b.Fatalf("delivery shortfall: %.4f%% (got %d / want %d); %s",
			deliveryPct, gotTotal, expectedTotal, strings.Join(shortfalls, "; "))
	}
}
