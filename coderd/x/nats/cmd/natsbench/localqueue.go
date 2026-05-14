package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/xerrors"
)

// localQueueSlotBytes is the in-memory size of a `[]byte` slice header
// on a 64-bit machine (pointer + length + capacity = 24 bytes).
// codernats.subscription.queue is `chan []byte`, so each pending-message
// slot in the per-listener inbox stores a slice header. The slice
// payload bytes live in a separate allocation (the *natsgo.Msg.Data
// buffer that the wrapper fans out zero-copy), so the channel buffer
// footprint scales linearly in this header size, not the payload size.
//
// The legacy "8 MiB of pointers" comment in pending.go was computing
// the wrong overhead. This constant exists so the header line prints
// an honest estimate.
const localQueueSlotBytes = int64(unsafe.Sizeof([]byte(nil)))

// localQueueCapacity returns the effective per-listener inbox capacity
// for a Coder natsbench run. override < 0 is an error. override == 0
// keeps the plan-derived default from benchmarkPendingMsgs. override > 0
// is clamped to [benchmarkPendingMsgsFloor, benchmarkPendingMsgsCap]
// so an operator typo cannot ask the wrapper to allocate an unbounded
// per-listener channel buffer.
//
// The cap is intentionally explicit: it bounds worst-case allocation
// before the run starts so a multi-million-msg run cannot look like
// "setup hangs" when it is really just a multi-gigabyte channel
// allocation. Asking for more capacity than the cap is reported on
// stderr (caller's responsibility) so the user sees the clamp.
func localQueueCapacity(plan subjectPlan, override int) (effective int, clamped bool, err error) {
	if override < 0 {
		return 0, false, xerrors.Errorf("local-queue-msgs must be >= 0, got %d", override)
	}
	if override == 0 {
		return benchmarkPendingMsgs(plan), false, nil
	}
	if override < benchmarkPendingMsgsFloor {
		return benchmarkPendingMsgsFloor, true, nil
	}
	if override > benchmarkPendingMsgsCap {
		return benchmarkPendingMsgsCap, true, nil
	}
	return override, false, nil
}

// localQueueMemoryEstimate is an approximate footprint for the
// per-listener inbox channel buffers across all listeners. It counts
// the slice-header slots in each chan []byte buffer; it does NOT count
// the payload bytes pointed at by those slices because the payload is
// pooled and zero-copied across listeners on the same shared
// subscription (see codernats.subscription.emit).
//
// The result is informational only; the wrapper allocates the channel
// buffer lazily-ish via make(chan []byte, cap), so this is a worst-case
// upper bound on the chan buffer footprint.
func localQueueMemoryEstimate(capacity, listeners int) int64 {
	if capacity <= 0 || listeners <= 0 {
		return 0
	}
	return int64(capacity) * int64(listeners) * localQueueSlotBytes
}

// localQueueDescription renders the header line for a Coder benchmark
// run describing the effective local-queue capacity and the slice-header
// memory estimate across all listeners. Example:
//
//	local-queue-msgs=4096 listeners=30 chan-buf~=2.81 MiB
//
// When override > 0 and the value was clamped to the floor/cap, the
// returned source string makes that visible so the operator does not
// silently get a different capacity than they asked for.
//
//nolint:revive // clamped is a status flag returned by localQueueCapacity, not a control flag.
func localQueueDescription(capacity, listeners int, override int, clamped bool) string {
	source := "plan-derived"
	if override > 0 {
		source = "override"
		if clamped {
			source = "override-clamped"
		}
	}
	mem := localQueueMemoryEstimate(capacity, listeners)
	return fmt.Sprintf("local-queue-msgs=%d (source=%s) listeners=%d chan-buf~=%s",
		capacity, source, listeners, humanBytesAbs(mem))
}

// humanBytesAbs renders a byte count using IEC units. Unlike
// humanBytes, which takes a per-second rate (float), this helper takes
// an absolute integer byte count.
func humanBytesAbs(n int64) string {
	const (
		kib = int64(1024)
		mib = 1024 * kib
		gib = 1024 * mib
		tib = 1024 * gib
	)
	switch {
	case n >= tib:
		return fmt.Sprintf("%.2f TiB", float64(n)/float64(tib))
	case n >= gib:
		return fmt.Sprintf("%.2f GiB", float64(n)/float64(gib))
	case n >= mib:
		return fmt.Sprintf("%.2f MiB", float64(n)/float64(mib))
	case n >= kib:
		return fmt.Sprintf("%.2f KiB", float64(n)/float64(kib))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
