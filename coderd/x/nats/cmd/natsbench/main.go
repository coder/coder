// Command natsbench is a standalone benchmark harness modeled after
// upstream `nats bench`. It measures publish/deliver throughput for
// several transports: raw TCP loopback, raw net.Pipe, embedded NATS
// (TCP or in-process), and the coderd/x/nats Pubsub wrapper (TCP or
// in-process).
//
// All publishers share one subject; all subscribers subscribe to that
// subject. Total publish work is -msgs messages split across -pubs
// publishers. Each subscriber expects to receive -msgs messages (full
// fan-out). Wall-clock is measured around the hot loop only.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	codernats "github.com/coder/coder/v2/coderd/x/nats"
)

func main() {
	mode := flag.String("mode", "", "one of: loopback-tcp, loopback-pipe, native-tcp, native-inproc, coder-tcp, coder-inproc, native-cluster, coder-cluster, native-cluster-symmetric, coder-cluster-symmetric")
	msgs := flag.Int("msgs", 1_000_000, "total messages to publish (shared across publishers)")
	size := flag.Int("size", 128, "payload size in bytes")
	pubs := flag.Int("pubs", 1, "number of publisher goroutines")
	subs := flag.Int("subs", 1, "number of subscriber goroutines")
	subj := flag.String("subj", "bench", "subject (NATS modes only)")
	timeout := flag.Duration("timeout", 5*time.Minute, "max wait for subscribers to drain")
	replicas := flag.Int("replicas", 10, "number of replicas for *-cluster modes (ignored elsewhere)")
	cpuProfile := flag.String("cpuprofile", "", "write a CPU profile of the hot phase to this path")
	memProfile := flag.String("memprofile", "", "write a heap profile of live memory after the hot phase to this path")
	flag.Parse()

	cpuProfilePath = *cpuProfile
	memProfilePath = *memProfile

	if *mode == "" {
		_, _ = fmt.Fprintln(os.Stderr, "natsbench: -mode is required")
		flag.Usage()
		os.Exit(2)
	}
	if *msgs <= 0 || *size <= 0 || *pubs <= 0 || *subs < 0 {
		_, _ = fmt.Fprintln(os.Stderr, "natsbench: -msgs/-size/-pubs must be > 0 and -subs must be >= 0")
		os.Exit(2)
	}

	if err := run(*mode, *msgs, *size, *pubs, *subs, *subj, *timeout, *replicas); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "natsbench: %v\n", err)
		os.Exit(1)
	}
}

type result struct {
	setup      time.Duration
	pubHot     time.Duration // publish loop start -> all publishers flushed
	deliverHot time.Duration // end-to-end window: from publish start to last subscriber reaching its target count
	published  int64
	delivered  int64
	subCount   int // number of subscribers in the run
	rstats     runtimeStats
	// symmetric is true for *-cluster-symmetric modes where -msgs is
	// interpreted per-publisher (not total). It only affects the header
	// line and delivery-count display, not any timing math.
	symmetric bool
}

type runtimeStats struct {
	goroutines int
	mallocs    uint64
	bytes      uint64
	gcCycles   uint32
	gcPauseNs  uint64
}

// cpuProfilePath/memProfilePath are populated from flags in main and read by
// hotStart/hotEnd so each runner can bracket its hot phase without plumbing
// the flag values through every signature.
var (
	cpuProfilePath string
	memProfilePath string
)

// hotStart snapshots runtime stats and (if -cpuprofile is set) begins CPU
// profiling. The returned MemStats must be passed back to hotEnd.
func hotStart() runtime.MemStats {
	if cpuProfilePath != "" {
		f, err := os.Create(cpuProfilePath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "natsbench: create cpuprofile: %v\n", err)
		} else if err := pprof.StartCPUProfile(f); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "natsbench: start cpuprofile: %v\n", err)
			_ = f.Close()
		}
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms
}

// hotEnd stops the CPU profile, writes the heap profile if requested, and
// returns the delta runtime stats relative to the snapshot from hotStart.
func hotEnd(before runtime.MemStats) runtimeStats {
	if cpuProfilePath != "" {
		pprof.StopCPUProfile()
	}
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if memProfilePath != "" {
		// Force a GC so the heap profile reflects live, reachable memory
		// at the moment the hot phase ended rather than transient garbage.
		runtime.GC() //nolint:revive // explicit GC is intentional before WriteHeapProfile
		f, err := os.Create(memProfilePath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "natsbench: create memprofile: %v\n", err)
		} else {
			if err := pprof.WriteHeapProfile(f); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "natsbench: write memprofile: %v\n", err)
			}
			_ = f.Close()
		}
	}
	return runtimeStats{
		goroutines: runtime.NumGoroutine(),
		mallocs:    after.Mallocs - before.Mallocs,
		bytes:      after.TotalAlloc - before.TotalAlloc,
		gcCycles:   after.NumGC - before.NumGC,
		gcPauseNs:  after.PauseTotalNs - before.PauseTotalNs,
	}
}

func run(mode string, msgs, size, pubs, subs int, subj string, timeout time.Duration, replicas int) error {
	switch mode {
	case "loopback-tcp", "loopback-pipe":
		// Loopback modes ignore -pubs/-subs/-subj: single writer, single
		// reader, raw byte stream. Echo back the chosen mode header for
		// the user.
		_, _ = fmt.Printf("mode=%s msgs=%d size=%d\n", mode, msgs, size)
		res, err := runLoopback(mode, msgs, size)
		if err != nil {
			return err
		}
		printResult(mode, res, msgs, size, 1, 1)
		return nil
	}

	isCluster := mode == "native-cluster" || mode == "coder-cluster"
	isClusterSym := mode == "native-cluster-symmetric" || mode == "coder-cluster-symmetric"
	if isCluster || isClusterSym {
		if replicas < 1 {
			return xerrors.Errorf("-replicas must be >= 1 for %s", mode)
		}
		if isClusterSym {
			// -msgs is interpreted per-publisher in symmetric modes; the
			// suffix makes that semantic difference explicit.
			_, _ = fmt.Printf("mode=%s pubs=%d subs=%d msgs=%d size=%d replicas=%d (msgs/pub)\n", mode, pubs, subs, msgs, size, replicas)
		} else {
			_, _ = fmt.Printf("mode=%s pubs=%d subs=%d msgs=%d size=%d replicas=%d\n", mode, pubs, subs, msgs, size, replicas)
		}
	} else {
		_, _ = fmt.Printf("mode=%s pubs=%d subs=%d msgs=%d size=%d\n", mode, pubs, subs, msgs, size)
	}
	var (
		res result
		err error
	)
	switch mode {
	case "native-tcp":
		res, err = runNative(false, msgs, size, pubs, subs, subj, timeout)
	case "native-inproc":
		res, err = runNative(true, msgs, size, pubs, subs, subj, timeout)
	case "coder-tcp":
		res, err = runCoder(false, msgs, size, pubs, subs, subj, timeout)
	case "coder-inproc":
		res, err = runCoder(true, msgs, size, pubs, subs, subj, timeout)
	case "native-cluster":
		res, err = runNativeCluster(msgs, size, pubs, subs, subj, timeout, replicas)
	case "coder-cluster":
		res, err = runCoderCluster(msgs, size, pubs, subs, subj, timeout, replicas)
	case "native-cluster-symmetric":
		res, err = runNativeClusterSymmetric(msgs, size, pubs, subs, subj, timeout, replicas)
	case "coder-cluster-symmetric":
		res, err = runCoderClusterSymmetric(msgs, size, pubs, subs, subj, timeout, replicas)
	default:
		return xerrors.Errorf("unknown mode %q", mode)
	}
	if err != nil {
		return err
	}
	printResult(mode, res, msgs, size, pubs, subs)
	return nil
}

// runLoopback measures the raw byte ceiling for TCP loopback or
// net.Pipe by transferring msgs * size bytes from a single writer to a
// single reader.
func runLoopback(mode string, msgs, size int) (result, error) {
	var (
		w, r net.Conn
		err  error
	)
	t0 := time.Now()
	switch mode {
	case "loopback-tcp":
		w, r, err = tcpPair()
	case "loopback-pipe":
		w, r = net.Pipe()
	default:
		return result{}, xerrors.Errorf("unknown loopback mode %q", mode)
	}
	if err != nil {
		return result{}, err
	}
	defer func() { _ = w.Close() }()
	defer func() { _ = r.Close() }()
	setup := time.Since(t0)

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	var (
		writeErr error
		wg       sync.WaitGroup
	)
	wg.Add(1)
	start := make(chan struct{})
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < msgs; i++ {
			if _, werr := w.Write(payload); werr != nil {
				writeErr = werr
				return
			}
		}
	}()
	memBefore := hotStart()
	hotStartT := time.Now()
	close(start)

	scratch := make([]byte, size)
	var delivered int64
	for i := 0; i < msgs; i++ {
		if _, rerr := io.ReadFull(r, scratch); rerr != nil {
			return result{}, xerrors.Errorf("read: %w", rerr)
		}
		delivered++
	}
	wg.Wait()
	if writeErr != nil {
		return result{}, xerrors.Errorf("write: %w", writeErr)
	}
	hot := time.Since(hotStartT)
	rs := hotEnd(memBefore)
	return result{
		setup:      setup,
		pubHot:     hot,
		deliverHot: hot,
		published:  int64(msgs),
		delivered:  delivered,
		subCount:   1,
		rstats:     rs,
	}, nil
}

func tcpPair() (client net.Conn, server net.Conn, err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = ln.Close() }()
	type accepted struct {
		c   net.Conn
		err error
	}
	ch := make(chan accepted, 1)
	go func() {
		c, aerr := ln.Accept()
		ch <- accepted{c, aerr}
	}()
	dialed, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		return nil, nil, err
	}
	a := <-ch
	if a.err != nil {
		_ = dialed.Close()
		return nil, nil, a.err
	}
	return dialed, a.c, nil
}

// runNative runs the bench against an embedded natsserver with raw
// nats.go clients. Each publisher and subscriber gets its own *nats.Conn.
//
//nolint:revive // inProcess is a transport selector, not a control flag.
func runNative(inProcess bool, msgs, size, pubs, subs int, subj string, timeout time.Duration) (result, error) {
	t0 := time.Now()
	sopts := &natsserver.Options{
		JetStream:  false,
		ServerName: fmt.Sprintf("natsbench-%d", os.Getpid()),
		Host:       "127.0.0.1",
		Port:       natsserver.RANDOM_PORT,
		NoLog:      true,
		NoSigs:     true,
		MaxPayload: 64 * 1024 * 1024,
		MaxPending: 1 << 30,
	}
	ns, err := natsserver.NewServer(sopts)
	if err != nil {
		return result{}, xerrors.Errorf("new server: %w", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		return result{}, xerrors.New("nats server not ready")
	}
	defer func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	}()

	connect := func(name string) (*natsgo.Conn, error) {
		opts := []natsgo.Option{
			natsgo.Name(name),
			natsgo.MaxReconnects(-1),
		}
		if inProcess {
			opts = append(opts, natsgo.InProcessServer(ns))
		}
		return natsgo.Connect(ns.ClientURL(), opts...)
	}

	type subState struct {
		nc     *natsgo.Conn
		sub    *natsgo.Subscription
		count  atomic.Int64
		done   chan struct{}
		expect int64
	}
	subStates := make([]*subState, subs)
	for i := 0; i < subs; i++ {
		nc, cerr := connect(fmt.Sprintf("natsbench-sub-%d", i))
		if cerr != nil {
			return result{}, xerrors.Errorf("connect sub %d: %w", i, cerr)
		}
		st := &subState{nc: nc, done: make(chan struct{}), expect: int64(msgs)}
		sub, serr := nc.Subscribe(subj, func(_ *natsgo.Msg) {
			n := st.count.Add(1)
			if n == st.expect {
				close(st.done)
			}
		})
		if serr != nil {
			return result{}, xerrors.Errorf("subscribe sub %d: %w", i, serr)
		}
		if err := sub.SetPendingLimits(-1, -1); err != nil {
			return result{}, xerrors.Errorf("pending limits sub %d: %w", i, err)
		}
		if err := nc.Flush(); err != nil {
			return result{}, xerrors.Errorf("flush sub %d: %w", i, err)
		}
		st.sub = sub
		subStates[i] = st
	}

	pubConns := make([]*natsgo.Conn, pubs)
	for i := 0; i < pubs; i++ {
		nc, cerr := connect(fmt.Sprintf("natsbench-pub-%d", i))
		if cerr != nil {
			return result{}, xerrors.Errorf("connect pub %d: %w", i, cerr)
		}
		pubConns[i] = nc
	}
	setup := time.Since(t0)

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	perPub, rem := msgs/pubs, msgs%pubs
	var wg sync.WaitGroup
	var publishErr atomic.Value // error
	start := make(chan struct{})
	for i := 0; i < pubs; i++ {
		n := perPub
		if i == 0 {
			n += rem
		}
		nc := pubConns[i]
		wg.Add(1)
		go func(nc *natsgo.Conn, n int) {
			defer wg.Done()
			<-start
			for j := 0; j < n; j++ {
				if err := nc.Publish(subj, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			if err := nc.Flush(); err != nil {
				publishErr.Store(err)
			}
		}(nc, n)
	}
	memBefore := hotStart()
	hotStartT := time.Now()
	close(start)
	wg.Wait()
	pubDone := time.Now()
	if v := publishErr.Load(); v != nil {
		perr, _ := v.(error)
		return result{}, xerrors.Errorf("publish: %w", perr)
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for _, st := range subStates {
		select {
		case <-st.done:
		case <-deadline.C:
			var delivered int64
			for _, s := range subStates {
				delivered += s.count.Load()
			}
			return result{}, xerrors.Errorf("timeout: delivered %d of %d (subs=%d)", delivered, int64(msgs)*int64(subs), subs)
		}
	}
	subDone := time.Now()
	pubHot := pubDone.Sub(hotStartT)
	deliverHot := subDone.Sub(hotStartT)
	if subs == 0 {
		deliverHot = pubHot
	}
	rs := hotEnd(memBefore)

	var delivered int64
	for _, st := range subStates {
		delivered += st.count.Load()
		st.nc.Close()
	}
	for _, nc := range pubConns {
		nc.Close()
	}
	return result{
		setup:      setup,
		pubHot:     pubHot,
		deliverHot: deliverHot,
		published:  int64(msgs),
		delivered:  delivered,
		subCount:   subs,
		rstats:     rs,
	}, nil
}

// runCoder runs the bench against the coderd/x/nats Pubsub wrapper.
// One Pubsub instance is shared by all publishers and subscribers
// (the wrapper multiplexes via its dual-conn design).
//
//nolint:revive // inProcess is a transport selector, not a control flag.
func runCoder(inProcess bool, msgs, size, pubs, subs int, subj string, timeout time.Duration) (result, error) {
	t0 := time.Now()
	logger := slog.Make() // discard
	ps, err := codernats.New(context.Background(), logger, codernats.Options{
		InProcess: inProcess,
		PendingLimits: codernats.PendingLimits{
			Msgs:  -1,
			Bytes: -1,
		},
	})
	if err != nil {
		return result{}, xerrors.Errorf("new pubsub: %w", err)
	}
	defer ps.Close()

	type subState struct {
		count  atomic.Int64
		done   chan struct{}
		expect int64
		cancel func()
	}
	subStates := make([]*subState, subs)
	for i := 0; i < subs; i++ {
		st := &subState{done: make(chan struct{}), expect: int64(msgs)}
		cancel, serr := ps.Subscribe(subj, func(_ context.Context, _ []byte) {
			n := st.count.Add(1)
			if n == st.expect {
				close(st.done)
			}
		})
		if serr != nil {
			return result{}, xerrors.Errorf("subscribe %d: %w", i, serr)
		}
		st.cancel = cancel
		subStates[i] = st
	}
	setup := time.Since(t0)

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	perPub, rem := msgs/pubs, msgs%pubs
	var wg sync.WaitGroup
	var publishErr atomic.Value
	start := make(chan struct{})
	for i := 0; i < pubs; i++ {
		n := perPub
		if i == 0 {
			n += rem
		}
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			for j := 0; j < n; j++ {
				if err := ps.Publish(subj, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			if err := ps.Flush(); err != nil {
				publishErr.Store(err)
			}
		}(n)
	}
	memBefore := hotStart()
	hotStartT := time.Now()
	close(start)
	wg.Wait()
	pubDone := time.Now()
	if v := publishErr.Load(); v != nil {
		perr, _ := v.(error)
		return result{}, xerrors.Errorf("publish: %w", perr)
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for _, st := range subStates {
		select {
		case <-st.done:
		case <-deadline.C:
			var delivered int64
			for _, s := range subStates {
				delivered += s.count.Load()
			}
			return result{}, xerrors.Errorf("timeout: delivered %d of %d (subs=%d)", delivered, int64(msgs)*int64(subs), subs)
		}
	}
	subDone := time.Now()
	pubHot := pubDone.Sub(hotStartT)
	deliverHot := subDone.Sub(hotStartT)
	if subs == 0 {
		deliverHot = pubHot
	}
	rs := hotEnd(memBefore)

	var delivered int64
	for _, st := range subStates {
		delivered += st.count.Load()
		st.cancel()
	}
	return result{
		setup:      setup,
		pubHot:     pubHot,
		deliverHot: deliverHot,
		published:  int64(msgs),
		delivered:  delivered,
		subCount:   subs,
		rstats:     rs,
	}, nil
}

func printResult(mode string, r result, msgs, size, pubs, subs int) {
	_ = mode
	_ = pubs
	_, _ = fmt.Printf("setup: %s\n", r.setup)
	_, _ = fmt.Printf("publish duration: %s\n", r.pubHot)
	if subs > 0 {
		_, _ = fmt.Printf("end-to-end deliver duration: %s\n", r.deliverHot)
	}
	_, _ = fmt.Printf("total msgs published: %d\n", r.published)
	if subs > 0 {
		// In symmetric cluster modes -msgs is per-publisher, so each
		// subscriber's target is msgs*pubs rather than msgs.
		perSub := msgs
		if r.symmetric {
			perSub = msgs * pubs
		}
		_, _ = fmt.Printf("total msgs delivered: %d (%d subs x %d)\n", r.delivered, r.subCount, perSub)
	}
	pubSecs := r.pubHot.Seconds()
	delSecs := r.deliverHot.Seconds()
	if pubSecs <= 0 || delSecs <= 0 {
		_, _ = fmt.Println("hot duration <= 0, skipping rate")
		printRuntimeStats(r.rstats, msgs, subs)
		return
	}
	pubRate := float64(r.published) / pubSecs
	pubBps := pubRate * float64(size)
	_, _ = fmt.Printf("publish rate: %s msgs/s, %s/s\n", humanCount(pubRate), humanBytes(pubBps))
	if subs > 0 {
		delRate := float64(r.delivered) / delSecs
		delBps := delRate * float64(size)
		_, _ = fmt.Printf("end-to-end delivery rate: %s msgs/s, %s/s\n", humanCount(delRate), humanBytes(delBps))
	}
	// Aggregate counts each message once on publish plus once per subscriber
	// delivery, measured over the end-to-end window. When subs == 0,
	// r.delivered == 0 and this collapses to the publish rate.
	aggMsgs := r.published + r.delivered
	aggRate := float64(aggMsgs) / delSecs
	aggBps := aggRate * float64(size)
	_, _ = fmt.Printf("aggregate rate: %s msgs/s, %s/s\n", humanCount(aggRate), humanBytes(aggBps))
	printRuntimeStats(r.rstats, msgs, subs)
}

func printRuntimeStats(rs runtimeStats, msgs, subs int) {
	// Aggregate work units: one publish + one delivery per subscriber per
	// message. With subs == 0 we only have publish-side work.
	units := int64(msgs) * int64(1+subs)
	if subs == 0 {
		units = int64(msgs)
	}
	var perMsg uint64
	if units > 0 {
		perMsg = rs.bytes / uint64(units)
	}
	_, _ = fmt.Println("runtime stats:")
	_, _ = fmt.Printf("  goroutines (end): %d\n", rs.goroutines)
	_, _ = fmt.Printf("  allocs:    %d new objects (%d bytes)\n", rs.mallocs, rs.bytes)
	_, _ = fmt.Printf("  gc pauses: %d cycles, total %dms\n", rs.gcCycles, rs.gcPauseNs/1_000_000)
	_, _ = fmt.Printf("  per-msg:   %d bytes/msg allocated\n", perMsg)
}

// runNativeCluster runs the bench against an N-replica full-mesh
// embedded NATS cluster using bare nats.go clients. Publishers connect
// to replica 0; subscribers are round-robin-distributed across replicas
// 1..N-1 so every published message must traverse a cluster route.
//
// When replicas==1 there are no remote replicas; subscribers all
// attach to replica 0 alongside the publishers. This degrades to the
// runNative shape but preserves the cluster-mode flag plumbing.
func runNativeCluster(msgs, size, pubs, subs int, subj string, timeout time.Duration, replicas int) (result, error) {
	t0 := time.Now()
	servers, err := startNativeCluster(replicas, 0)
	if err != nil {
		return result{}, xerrors.Errorf("start native cluster: %w", err)
	}
	defer func() {
		for _, ns := range servers {
			ns.Shutdown()
			ns.WaitForShutdown()
		}
	}()

	// Publishers go to replica 0; subscribers spread across 1..N-1.
	// When there are no remote replicas (N==1), fall back to replica 0
	// for subscribers too.
	pubReplica := servers[0]
	subReplicaAt := func(i int) *natsserver.Server {
		if replicas <= 1 {
			return servers[0]
		}
		return servers[1+(i%(replicas-1))]
	}

	connect := func(ns *natsserver.Server, name string) (*natsgo.Conn, error) {
		return natsgo.Connect(ns.ClientURL(),
			natsgo.Name(name),
			natsgo.MaxReconnects(-1),
		)
	}

	type subState struct {
		nc     *natsgo.Conn
		sub    *natsgo.Subscription
		count  atomic.Int64
		done   chan struct{}
		expect int64
	}
	subStates := make([]*subState, subs)
	for i := 0; i < subs; i++ {
		nc, cerr := connect(subReplicaAt(i), fmt.Sprintf("natsbench-sub-%d", i))
		if cerr != nil {
			return result{}, xerrors.Errorf("connect sub %d: %w", i, cerr)
		}
		st := &subState{nc: nc, done: make(chan struct{}), expect: int64(msgs)}
		sub, serr := nc.Subscribe(subj, func(_ *natsgo.Msg) {
			n := st.count.Add(1)
			if n == st.expect {
				close(st.done)
			}
		})
		if serr != nil {
			return result{}, xerrors.Errorf("subscribe sub %d: %w", i, serr)
		}
		if err := sub.SetPendingLimits(-1, -1); err != nil {
			return result{}, xerrors.Errorf("pending limits sub %d: %w", i, err)
		}
		if err := nc.Flush(); err != nil {
			return result{}, xerrors.Errorf("flush sub %d: %w", i, err)
		}
		st.sub = sub
		subStates[i] = st
	}

	pubConns := make([]*natsgo.Conn, pubs)
	for i := 0; i < pubs; i++ {
		nc, cerr := connect(pubReplica, fmt.Sprintf("natsbench-pub-%d", i))
		if cerr != nil {
			return result{}, xerrors.Errorf("connect pub %d: %w", i, cerr)
		}
		pubConns[i] = nc
	}
	setup := time.Since(t0)

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	perPub, rem := msgs/pubs, msgs%pubs
	var wg sync.WaitGroup
	var publishErr atomic.Value
	start := make(chan struct{})
	for i := 0; i < pubs; i++ {
		n := perPub
		if i == 0 {
			n += rem
		}
		nc := pubConns[i]
		wg.Add(1)
		go func(nc *natsgo.Conn, n int) {
			defer wg.Done()
			<-start
			for j := 0; j < n; j++ {
				if err := nc.Publish(subj, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			if err := nc.Flush(); err != nil {
				publishErr.Store(err)
			}
		}(nc, n)
	}
	memBefore := hotStart()
	hotStartT := time.Now()
	close(start)
	wg.Wait()
	pubDone := time.Now()
	if v := publishErr.Load(); v != nil {
		perr, _ := v.(error)
		return result{}, xerrors.Errorf("publish: %w", perr)
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for _, st := range subStates {
		select {
		case <-st.done:
		case <-deadline.C:
			var delivered int64
			for _, s := range subStates {
				delivered += s.count.Load()
			}
			return result{}, xerrors.Errorf("timeout: delivered %d of %d (subs=%d)", delivered, int64(msgs)*int64(subs), subs)
		}
	}
	subDone := time.Now()
	pubHot := pubDone.Sub(hotStartT)
	deliverHot := subDone.Sub(hotStartT)
	if subs == 0 {
		deliverHot = pubHot
	}
	rs := hotEnd(memBefore)

	var delivered int64
	for _, st := range subStates {
		delivered += st.count.Load()
		st.nc.Close()
	}
	for _, nc := range pubConns {
		nc.Close()
	}
	return result{
		setup:      setup,
		pubHot:     pubHot,
		deliverHot: deliverHot,
		published:  int64(msgs),
		delivered:  delivered,
		subCount:   subs,
		rstats:     rs,
	}, nil
}

// runCoderCluster runs the bench against an N-replica full-mesh
// coderd/x/nats.Pubsub cluster. Publishers go to replica 0 (a single
// *Pubsub instance, since the wrapper multiplexes internally);
// subscribers register against replicas 1..N-1 round-robin so every
// published message must cross a route. With replicas==1, subscribers
// attach to replica 0 (degrades to runCoder shape).
func runCoderCluster(msgs, size, pubs, subs int, subj string, timeout time.Duration, replicas int) (result, error) {
	t0 := time.Now()
	logger := slog.Make() // discard
	pubsubs, err := startCoderCluster(context.Background(), logger, replicas, 0)
	if err != nil {
		return result{}, xerrors.Errorf("start coder cluster: %w", err)
	}
	defer func() {
		for _, p := range pubsubs {
			_ = p.Close()
		}
	}()

	pubPS := pubsubs[0]
	subPSAt := func(i int) *codernats.Pubsub {
		if replicas <= 1 {
			return pubsubs[0]
		}
		return pubsubs[1+(i%(replicas-1))]
	}

	type subState struct {
		count  atomic.Int64
		done   chan struct{}
		expect int64
		cancel func()
	}
	subStates := make([]*subState, subs)
	for i := 0; i < subs; i++ {
		st := &subState{done: make(chan struct{}), expect: int64(msgs)}
		cancel, serr := subPSAt(i).Subscribe(subj, func(_ context.Context, _ []byte) {
			n := st.count.Add(1)
			if n == st.expect {
				close(st.done)
			}
		})
		if serr != nil {
			return result{}, xerrors.Errorf("subscribe %d: %w", i, serr)
		}
		st.cancel = cancel
		subStates[i] = st
	}
	setup := time.Since(t0)

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	perPub, rem := msgs/pubs, msgs%pubs
	var wg sync.WaitGroup
	var publishErr atomic.Value
	start := make(chan struct{})
	for i := 0; i < pubs; i++ {
		n := perPub
		if i == 0 {
			n += rem
		}
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			for j := 0; j < n; j++ {
				if err := pubPS.Publish(subj, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			if err := pubPS.Flush(); err != nil {
				publishErr.Store(err)
			}
		}(n)
	}
	memBefore := hotStart()
	hotStartT := time.Now()
	close(start)
	wg.Wait()
	pubDone := time.Now()
	if v := publishErr.Load(); v != nil {
		perr, _ := v.(error)
		return result{}, xerrors.Errorf("publish: %w", perr)
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for _, st := range subStates {
		select {
		case <-st.done:
		case <-deadline.C:
			var delivered int64
			for _, s := range subStates {
				delivered += s.count.Load()
			}
			return result{}, xerrors.Errorf("timeout: delivered %d of %d (subs=%d)", delivered, int64(msgs)*int64(subs), subs)
		}
	}
	subDone := time.Now()
	pubHot := pubDone.Sub(hotStartT)
	deliverHot := subDone.Sub(hotStartT)
	if subs == 0 {
		deliverHot = pubHot
	}
	rs := hotEnd(memBefore)

	var delivered int64
	for _, st := range subStates {
		delivered += st.count.Load()
		st.cancel()
	}
	return result{
		setup:      setup,
		pubHot:     pubHot,
		deliverHot: deliverHot,
		published:  int64(msgs),
		delivered:  delivered,
		subCount:   subs,
		rstats:     rs,
	}, nil
}

// runNativeClusterSymmetric is the symmetric variant of runNativeCluster:
// publishers and subscribers are distributed round-robin across all N
// replicas (no replica is reserved). -msgs is interpreted per-publisher,
// so the total messages flowing is msgs*pubs and every subscriber, on
// the one shared subject, expects msgs*pubs deliveries.
//
// MaxPending is bounded at 128 MiB (instead of the default 1 GiB) to
// cap worst-case in-flight bytes in cluster fan-out scenarios.
func runNativeClusterSymmetric(msgs, size, pubs, subs int, subj string, timeout time.Duration, replicas int) (result, error) {
	const symMaxPending int64 = 128 << 20
	t0 := time.Now()
	servers, err := startNativeCluster(replicas, symMaxPending)
	if err != nil {
		return result{}, xerrors.Errorf("start native cluster: %w", err)
	}
	defer func() {
		for _, ns := range servers {
			ns.Shutdown()
			ns.WaitForShutdown()
		}
	}()

	// Symmetric: every replica hosts both publishers and subscribers.
	replicaAt := func(i int) *natsserver.Server {
		return servers[i%replicas]
	}

	connect := func(ns *natsserver.Server, name string) (*natsgo.Conn, error) {
		return natsgo.Connect(ns.ClientURL(),
			natsgo.Name(name),
			natsgo.MaxReconnects(-1),
		)
	}

	expectPerSub := int64(msgs) * int64(pubs)
	totalPublished := int64(msgs) * int64(pubs)

	type subState struct {
		nc     *natsgo.Conn
		sub    *natsgo.Subscription
		count  atomic.Int64
		done   chan struct{}
		expect int64
	}
	subStates := make([]*subState, subs)
	for i := 0; i < subs; i++ {
		nc, cerr := connect(replicaAt(i), fmt.Sprintf("natsbench-sub-%d", i))
		if cerr != nil {
			return result{}, xerrors.Errorf("connect sub %d: %w", i, cerr)
		}
		st := &subState{nc: nc, done: make(chan struct{}), expect: expectPerSub}
		sub, serr := nc.Subscribe(subj, func(_ *natsgo.Msg) {
			n := st.count.Add(1)
			if n == st.expect {
				close(st.done)
			}
		})
		if serr != nil {
			return result{}, xerrors.Errorf("subscribe sub %d: %w", i, serr)
		}
		if err := sub.SetPendingLimits(-1, -1); err != nil {
			return result{}, xerrors.Errorf("pending limits sub %d: %w", i, err)
		}
		if err := nc.Flush(); err != nil {
			return result{}, xerrors.Errorf("flush sub %d: %w", i, err)
		}
		st.sub = sub
		subStates[i] = st
	}

	pubConns := make([]*natsgo.Conn, pubs)
	for i := 0; i < pubs; i++ {
		nc, cerr := connect(replicaAt(i), fmt.Sprintf("natsbench-pub-%d", i))
		if cerr != nil {
			return result{}, xerrors.Errorf("connect pub %d: %w", i, cerr)
		}
		pubConns[i] = nc
	}
	setup := time.Since(t0)

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	// Symmetric mode: each publisher publishes the full msgs count.
	var wg sync.WaitGroup
	var publishErr atomic.Value
	start := make(chan struct{})
	for i := 0; i < pubs; i++ {
		nc := pubConns[i]
		wg.Add(1)
		go func(nc *natsgo.Conn) {
			defer wg.Done()
			<-start
			for j := 0; j < msgs; j++ {
				if err := nc.Publish(subj, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			if err := nc.Flush(); err != nil {
				publishErr.Store(err)
			}
		}(nc)
	}
	memBefore := hotStart()
	hotStartT := time.Now()
	close(start)
	wg.Wait()
	pubDone := time.Now()
	if v := publishErr.Load(); v != nil {
		perr, _ := v.(error)
		return result{}, xerrors.Errorf("publish: %w", perr)
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for _, st := range subStates {
		select {
		case <-st.done:
		case <-deadline.C:
			var delivered int64
			for _, s := range subStates {
				delivered += s.count.Load()
			}
			return result{}, xerrors.Errorf("timeout: delivered %d of %d (subs=%d)", delivered, expectPerSub*int64(subs), subs)
		}
	}
	subDone := time.Now()
	pubHot := pubDone.Sub(hotStartT)
	deliverHot := subDone.Sub(hotStartT)
	if subs == 0 {
		deliverHot = pubHot
	}
	rs := hotEnd(memBefore)

	var delivered int64
	for _, st := range subStates {
		delivered += st.count.Load()
		st.nc.Close()
	}
	for _, nc := range pubConns {
		nc.Close()
	}
	return result{
		setup:      setup,
		pubHot:     pubHot,
		deliverHot: deliverHot,
		published:  totalPublished,
		delivered:  delivered,
		subCount:   subs,
		rstats:     rs,
		symmetric:  true,
	}, nil
}

// runCoderClusterSymmetric is the symmetric variant of runCoderCluster:
// publishers and subscribers are distributed round-robin across all N
// replicas. -msgs is interpreted per-publisher (total = msgs*pubs);
// every subscriber sees every message on the shared subject.
//
// MaxPending is bounded at 128 MiB (instead of the default 1 GiB) to
// cap worst-case in-flight bytes in cluster fan-out scenarios.
func runCoderClusterSymmetric(msgs, size, pubs, subs int, subj string, timeout time.Duration, replicas int) (result, error) {
	const symMaxPending int64 = 128 << 20
	t0 := time.Now()
	logger := slog.Make() // discard
	pubsubs, err := startCoderCluster(context.Background(), logger, replicas, symMaxPending)
	if err != nil {
		return result{}, xerrors.Errorf("start coder cluster: %w", err)
	}
	defer func() {
		for _, p := range pubsubs {
			_ = p.Close()
		}
	}()

	replicaAt := func(i int) *codernats.Pubsub {
		return pubsubs[i%replicas]
	}

	expectPerSub := int64(msgs) * int64(pubs)
	totalPublished := int64(msgs) * int64(pubs)

	type subState struct {
		count  atomic.Int64
		done   chan struct{}
		expect int64
		cancel func()
	}
	subStates := make([]*subState, subs)
	for i := 0; i < subs; i++ {
		st := &subState{done: make(chan struct{}), expect: expectPerSub}
		cancel, serr := replicaAt(i).Subscribe(subj, func(_ context.Context, _ []byte) {
			n := st.count.Add(1)
			if n == st.expect {
				close(st.done)
			}
		})
		if serr != nil {
			return result{}, xerrors.Errorf("subscribe %d: %w", i, serr)
		}
		st.cancel = cancel
		subStates[i] = st
	}
	setup := time.Since(t0)

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	// Symmetric mode: each publisher publishes the full msgs count on
	// its assigned replica.
	var wg sync.WaitGroup
	var publishErr atomic.Value
	start := make(chan struct{})
	for i := 0; i < pubs; i++ {
		ps := replicaAt(i)
		wg.Add(1)
		go func(ps *codernats.Pubsub) {
			defer wg.Done()
			<-start
			for j := 0; j < msgs; j++ {
				if err := ps.Publish(subj, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			if err := ps.Flush(); err != nil {
				publishErr.Store(err)
			}
		}(ps)
	}
	memBefore := hotStart()
	hotStartT := time.Now()
	close(start)
	wg.Wait()
	pubDone := time.Now()
	if v := publishErr.Load(); v != nil {
		perr, _ := v.(error)
		return result{}, xerrors.Errorf("publish: %w", perr)
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for _, st := range subStates {
		select {
		case <-st.done:
		case <-deadline.C:
			var delivered int64
			for _, s := range subStates {
				delivered += s.count.Load()
			}
			return result{}, xerrors.Errorf("timeout: delivered %d of %d (subs=%d)", delivered, expectPerSub*int64(subs), subs)
		}
	}
	subDone := time.Now()
	pubHot := pubDone.Sub(hotStartT)
	deliverHot := subDone.Sub(hotStartT)
	if subs == 0 {
		deliverHot = pubHot
	}
	rs := hotEnd(memBefore)

	var delivered int64
	for _, st := range subStates {
		delivered += st.count.Load()
		st.cancel()
	}
	return result{
		setup:      setup,
		pubHot:     pubHot,
		deliverHot: deliverHot,
		published:  totalPublished,
		delivered:  delivered,
		subCount:   subs,
		rstats:     rs,
		symmetric:  true,
	}, nil
}

// humanCount renders a count rate with thousands separators (best-effort).
func humanCount(v float64) string {
	return commas(int64(v))
}

func commas(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	// Insert commas every three digits from the right.
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func humanBytes(bps float64) string {
	const (
		kib = 1024.0
		mib = 1024.0 * kib
		gib = 1024.0 * mib
		tib = 1024.0 * gib
	)
	switch {
	case bps >= tib:
		return fmt.Sprintf("%.2f TiB", bps/tib)
	case bps >= gib:
		return fmt.Sprintf("%.2f GiB", bps/gib)
	case bps >= mib:
		return fmt.Sprintf("%.2f MiB", bps/mib)
	case bps >= kib:
		return fmt.Sprintf("%.2f KiB", bps/kib)
	default:
		return fmt.Sprintf("%.0f B", bps)
	}
}
