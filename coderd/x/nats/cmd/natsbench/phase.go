package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/xerrors"
)

// phaseTimeoutError is the error returned when a benchmark phase
// exceeds its allotted timeout. The stack snapshot is written to
// stderr by awaitOrTimeout (not stored on the error) so the operator
// can tell apart hangs in publish vs Flush vs delivery vs cleanup at
// a glance without bloating the error value.
type phaseTimeoutError struct {
	phase   string
	timeout time.Duration
	diag    string
}

func (e *phaseTimeoutError) Error() string {
	msg := fmt.Sprintf("phase %q timed out after %s", e.phase, e.timeout)
	if e.diag != "" {
		msg += "\n" + e.diag
	}
	return msg
}

// dumpStacks returns a snapshot of all goroutine stacks. It uses a
// growing buffer so the snapshot is not truncated for runs with many
// goroutines (large cluster runs spawn one publisher + one subscriber
// goroutine pair per local listener plus N replica internals).
func dumpStacks() []byte {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
		if len(buf) > 64<<20 {
			// Hard cap. If we somehow have more than 64 MiB of stacks
			// the truncated dump is still useful.
			n = runtime.Stack(buf, true)
			return buf[:n]
		}
	}
}

// writeStacksTo writes a labeled goroutine dump to w. Returns the
// number of bytes written and the first write error (if any).
func writeStacksTo(w io.Writer, label string) {
	_, _ = fmt.Fprintf(w, "\n=== goroutine dump: %s ===\n", label)
	_, _ = w.Write(dumpStacks())
	_, _ = fmt.Fprintln(w, "=== end goroutine dump ===")
}

// awaitOrTimeout waits for done to close or for timeout to elapse. On
// timeout it dumps all goroutine stacks to stderr (so users can identify
// which phase hung even when the runner returns its result) and returns
// a phaseTimeoutError labeled with phase. diag, if non-nil, is invoked
// when the timeout fires and its return value is included in the error
// (e.g. published/delivered counters at the moment of timeout).
func awaitOrTimeout(phase string, timeout time.Duration, done <-chan struct{}, diag func() string) error {
	if timeout <= 0 {
		// Treat a non-positive timeout as "wait forever". This matches the
		// historical natsbench behavior when -timeout is interpreted as the
		// delivery wait only; callers that want a bounded wait pass a
		// positive value.
		<-done
		return nil
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-done:
		return nil
	case <-t.C:
		var d string
		if diag != nil {
			d = diag()
		}
		writeStacksTo(os.Stderr, fmt.Sprintf("phase %q timeout", phase))
		return &phaseTimeoutError{phase: phase, timeout: timeout, diag: d}
	}
}

// awaitWaitGroup is a convenience wrapper that bounds wg.Wait with
// awaitOrTimeout. The returned closer goroutine never leaks: it always
// observes wg.Wait either before or after the timeout fires, then exits.
func awaitWaitGroup(phase string, timeout time.Duration, wg *sync.WaitGroup, diag func() string) error {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	return awaitOrTimeout(phase, timeout, done, diag)
}

// runBoundedCleanup invokes fn with the given timeout. If fn returns
// before the timeout, returns fn's error. If the timeout fires first, it
// dumps goroutine stacks to stderr and returns a phaseTimeoutError. fn
// continues running in the background; the natsbench process is
// expected to exit shortly after the deferred cleanup runs, which
// terminates any leftover cleanup goroutine.
//
// Cleanup is explicitly bounded so the benchmark cannot silently hang
// AFTER successful delivery while waiting for resource teardown. If
// teardown hangs, users still get their results printed and a
// diagnostic showing where teardown is stuck.
func runBoundedCleanup(phase string, timeout time.Duration, fn func() error) error {
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case err := <-done:
		return err
	case <-t.C:
		writeStacksTo(os.Stderr, fmt.Sprintf("cleanup %q timeout", phase))
		return &phaseTimeoutError{phase: phase, timeout: timeout, diag: "cleanup did not return; see goroutine dump on stderr"}
	}
}

// reportCleanupErr logs a non-fatal cleanup error to stderr so it does
// not silently suppress the benchmark's already-computed results. The
// benchmark result is still printed by run().
func reportCleanupErr(label string, err error) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "natsbench: cleanup warning (%s): %v\n", label, err)
}

// publishPhaseDiag is the diagnostic snapshot for a publish-phase
// timeout. It is intentionally compact: published count, total expected
// publishes, delivered count so far, expected deliveries, drops, first
// publish error, and first subscriber error. The goroutine stacks are
// emitted separately by awaitOrTimeout.
type publishPhaseDiag struct {
	published       int64
	expectPublished int64
	delivered       int64
	expectDelivered int64
	drops           int64
	firstPubErr     error
	firstSubErr     error
}

func (d publishPhaseDiag) String() string {
	out := fmt.Sprintf("published=%d/%d delivered=%d/%d drops=%d",
		d.published, d.expectPublished, d.delivered, d.expectDelivered, d.drops)
	if d.firstPubErr != nil {
		out += fmt.Sprintf(" first_publish_err=%q", d.firstPubErr.Error())
	}
	if d.firstSubErr != nil {
		out += fmt.Sprintf(" first_sub_err=%q", d.firstSubErr.Error())
	}
	return out
}

// wrapPhaseError wraps a phase error so users see the phase name even
// when the underlying error is propagated up by xerrors.Errorf.
func wrapPhaseError(phase string, err error) error {
	if err == nil {
		return nil
	}
	return xerrors.Errorf("%s: %w", phase, err)
}
