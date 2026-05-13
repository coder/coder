//nolint:testpackage
package nats

// These benchmarks require `go test -bench ... -benchtime=<msgs>x`.
// The integer `<msgs>` is the explicit message count, equivalent to
// upstream `nats bench --msgs`. Time-based `-benchtime` values are not
// supported because testing.B calibration would rerun expensive NATS
// topologies with changing b.N values, and b.N would stop matching
// the upstream message-count model.

import (
	"bytes"
	"flag"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	// benchMaxPayload is the configured NATS MaxPayload so a 512 KiB
	// payload always fits regardless of upstream default drift.
	benchMaxPayload int32 = 1 << 20
	// benchPendingBytes is a generous per-subscription byte limit
	// (512 MiB) chosen so the fanout loop can flood the subscriber
	// pending queue without immediate drops at the swept fan-out
	// sizes. NATS rejects a non-zero Bytes with a zero Msgs, so
	// PendingLimits.Msgs is set to -1 (unlimited).
	benchPendingBytes = 512 << 20
	// benchDeliveryDeadline bounds how long the harness waits for
	// in-flight deliveries after the publish loop completes. Five
	// minutes prevents indefinite hangs while leaving headroom for
	// the largest sweep leaves on a loaded developer machine.
	benchDeliveryDeadline = 5 * time.Minute
)

// benchPayloads sweeps payload sizes that bracket realistic Coder
// pubsub traffic: 8 KiB for common control-plane messages and 512 KiB
// for the upper end of legitimate payloads.
var benchPayloads = []struct {
	name string
	size int
}{
	{"8KiB", 8 * 1024},
	{"512KiB", 512 * 1024},
}

// makePayload returns a deterministic, non-zero byte slice of the
// requested size.
func makePayload(size int) []byte {
	return bytes.Repeat([]byte("x"), size)
}

func benchPendingLimits() PendingLimits {
	return PendingLimits{Msgs: -1, Bytes: benchPendingBytes}
}

// isFixedBenchtime returns true if v is a fixed-iteration `-benchtime`
// value of the form "<int>x".
func isFixedBenchtime(v string) bool {
	_, ok := parseFixedBenchtime(v)
	return ok
}

// parseFixedBenchtime parses a `-benchtime=<int>x` value and returns
// the integer iteration count.
func parseFixedBenchtime(v string) (int, bool) {
	if !strings.HasSuffix(v, "x") {
		return 0, false
	}
	n := strings.TrimSuffix(v, "x")
	if n == "" {
		return 0, false
	}
	var out int
	for _, r := range n {
		if r < '0' || r > '9' {
			return 0, false
		}
		out = out*10 + int(r-'0')
	}
	return out, true
}

// benchTargetN returns the target iteration count from `-benchtime=Nx`.
// Returns 0 if not set.
func benchTargetN() int {
	f := flag.Lookup("test.benchtime")
	if f == nil {
		return 0
	}
	n, _ := parseFixedBenchtime(f.Value.String())
	return n
}

// isBenchWarmup reports whether the current b.N is the testing.B
// discovery pass (b.N=1) rather than the target run. testing.B always
// runs the benchmark function once with b.N=1 before the real run,
// even with `-benchtime=<msgs>x`. Skipping expensive setup on the
// warmup pass keeps the leaf cost predictable and prevents the
// `--- BENCH:` log block from showing warmup output.
func isBenchWarmup(b *testing.B) bool {
	b.Helper()
	target := benchTargetN()
	return target > 1 && b.N < target
}

// requireFixedBenchtime fast-fails the benchmark unless the operator
// supplied `-benchtime=<msgs>x`. b.N is treated as the total message
// count per leaf, equivalent to upstream `nats bench --msgs`. Allowing
// time-based calibration would rerun expensive NATS topologies with
// changing b.N and break the message-count contract.
func requireFixedBenchtime(b *testing.B) {
	b.Helper()
	f := flag.Lookup("test.benchtime")
	got := ""
	if f != nil {
		got = f.Value.String()
	}
	if !isFixedBenchtime(got) {
		b.Fatalf("coderd/x/nats benchmarks require -benchtime=<msgs>x, got -benchtime=%q", got)
	}
}

// splitCounts distributes total across clients message buckets so that
// the sum is exactly total and the bucket sizes differ by at most one.
// The remainder is distributed across the first buckets, matching
// upstream natscli message distribution.
func splitCounts(total, clients int) []int {
	if clients <= 0 {
		return nil
	}
	out := make([]int, clients)
	base := total / clients
	rem := total % clients
	for i := range out {
		out[i] = base
		if i < rem {
			out[i]++
		}
	}
	return out
}

// benchSubjectID generates a unique subject suffix per leaf invocation
// so stragglers from a prior leaf cannot inflate the next leaf's
// counters.
var benchSubjectID atomic.Uint64

func uniqueBenchSubject(prefix string) string {
	return fmt.Sprintf("%s.%d", prefix, benchSubjectID.Add(1))
}
