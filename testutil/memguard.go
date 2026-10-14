package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"runtime/pprof"
	"strconv"
	"time"

	"golang.org/x/xerrors"
)

// MemoryLimitEnv is the environment variable StartMemoryGuardFromEnv reads
// the limit from, in MiB. CI sets it per runner in test-go-pg/action.yaml:
// 5120 on macOS, whose 17 GiB runner must trip before the compressor and
// swap absorb a runaway allocation, and 6144 elsewhere. The values assume
// -tags=testsmallbatch, which keeps wireguard-go's memory use low; without
// that tag, a plain `go test` run can activate the guard with no real leak.
const MemoryLimitEnv = "CODER_TEST_MEMORY_LIMIT_MIB"

// heapProfileGCTimeout bounds the forced collection that precedes the
// second heap profile. A runaway test keeps allocating while the
// collection runs, and the process must fail before the runner starts
// swapping, so the second profile is skipped when the collection takes
// longer than this.
const heapProfileGCTimeout = 5 * time.Second

// StartMemoryGuardFromEnv is like StartMemoryGuard, but reads the limit
// from MemoryLimitEnv and does nothing when it is unset. Call the
// returned function to stop it.
func StartMemoryGuardFromEnv() func() {
	val := os.Getenv(MemoryLimitEnv)
	if val == "" {
		return func() {}
	}
	mib, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("%s=%q is not a valid MiB count: %v", MemoryLimitEnv, val, err))
	}
	return StartMemoryGuard(mib << 20)
}

// StartMemoryGuard ends the test process once its memory use passes
// limit. It writes heap profiles first, then panics with a dump of
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
		immediate, collected, err := writeHeapProfiles(dir, heapProfileGCTimeout)
		report := "heap profile: " + immediate
		switch {
		case err != nil:
			report += "; " + err.Error()
		case collected == "":
			report += fmt.Sprintf("; no profile after collection, it did not finish within %s", heapProfileGCTimeout)
		default:
			report += "; heap profile after collection: " + collected
		}
		debug.SetTraceback("all")
		panic(fmt.Sprintf("memory guard: test process uses %d MiB, over the %d MiB limit; %s",
			total>>20, limit>>20, report))
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

// writeHeapProfiles writes a heap profile at once and a second one after
// a forced collection, and returns the path of each. The runtime
// publishes allocation samples only after the second mark termination
// that follows them (see memRecord in runtime/mprof.go), so the first
// profile can miss the burst that tripped the guard even in its
// alloc_space view. The collection may run long or never finish when
// memory is this high, so collected is empty when it does not finish
// within gcTimeout. The immediate profile is kept in that case.
func writeHeapProfiles(dir string, gcTimeout time.Duration) (immediate, collected string, err error) {
	immediate, err = writeHeapProfile(dir, "")
	if err != nil {
		return "", "", xerrors.Errorf("write heap profile: %w", err)
	}
	if !collectWithin(gcTimeout) {
		return immediate, "", nil
	}
	collected, err = writeHeapProfile(dir, "-after-gc")
	if err != nil {
		return immediate, "", xerrors.Errorf("write heap profile after collection: %w", err)
	}
	return immediate, collected, nil
}

// collectWithin runs a garbage collection and reports whether it
// finished within timeout. The collection keeps running otherwise.
func collectWithin(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.GC()
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func writeHeapProfile(dir, suffix string) (string, error) {
	name := fmt.Sprintf("heap-%s-%s%s.pprof",
		filepath.Base(os.Args[0]), time.Now().UTC().Format("20060102T150405Z"), suffix)
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
