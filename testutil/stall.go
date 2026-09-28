package testutil

import (
	"fmt"
	"os"
	"runtime"
	"runtime/metrics"
	"time"
)

// StartStallDetector reports when the test process stops running for
// longer than threshold. It checks a one second ticker and writes a line
// to stderr when a tick arrives late, so the report shows up in the test
// output next to the goroutine dump on a timeout. It stays quiet when the
// process runs normally. Call the returned function to stop it.
func StartStallDetector(threshold time.Duration) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		last := time.Now()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
			}
			now := time.Now()
			gap := now.Sub(last)
			last = now
			if gap < threshold {
				continue
			}
			samples := []metrics.Sample{
				{Name: "/memory/classes/heap/objects:bytes"},
				{Name: "/gc/cycles/total:gc-cycles"},
				{Name: "/sched/goroutines:goroutines"},
			}
			metrics.Read(samples)
			fmt.Fprintf(os.Stderr,
				"stall detector: %s passed between 1s ticks, ending at %s: heap=%dMiB gc_cycles=%d goroutines=%d gomaxprocs=%d\n",
				gap.Round(time.Millisecond),
				now.UTC().Format(time.RFC3339Nano),
				samples[0].Value.Uint64()>>20,
				samples[1].Value.Uint64(),
				samples[2].Value.Uint64(),
				runtime.GOMAXPROCS(0),
			)
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}
