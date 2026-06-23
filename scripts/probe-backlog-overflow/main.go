// Command probe-backlog-overflow measures how the local OS responds to TCP
// accept-queue overflow. It opens a listening socket with a configurable
// backlog, never accepts, then fires N concurrent dials and classifies each
// outcome (success, refused, timeout, other). The goal is to compare Linux
// and Windows: under accept-queue overflow Linux silently drops the SYN
// while Windows answers with RST.
//
// Throwaway diagnostic for PLAT-307. Not wired into the regular build.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"time"
)

type outcome struct {
	idx     int
	elapsed time.Duration
	err     error
}

func (o outcome) class() string {
	switch {
	case o.err == nil:
		return "success"
	case errors.Is(o.err, syscall.ECONNREFUSED):
		return "refused"
	case errors.Is(o.err, syscall.ECONNRESET):
		return "reset"
	}
	// Timeout detection has to cover three flavors that Go's net package
	// surfaces interchangeably for a dial that runs out the clock:
	//   - context.DeadlineExceeded (ctx-driven cancellation)
	//   - os.ErrDeadlineExceeded   (net package's internal deadline)
	//   - any error reporting net.Error.Timeout() (catch-all, includes the
	//     bare "i/o timeout" wrapper that net.OpError returns when the
	//     dial loop exhausts its SYN-retransmit budget on Linux).
	if errors.Is(o.err, context.DeadlineExceeded) ||
		errors.Is(o.err, os.ErrDeadlineExceeded) {
		return "timeout"
	}
	var ne net.Error
	if errors.As(o.err, &ne) && ne.Timeout() {
		return "timeout"
	}
	return "other"
}

func main() {
	backlog := flag.Int("backlog", 1, "listen(2) backlog hint passed to the kernel")
	n := flag.Int("dials", 200, "number of concurrent dials")
	timeout := flag.Duration("timeout", 6*time.Second, "per-dial timeout (must exceed Linux SYN retransmit window if a refused/silent-drop comparison is desired)")
	flag.Parse()

	addr, closeFn, err := listenOnRandomPort(*backlog)
	if err != nil {
		log.Fatalf("listenOnRandomPort: %v", err)
	}
	defer func() { _ = closeFn() }()

	fmt.Printf("os:        %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("address:   %s\n", addr)
	fmt.Printf("backlog:   %d (kernel-requested; the OS may cap this)\n", *backlog)
	fmt.Printf("dials:     %d\n", *n)
	fmt.Printf("timeout:   %v\n", *timeout)
	fmt.Println()

	results := make([]outcome, *n)
	var wg sync.WaitGroup
	gate := make(chan struct{})

	for i := 0; i < *n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-gate
			t0 := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), *timeout)
			defer cancel()
			d := net.Dialer{}
			conn, err := d.DialContext(ctx, "tcp", addr)
			elapsed := time.Since(t0)
			if conn != nil {
				_ = conn.Close()
			}
			results[idx] = outcome{idx: idx, elapsed: elapsed, err: err}
		}(i)
	}

	start := time.Now()
	close(gate)
	wg.Wait()
	wallTime := time.Since(start)

	fmt.Printf("wall time: %v\n\n", wallTime)

	buckets := map[string][]time.Duration{}
	samples := map[string]string{}
	for _, r := range results {
		c := r.class()
		buckets[c] = append(buckets[c], r.elapsed)
		if r.err != nil {
			if _, ok := samples[c]; !ok {
				samples[c] = r.err.Error()
			}
		}
	}

	classes := []string{"success", "refused", "reset", "timeout", "other"}
	fmt.Printf("%-8s %5s  %-12s %-12s %-12s\n", "class", "count", "min", "p50", "max")
	for _, c := range classes {
		ds := buckets[c]
		if len(ds) == 0 {
			continue
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		fmt.Printf("%-8s %5d  %-12s %-12s %-12s\n", c, len(ds), ds[0], ds[len(ds)/2], ds[len(ds)-1])
	}

	if len(samples) > 0 {
		fmt.Println("\nerror samples (first occurrence per class):")
		for _, c := range classes {
			if s, ok := samples[c]; ok {
				fmt.Printf("  [%-7s] %s\n", c, s)
			}
		}
	}

	fmt.Println("\nlegend:")
	fmt.Println("  success — TCP three-way handshake completed (kernel queued the conn for accept).")
	fmt.Println("  refused — peer (or its kernel) sent RST during connect; immediate ECONNREFUSED.")
	fmt.Println("  reset   — connection established then RST; should not happen in this probe.")
	fmt.Println("  timeout — no response; client SYN-retransmit window expired (Linux silent-drop signature).")
	fmt.Println("  other   — unexpected; see error samples.")

	// Exit non-zero only if no dials completed at all (probe was unable to run).
	total := 0
	for _, ds := range buckets {
		total += len(ds)
	}
	if total != *n {
		fmt.Fprintf(os.Stderr, "internal error: counted %d outcomes for %d dials\n", total, *n)
		os.Exit(1)
	}
}
