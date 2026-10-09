package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"runtime/metrics"
	"runtime/pprof"
	"time"
)

// MemoryLimit is twice the peak of the largest test package in CI (cli,
// about 3 GiB) and a third of the smallest runner (macOS, 17 GiB).
const MemoryLimit uint64 = 6 << 30

// StartMemoryGuard ends the test process once its memory use passes
// limit. It writes a heap profile first, then panics with a dump of
// every goroutine the way a test timeout does. Without it a runaway
// allocation pushes the runner into swap and the process dies before
// the test timeout can produce a dump, see coder/internal#1365. Call
// the returned function to stop it.
func StartMemoryGuard(limit uint64) func() {
	return watchMemory(limit, func(total uint64) {
		dir := os.Getenv("RUNNER_TEMP")
		if dir == "" {
			dir = os.TempDir()
		}
		path, err := writeHeapProfile(dir)
		if err != nil {
			path = "not written: " + err.Error()
		}
		debug.SetTraceback("all")
		panic(fmt.Sprintf("memory guard: test process uses %d MiB, over the %d MiB limit; heap profile: %s",
			total>>20, limit>>20, path))
	})
}

func watchMemory(limit uint64, exceeded func(total uint64)) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		sample := []metrics.Sample{{Name: "/memory/classes/total:bytes"}}
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
			}
			metrics.Read(sample)
			total := sample[0].Value.Uint64()
			if total < limit {
				continue
			}
			exceeded(total)
			return
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

// writeHeapProfile does not run a collection first. The collector may be
// stuck when memory is this high, and allocation sites are sampled as
// they happen regardless.
func writeHeapProfile(dir string) (string, error) {
	name := fmt.Sprintf("heap-%s-%s.pprof",
		filepath.Base(os.Args[0]), time.Now().UTC().Format("20060102T150405Z"))
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if err := pprof.Lookup("heap").WriteTo(f, 0); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return path, nil
}
