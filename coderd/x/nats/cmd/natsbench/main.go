// Command natsbench is a standalone benchmark harness modeled after
// upstream `nats bench`. It measures publish/deliver throughput for
// several transports: raw TCP loopback, raw net.Pipe, embedded NATS
// (TCP or in-process), and the coderd/x/nats Pubsub wrapper (TCP or
// in-process).
//
// Subject distribution: when -subjects=1 (default) every publisher and
// subscriber share one subject and behavior matches the legacy
// single-subject mode. With -subjects=N>1, N subjects are generated
// per mode (native: "<-subj>.0"..."<-subj>.N-1"; coder: "bench_0"..."
// bench_N-1" to satisfy legacy-event token rules). Publisher i and
// subscriber j are pinned to subject (i%N) and (j%N) respectively; a
// subscriber only receives messages from publishers assigned to its
// subject. Subject distribution is encapsulated by planSubjects so
// every mode shares the same shape and Coder/native results stay
// comparable. Coder modes pin PublishConns/SubscribeConns to 3.
//
// Total publish work is -msgs messages split across -pubs publishers
// (split as evenly as possible with any remainder dumped on publisher
// 0), except for the *-cluster-symmetric modes where -msgs is the
// per-publisher count and total = msgs*pubs. Wall-clock is measured
// around the hot loop only.
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
	subj := flag.String("subj", "bench", "subject prefix (NATS modes). With -subjects=1 this is used as the subject as-is; with -subjects>1 native modes append \".<idx>\" and coder modes append \"_<idx>\". For coder modes the prefix must be a valid legacy event token ([A-Za-z0-9_-]+).")
	subjects := flag.Int("subjects", 1, "number of subjects publishers/subscribers are distributed across (round-robin). 1 preserves legacy single-subject behavior.")
	timeout := flag.Duration("timeout", 5*time.Minute, "per-phase timeout. Applied independently to the publish phase (wg.Wait + pool flush), the delivery wait, and the bounded cleanup phase. Setup uses its own embedded-server timeouts; warmup uses a derived sub-budget. Zero means \"wait forever\" for the delivery phase only.")
	replicas := flag.Int("replicas", 10, "number of replicas for *-cluster modes (ignored elsewhere)")
	localQueueMsgs := flag.Int("local-queue-msgs", 0, "override the per-listener inbox channel capacity for Coder modes. 0 means derive from the benchmark plan (benchmarkPendingMsgs). Values are clamped to [benchmarkPendingMsgsFloor, benchmarkPendingMsgsCap] so an operator typo cannot allocate an unbounded local listener channel buffer.")
	cpuProfile := flag.String("cpuprofile", "", "write a CPU profile of the hot phase to this path")
	memProfile := flag.String("memprofile", "", "write a heap profile of live memory after the hot phase to this path")
	writeBuffer := flag.Int("write-buffer", 0, "NATS Go client write buffer size in bytes for every wrapper-owned or natsbench-owned client connection. 0 keeps the nats.go default (32 KiB). Applies to both Coder modes (via codernats.Options.WriteBufferSize) and native modes (via natsgo.WriteBufferSize on every raw nats.go client).")
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
	if *subjects < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "natsbench: -subjects must be >= 1")
		os.Exit(2)
	}
	if *writeBuffer < 0 {
		_, _ = fmt.Fprintln(os.Stderr, "natsbench: -write-buffer must be >= 0")
		os.Exit(2)
	}
	if *localQueueMsgs < 0 {
		_, _ = fmt.Fprintln(os.Stderr, "natsbench: -local-queue-msgs must be >= 0")
		os.Exit(2)
	}

	if err := run(*mode, *msgs, *size, *pubs, *subs, *subj, *subjects, *timeout, *replicas, *writeBuffer, *localQueueMsgs); err != nil {
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
	// drops counts pubsub.ErrDroppedMessages signals observed by
	// SubscribeWithErr listeners across all subscribers, summed for
	// the run. Native and loopback modes leave this at zero. Coder
	// modes populate it even on a successful run so a benchmark
	// report always shows whether the local listener queue overflowed.
	drops    int64
	subCount int // number of subscribers in the run
	rstats   runtimeStats
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

func run(mode string, msgs, size, pubs, subs int, subj string, subjects int, timeout time.Duration, replicas, writeBuffer, localQueueMsgs int) error {
	// writeBufferSuffix builds a " write-buffer=N" tail for the header
	// line so each run is self-describing. Empty when zero (i.e. the
	// nats.go default is in effect) to keep legacy runs visually
	// identical to pre-flag output.
	wbSuffix := writeBufferHeader(writeBuffer)
	switch mode {
	case "loopback-tcp", "loopback-pipe":
		// Loopback modes ignore -pubs/-subs/-subj/-subjects and
		// -write-buffer: they're a raw kernel/net.Pipe byte stream
		// with no nats.go client involved.
		_, _ = fmt.Printf("mode=%s msgs=%d size=%d\n", mode, msgs, size)
		res, err := runLoopback(mode, msgs, size)
		if err != nil {
			return err
		}
		printResult(mode, res, msgs, size, 1, 1, 1)
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
			_, _ = fmt.Printf("mode=%s pubs=%d subs=%d msgs=%d size=%d subjects=%d replicas=%d%s (msgs/pub)\n", mode, pubs, subs, msgs, size, subjects, replicas, wbSuffix)
		} else {
			_, _ = fmt.Printf("mode=%s pubs=%d subs=%d msgs=%d size=%d subjects=%d replicas=%d%s\n", mode, pubs, subs, msgs, size, subjects, replicas, wbSuffix)
		}
	} else {
		_, _ = fmt.Printf("mode=%s pubs=%d subs=%d msgs=%d size=%d subjects=%d%s\n", mode, pubs, subs, msgs, size, subjects, wbSuffix)
	}
	var (
		res result
		err error
	)
	switch mode {
	case "native-tcp":
		res, err = runNative(false, msgs, size, pubs, subs, subj, subjects, timeout, writeBuffer)
	case "native-inproc":
		res, err = runNative(true, msgs, size, pubs, subs, subj, subjects, timeout, writeBuffer)
	case "coder-tcp":
		res, err = runCoder(false, msgs, size, pubs, subs, subj, subjects, timeout, writeBuffer, localQueueMsgs)
	case "coder-inproc":
		res, err = runCoder(true, msgs, size, pubs, subs, subj, subjects, timeout, writeBuffer, localQueueMsgs)
	case "native-cluster":
		res, err = runNativeCluster(msgs, size, pubs, subs, subj, subjects, timeout, replicas, writeBuffer)
	case "coder-cluster":
		res, err = runCoderCluster(msgs, size, pubs, subs, subj, subjects, timeout, replicas, writeBuffer, localQueueMsgs)
	case "native-cluster-symmetric":
		res, err = runNativeClusterSymmetric(msgs, size, pubs, subs, subj, subjects, timeout, replicas, writeBuffer)
	case "coder-cluster-symmetric":
		res, err = runCoderClusterSymmetric(msgs, size, pubs, subs, subj, subjects, timeout, replicas, writeBuffer, localQueueMsgs)
	default:
		return xerrors.Errorf("unknown mode %q", mode)
	}
	if err != nil {
		return err
	}
	printResult(mode, res, msgs, size, pubs, subs, subjects)
	return nil
}

// writeBufferHeader renders a " write-buffer=N" suffix for the run
// header line, or the empty string when writeBuffer == 0 so legacy
// runs that don't pass the flag print the same header they always
// have.
func writeBufferHeader(writeBuffer int) string {
	if writeBuffer == 0 {
		return ""
	}
	return fmt.Sprintf(" write-buffer=%d", writeBuffer)
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
func runNative(inProcess bool, msgs, size, pubs, subs int, subj string, numSubjects int, timeout time.Duration, writeBuffer int) (result, error) {
	plan, err := planSubjects(pubs, subs, numSubjects, msgs, false)
	if err != nil {
		return result{}, xerrors.Errorf("plan subjects: %w", err)
	}
	subjectNames := buildNativeSubjects(subj, numSubjects)
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
		if writeBuffer > 0 {
			opts = append(opts, natsgo.WriteBufferSize(writeBuffer))
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
		st := &subState{nc: nc, done: make(chan struct{}), expect: plan.ExpectPerSub[i]}
		// If a subscriber's subject has zero publishers, it expects
		// zero messages and is considered done immediately.
		if st.expect == 0 {
			close(st.done)
		}
		sub, serr := nc.Subscribe(subjectNames[plan.SubSubject[i]], func(_ *natsgo.Msg) {
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

	var wg sync.WaitGroup
	var publishErr atomic.Value // error
	start := make(chan struct{})
	for i := 0; i < pubs; i++ {
		n := plan.PerPubMsgs[i]
		nc := pubConns[i]
		pubSubject := subjectNames[plan.PubSubject[i]]
		wg.Add(1)
		go func(nc *natsgo.Conn, n int, subject string) {
			defer wg.Done()
			<-start
			for j := 0; j < n; j++ {
				if err := nc.Publish(subject, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			if err := nc.Flush(); err != nil {
				publishErr.Store(err)
			}
		}(nc, n, pubSubject)
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
			var expected int64
			for _, s := range subStates {
				delivered += s.count.Load()
				expected += s.expect
			}
			return result{}, xerrors.Errorf("timeout: delivered %d of %d (subs=%d)", delivered, expected, subs)
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
		published:  plan.TotalPublished,
		delivered:  delivered,
		subCount:   subs,
		rstats:     rs,
	}, nil
}

// runCoder runs the bench against the coderd/x/nats Pubsub wrapper.
// One Pubsub instance is shared by all publishers and subscribers
// (the wrapper multiplexes via its dual-conn design). PublishConns and
// SubscribeConns are pinned to benchmarkPublishConns/SubscribeConns so
// every Coder mode run is directly comparable.
//
//nolint:revive // inProcess is a transport selector, not a control flag.
func runCoder(inProcess bool, msgs, size, pubs, subs int, subj string, numSubjects int, timeout time.Duration, writeBuffer, localQueueMsgs int) (result, error) {
	plan, err := planSubjects(pubs, subs, numSubjects, msgs, false)
	if err != nil {
		return result{}, xerrors.Errorf("plan subjects: %w", err)
	}
	subjectNames := buildCoderSubjects(subj, numSubjects)
	pendingMsgs, clamped, err := localQueueCapacity(plan, localQueueMsgs)
	if err != nil {
		return result{}, err
	}
	_, _ = fmt.Println(localQueueDescription(pendingMsgs, subs, localQueueMsgs, clamped))
	t0 := time.Now()
	logger := slog.Make() // discard
	ps, err := codernats.New(context.Background(), logger, codernats.Options{
		InProcess: inProcess,
		// Benchmark-only sizing: PendingLimits.Msgs sets BOTH the
		// per-subscription NATS pending cap and the per-listener
		// inbox capacity (see codernats.listenerQueueSize). After
		// same-subject coalescing the default 1024 local inbox is
		// the first thing to overflow in exact-delivery throughput
		// runs; size it from the plan so legitimate burst traffic
		// is absorbed and drops genuinely indicate runtime backpressure.
		PendingLimits: codernats.PendingLimits{
			Msgs:  pendingMsgs,
			Bytes: -1,
		},
		PublishConns:    benchmarkPublishConns,
		SubscribeConns:  benchmarkSubscribeConns,
		WriteBufferSize: writeBuffer,
		DrainTimeout:    benchmarkDrainTimeout(timeout),
	})
	if err != nil {
		return result{}, xerrors.Errorf("new pubsub: %w", err)
	}
	// Bounded cleanup: ensures Close cannot silently hang AFTER a
	// successful hot phase. We run it deferred so it also fires on
	// early errors. See runBoundedCleanup for the contract.
	defer func() {
		if cerr := runBoundedCleanup("ps.Close", cleanupTimeout(timeout), ps.Close); cerr != nil {
			reportCleanupErr("ps.Close", cerr)
		}
	}()

	subStates := make([]*coderSubState, subs)
	var firstSubErr atomic.Value // error
	for i := 0; i < subs; i++ {
		st := newCoderSubState(plan.ExpectPerSub[i])
		cancel, serr := ps.SubscribeWithErr(subjectNames[plan.SubSubject[i]], coderSubCallback(st, &firstSubErr))
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

	var wg sync.WaitGroup
	var publishErr atomic.Value
	start := make(chan struct{})
	for i := 0; i < pubs; i++ {
		n := plan.PerPubMsgs[i]
		pubSubject := subjectNames[plan.PubSubject[i]]
		wg.Add(1)
		go func(n int, subject string) {
			defer wg.Done()
			<-start
			for j := 0; j < n; j++ {
				if err := ps.Publish(subject, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			// Per publisher Flush removed: Pubsub.Flush flushes the
			// whole publisher pool, so calling it from every
			// publisher goroutine made one slow pub conn block
			// every other publisher. We now flush the pool once
			// from the main goroutine after wg.Wait.
		}(n, pubSubject)
	}
	memBefore := hotStart()
	hotStartT := time.Now()
	close(start)
	publishDiag := func() string {
		return publishPhaseDiagFromCoderStates(subStates, plan.TotalPublished, &publishErr, &firstSubErr).String()
	}
	if perr := awaitWaitGroup("publish", timeout, &wg, publishDiag); perr != nil {
		return result{}, wrapPhaseError("publish wg.Wait", perr)
	}
	if v := publishErr.Load(); v != nil {
		perr, _ := v.(error)
		return result{}, xerrors.Errorf("publish: %w", perr)
	}
	// Single pool flush under the publish-phase timeout. Pubsub.Flush
	// already iterates every pubConn internally.
	if ferr := runBoundedCleanup("publish-flush", timeout, ps.Flush); ferr != nil {
		return result{}, wrapPhaseError("publish-flush", ferr)
	}
	pubDone := time.Now()

	if derr := awaitCoderDeliveryDone("delivery", timeout, subStates, &firstSubErr); derr != nil {
		return result{}, derr
	}
	subDone := time.Now()
	pubHot := pubDone.Sub(hotStartT)
	deliverHot := subDone.Sub(hotStartT)
	if subs == 0 {
		deliverHot = pubHot
	}
	rs := hotEnd(memBefore)

	var delivered, drops int64
	for _, st := range subStates {
		delivered += st.count.Load()
		drops += st.drops.Load()
		st.cancel()
	}
	if v := firstSubErr.Load(); v != nil {
		if ferr, _ := v.(error); ferr != nil {
			return result{}, xerrors.Errorf("subscriber error: %w", ferr)
		}
	}
	return result{
		setup:      setup,
		pubHot:     pubHot,
		deliverHot: deliverHot,
		published:  plan.TotalPublished,
		delivered:  delivered,
		drops:      drops,
		subCount:   subs,
		rstats:     rs,
	}, nil
}

func printResult(mode string, r result, msgs, size, pubs, subs, subjects int) {
	_ = mode
	_, _ = fmt.Printf("setup: %s\n", r.setup)
	_, _ = fmt.Printf("publish duration: %s\n", r.pubHot)
	if subs > 0 {
		_, _ = fmt.Printf("end-to-end deliver duration: %s\n", r.deliverHot)
	}
	_, _ = fmt.Printf("total msgs published: %d (subjects=%d)\n", r.published, subjects)
	if subs > 0 {
		// In symmetric cluster modes -msgs is per-publisher (total
		// = msgs*pubs); otherwise -msgs is the total publish budget.
		// With multiple subjects, publishers and subscribers are
		// pinned round-robin to subjects, so each subscriber sees
		// only the publishes targeted at its subject. When pubs and
		// subs are both multiples of subjects (the natural shape)
		// every subject has the same number of publishers and the
		// expected per-subscriber count is r.published / subjects.
		// Otherwise per-subscriber counts vary; we report the
		// aggregated total as authoritative and the "expected per
		// subscriber" line as a best-effort even-split summary.
		totalPub := msgs
		if r.symmetric {
			totalPub = msgs * pubs
		}
		perSub := totalPub
		if subjects > 1 {
			perSub = totalPub / subjects
		}
		_, _ = fmt.Printf("total msgs delivered: %d (%d subs, ~%d msgs/sub)\n", r.delivered, r.subCount, perSub)
		// Always print drop-signal accounting (zero or nonzero) for
		// modes that exercise SubscribeWithErr so users can confirm
		// at a glance that the run was not silently shedding messages
		// to a bounded local listener queue. Native/loopback modes
		// leave drops at zero.
		_, _ = fmt.Printf("drop signals: %d\n", r.drops)
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
func runNativeCluster(msgs, size, pubs, subs int, subj string, numSubjects int, timeout time.Duration, replicas, writeBuffer int) (result, error) {
	plan, err := planSubjects(pubs, subs, numSubjects, msgs, false)
	if err != nil {
		return result{}, xerrors.Errorf("plan subjects: %w", err)
	}
	subjectNames := buildNativeSubjects(subj, numSubjects)
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
		opts := []natsgo.Option{
			natsgo.Name(name),
			natsgo.MaxReconnects(-1),
		}
		if writeBuffer > 0 {
			opts = append(opts, natsgo.WriteBufferSize(writeBuffer))
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
		nc, cerr := connect(subReplicaAt(i), fmt.Sprintf("natsbench-sub-%d", i))
		if cerr != nil {
			return result{}, xerrors.Errorf("connect sub %d: %w", i, cerr)
		}
		st := &subState{nc: nc, done: make(chan struct{}), expect: plan.ExpectPerSub[i]}
		if st.expect == 0 {
			close(st.done)
		}
		sub, serr := nc.Subscribe(subjectNames[plan.SubSubject[i]], func(_ *natsgo.Msg) {
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

	var wg sync.WaitGroup
	var publishErr atomic.Value
	start := make(chan struct{})
	for i := 0; i < pubs; i++ {
		n := plan.PerPubMsgs[i]
		nc := pubConns[i]
		pubSubject := subjectNames[plan.PubSubject[i]]
		wg.Add(1)
		go func(nc *natsgo.Conn, n int, subject string) {
			defer wg.Done()
			<-start
			for j := 0; j < n; j++ {
				if err := nc.Publish(subject, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			if err := nc.Flush(); err != nil {
				publishErr.Store(err)
			}
		}(nc, n, pubSubject)
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
			var expected int64
			for _, s := range subStates {
				delivered += s.count.Load()
				expected += s.expect
			}
			return result{}, xerrors.Errorf("timeout: delivered %d of %d (subs=%d)", delivered, expected, subs)
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
		published:  plan.TotalPublished,
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
//
// Before the timed hot phase begins, this runner runs a warmup phase
// on the real benchmark subjects so that subscribers across the
// cluster have proven cross-route interest from replica 0. Warmup
// payloads are tagged with warmupSentinel and do NOT count toward the
// measured delivery totals (see coderSubCallback). The warmup phase
// is bounded by warmupTimeout; if it times out the hot phase still
// runs and the normal timeout path surfaces any delivery shortfall.
func runCoderCluster(msgs, size, pubs, subs int, subj string, numSubjects int, timeout time.Duration, replicas, writeBuffer, localQueueMsgs int) (result, error) {
	plan, err := planSubjects(pubs, subs, numSubjects, msgs, false)
	if err != nil {
		return result{}, xerrors.Errorf("plan subjects: %w", err)
	}
	subjectNames := buildCoderSubjects(subj, numSubjects)
	pendingMsgs, clamped, err := localQueueCapacity(plan, localQueueMsgs)
	if err != nil {
		return result{}, err
	}
	_, _ = fmt.Println(localQueueDescription(pendingMsgs, subs, localQueueMsgs, clamped))
	t0 := time.Now()
	logger := slog.Make() // discard
	pubsubs, err := startCoderCluster(context.Background(), logger, replicas, 0, benchmarkPublishConns, benchmarkSubscribeConns, pendingMsgs, writeBuffer)
	if err != nil {
		return result{}, xerrors.Errorf("start coder cluster: %w", err)
	}
	defer func() {
		if cerr := runBoundedCleanup("coder-cluster.Close", cleanupTimeout(timeout), func() error {
			return closeCoderClusterConcurrent(pubsubs)
		}); cerr != nil {
			reportCleanupErr("coder-cluster.Close", cerr)
		}
	}()

	pubPS := pubsubs[0]
	subPSAt := func(i int) *codernats.Pubsub {
		if replicas <= 1 {
			return pubsubs[0]
		}
		return pubsubs[1+(i%(replicas-1))]
	}

	subStates := make([]*coderSubState, subs)
	var firstSubErr atomic.Value // error
	for i := 0; i < subs; i++ {
		st := newClusterCoderSubState(plan.ExpectPerSub[i])
		cancel, serr := subPSAt(i).SubscribeWithErr(subjectNames[plan.SubSubject[i]], coderSubCallback(st, &firstSubErr))
		if serr != nil {
			return result{}, xerrors.Errorf("subscribe %d: %w", i, serr)
		}
		st.cancel = cancel
		subStates[i] = st
	}
	setup := time.Since(t0)

	// Warmup phase: replica 0 publishes a tagged warmup payload on
	// every actual benchmark subject so cross-route interest is
	// established before counters start. All publishers in this mode
	// are on replica 0, so the per-subject pubReplicas mask is just {0}.
	pubReplicaOf := func(int) int { return 0 }
	if err := runCoderClusterWarmup(subjectNames, plan, subStates, pubsubs, pubReplicaOf, timeout); err != nil {
		return result{}, wrapPhaseError("warmup", err)
	}

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	var wg sync.WaitGroup
	var publishErr atomic.Value
	start := make(chan struct{})
	for i := 0; i < pubs; i++ {
		n := plan.PerPubMsgs[i]
		pubSubject := subjectNames[plan.PubSubject[i]]
		wg.Add(1)
		go func(n int, subject string) {
			defer wg.Done()
			<-start
			for j := 0; j < n; j++ {
				if err := pubPS.Publish(subject, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			// Per publisher Flush removed: see runCoder for rationale.
		}(n, pubSubject)
	}
	memBefore := hotStart()
	hotStartT := time.Now()
	close(start)
	publishDiag := func() string {
		return publishPhaseDiagFromCoderStates(subStates, plan.TotalPublished, &publishErr, &firstSubErr).String()
	}
	if perr := awaitWaitGroup("publish", timeout, &wg, publishDiag); perr != nil {
		return result{}, wrapPhaseError("publish wg.Wait", perr)
	}
	if v := publishErr.Load(); v != nil {
		perr, _ := v.(error)
		return result{}, xerrors.Errorf("publish: %w", perr)
	}
	// Single pool flush; pubPS.Flush iterates every pubConn internally.
	if ferr := runBoundedCleanup("publish-flush", timeout, pubPS.Flush); ferr != nil {
		return result{}, wrapPhaseError("publish-flush", ferr)
	}
	pubDone := time.Now()

	if derr := awaitCoderDeliveryDone("delivery", timeout, subStates, &firstSubErr); derr != nil {
		return result{}, derr
	}
	subDone := time.Now()
	pubHot := pubDone.Sub(hotStartT)
	deliverHot := subDone.Sub(hotStartT)
	if subs == 0 {
		deliverHot = pubHot
	}
	rs := hotEnd(memBefore)

	var delivered, drops int64
	for _, st := range subStates {
		delivered += st.count.Load()
		drops += st.drops.Load()
		st.cancel()
	}
	if v := firstSubErr.Load(); v != nil {
		if ferr, _ := v.(error); ferr != nil {
			return result{}, xerrors.Errorf("subscriber error: %w", ferr)
		}
	}
	return result{
		setup:      setup,
		pubHot:     pubHot,
		deliverHot: deliverHot,
		published:  plan.TotalPublished,
		delivered:  delivered,
		drops:      drops,
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
func runNativeClusterSymmetric(msgs, size, pubs, subs int, subj string, numSubjects int, timeout time.Duration, replicas, writeBuffer int) (result, error) {
	const symMaxPending int64 = 128 << 20
	plan, err := planSubjects(pubs, subs, numSubjects, msgs, true)
	if err != nil {
		return result{}, xerrors.Errorf("plan subjects: %w", err)
	}
	subjectNames := buildNativeSubjects(subj, numSubjects)
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
		opts := []natsgo.Option{
			natsgo.Name(name),
			natsgo.MaxReconnects(-1),
		}
		if writeBuffer > 0 {
			opts = append(opts, natsgo.WriteBufferSize(writeBuffer))
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
		nc, cerr := connect(replicaAt(i), fmt.Sprintf("natsbench-sub-%d", i))
		if cerr != nil {
			return result{}, xerrors.Errorf("connect sub %d: %w", i, cerr)
		}
		st := &subState{nc: nc, done: make(chan struct{}), expect: plan.ExpectPerSub[i]}
		if st.expect == 0 {
			close(st.done)
		}
		sub, serr := nc.Subscribe(subjectNames[plan.SubSubject[i]], func(_ *natsgo.Msg) {
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

	// Symmetric mode: each publisher publishes the full msgs count on
	// its assigned subject.
	var wg sync.WaitGroup
	var publishErr atomic.Value
	start := make(chan struct{})
	for i := 0; i < pubs; i++ {
		nc := pubConns[i]
		n := plan.PerPubMsgs[i]
		pubSubject := subjectNames[plan.PubSubject[i]]
		wg.Add(1)
		go func(nc *natsgo.Conn, n int, subject string) {
			defer wg.Done()
			<-start
			for j := 0; j < n; j++ {
				if err := nc.Publish(subject, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			if err := nc.Flush(); err != nil {
				publishErr.Store(err)
			}
		}(nc, n, pubSubject)
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
			var expected int64
			for _, s := range subStates {
				delivered += s.count.Load()
				expected += s.expect
			}
			return result{}, xerrors.Errorf("timeout: delivered %d of %d (subs=%d)", delivered, expected, subs)
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
		published:  plan.TotalPublished,
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
//
// Like runCoderCluster, this runner warms up cross-route interest on
// the real benchmark subjects before the timed hot phase. Because
// publishers are spread across replicas in symmetric mode, the warmup
// loop tracks per-replica interest using a uint64 bitmask
// (warmupReplicaCap caps the trackable replica count at 64; runs above
// the cap fall back to "any one warmup observed" semantics).
func runCoderClusterSymmetric(msgs, size, pubs, subs int, subj string, numSubjects int, timeout time.Duration, replicas, writeBuffer, localQueueMsgs int) (result, error) {
	const symMaxPending int64 = 128 << 20
	plan, err := planSubjects(pubs, subs, numSubjects, msgs, true)
	if err != nil {
		return result{}, xerrors.Errorf("plan subjects: %w", err)
	}
	subjectNames := buildCoderSubjects(subj, numSubjects)
	pendingMsgs, clamped, err := localQueueCapacity(plan, localQueueMsgs)
	if err != nil {
		return result{}, err
	}
	_, _ = fmt.Println(localQueueDescription(pendingMsgs, subs, localQueueMsgs, clamped))
	t0 := time.Now()
	logger := slog.Make() // discard
	pubsubs, err := startCoderCluster(context.Background(), logger, replicas, symMaxPending, benchmarkPublishConns, benchmarkSubscribeConns, pendingMsgs, writeBuffer)
	if err != nil {
		return result{}, xerrors.Errorf("start coder cluster: %w", err)
	}
	defer func() {
		if cerr := runBoundedCleanup("coder-cluster.Close", cleanupTimeout(timeout), func() error {
			return closeCoderClusterConcurrent(pubsubs)
		}); cerr != nil {
			reportCleanupErr("coder-cluster.Close", cerr)
		}
	}()

	replicaAt := func(i int) *codernats.Pubsub {
		return pubsubs[i%replicas]
	}

	subStates := make([]*coderSubState, subs)
	var firstSubErr atomic.Value // error
	for i := 0; i < subs; i++ {
		st := newClusterCoderSubState(plan.ExpectPerSub[i])
		cancel, serr := replicaAt(i).SubscribeWithErr(subjectNames[plan.SubSubject[i]], coderSubCallback(st, &firstSubErr))
		if serr != nil {
			return result{}, xerrors.Errorf("subscribe %d: %w", i, serr)
		}
		st.cancel = cancel
		subStates[i] = st
	}
	setup := time.Since(t0)

	pubReplicaOf := func(i int) int { return i % replicas }
	if err := runCoderClusterWarmup(subjectNames, plan, subStates, pubsubs, pubReplicaOf, timeout); err != nil {
		return result{}, wrapPhaseError("warmup", err)
	}

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	// Symmetric mode: each publisher publishes the full msgs count on
	// its assigned replica and its assigned subject.
	var wg sync.WaitGroup
	var publishErr atomic.Value
	start := make(chan struct{})
	usedPubsubs := make(map[*codernats.Pubsub]struct{}, replicas)
	for i := 0; i < pubs; i++ {
		ps := replicaAt(i)
		usedPubsubs[ps] = struct{}{}
		n := plan.PerPubMsgs[i]
		pubSubject := subjectNames[plan.PubSubject[i]]
		wg.Add(1)
		go func(ps *codernats.Pubsub, n int, subject string) {
			defer wg.Done()
			<-start
			for j := 0; j < n; j++ {
				if err := ps.Publish(subject, payload); err != nil {
					publishErr.Store(err)
					return
				}
			}
			// Per publisher Flush removed: see runCoder for rationale.
		}(ps, n, pubSubject)
	}
	memBefore := hotStart()
	hotStartT := time.Now()
	close(start)
	publishDiag := func() string {
		return publishPhaseDiagFromCoderStates(subStates, plan.TotalPublished, &publishErr, &firstSubErr).String()
	}
	if perr := awaitWaitGroup("publish", timeout, &wg, publishDiag); perr != nil {
		return result{}, wrapPhaseError("publish wg.Wait", perr)
	}
	if v := publishErr.Load(); v != nil {
		perr, _ := v.(error)
		return result{}, xerrors.Errorf("publish: %w", perr)
	}
	// Flush each used Pubsub once (each is a pool). This replaces the
	// per-goroutine flush that used to redundantly flush every replica's
	// entire pool from every publisher goroutine.
	if ferr := runBoundedCleanup("publish-flush", timeout, func() error {
		return flushPubsubsConcurrent(usedPubsubs)
	}); ferr != nil {
		return result{}, wrapPhaseError("publish-flush", ferr)
	}
	pubDone := time.Now()

	if derr := awaitCoderDeliveryDone("delivery", timeout, subStates, &firstSubErr); derr != nil {
		return result{}, derr
	}
	subDone := time.Now()
	pubHot := pubDone.Sub(hotStartT)
	deliverHot := subDone.Sub(hotStartT)
	if subs == 0 {
		deliverHot = pubHot
	}
	rs := hotEnd(memBefore)

	var delivered, drops int64
	for _, st := range subStates {
		delivered += st.count.Load()
		drops += st.drops.Load()
		st.cancel()
	}
	if v := firstSubErr.Load(); v != nil {
		if ferr, _ := v.(error); ferr != nil {
			return result{}, xerrors.Errorf("subscriber error: %w", ferr)
		}
	}
	return result{
		setup:      setup,
		pubHot:     pubHot,
		deliverHot: deliverHot,
		published:  plan.TotalPublished,
		delivered:  delivered,
		drops:      drops,
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
