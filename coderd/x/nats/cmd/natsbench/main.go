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
	mode := flag.String("mode", "", "one of: loopback-tcp, loopback-pipe, native-tcp, native-inproc, coder-tcp, coder-inproc")
	msgs := flag.Int("msgs", 1_000_000, "total messages to publish (shared across publishers)")
	size := flag.Int("size", 128, "payload size in bytes")
	pubs := flag.Int("pubs", 1, "number of publisher goroutines")
	subs := flag.Int("subs", 1, "number of subscriber goroutines")
	subj := flag.String("subj", "bench", "subject (NATS modes only)")
	timeout := flag.Duration("timeout", 5*time.Minute, "max wait for subscribers to drain")
	flag.Parse()

	if *mode == "" {
		_, _ = fmt.Fprintln(os.Stderr, "natsbench: -mode is required")
		flag.Usage()
		os.Exit(2)
	}
	if *msgs <= 0 || *size <= 0 || *pubs <= 0 || *subs < 0 {
		_, _ = fmt.Fprintln(os.Stderr, "natsbench: -msgs/-size/-pubs must be > 0 and -subs must be >= 0")
		os.Exit(2)
	}

	if err := run(*mode, *msgs, *size, *pubs, *subs, *subj, *timeout); err != nil {
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
}

func run(mode string, msgs, size, pubs, subs int, subj string, timeout time.Duration) error {
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

	_, _ = fmt.Printf("mode=%s pubs=%d subs=%d msgs=%d size=%d\n", mode, pubs, subs, msgs, size)
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
	hotStart := time.Now()
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
	hot := time.Since(hotStart)
	return result{
		setup:      setup,
		pubHot:     hot,
		deliverHot: hot,
		published:  int64(msgs),
		delivered:  delivered,
		subCount:   1,
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
	hotStart := time.Now()
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
	pubHot := pubDone.Sub(hotStart)
	deliverHot := subDone.Sub(hotStart)
	if subs == 0 {
		deliverHot = pubHot
	}

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
	hotStart := time.Now()
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
	pubHot := pubDone.Sub(hotStart)
	deliverHot := subDone.Sub(hotStart)
	if subs == 0 {
		deliverHot = pubHot
	}

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
		_, _ = fmt.Printf("total msgs delivered: %d (%d subs x %d)\n", r.delivered, r.subCount, msgs)
	}
	pubSecs := r.pubHot.Seconds()
	delSecs := r.deliverHot.Seconds()
	if pubSecs <= 0 || delSecs <= 0 {
		_, _ = fmt.Println("hot duration <= 0, skipping rate")
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
