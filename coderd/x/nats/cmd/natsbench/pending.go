package main

import (
	"fmt"

	"golang.org/x/xerrors"
)

// Benchmark harness sizing for the Coder Pubsub per-listener local
// queue. After same-subject coalescing the shared NATS subscription
// drains into per-listener bounded inboxes; setting Options.PendingLimits.Msgs
// to a positive value sizes BOTH the underlying *natsgo.Subscription
// pending limit and the per-listener inbox (see codernats.listenerQueueSize).
// natsbench exact-delivery throughput modes need enough capacity to
// absorb the entire expected per-subscriber burst, otherwise the local
// queue overflows and messages drop while the benchmark waits silently
// for deliveries that will never come.
//
// These constants are a BENCHMARK HARNESS setting, not a production
// recommendation. Real callers should choose PendingLimits based on the
// memory budget they actually have for one subscriber to fall behind by.
const (
	// benchmarkPendingMsgsFloor matches the wrapper's defaultListenerQueueSize
	// so we never SHRINK the inbox below the production default.
	benchmarkPendingMsgsFloor = 1024
	// benchmarkPendingMsgsCap bounds worst-case memory at roughly one
	// million message pointers (~8 MiB of pointers per listener on a
	// 64-bit machine, before payload bytes). If a benchmark configuration
	// asks for more than this per subscriber we cap; drops above the cap
	// are visible via the new drop-signal accounting rather than being
	// hidden behind a larger queue.
	benchmarkPendingMsgsCap = 1 << 20
)

// benchmarkPendingMsgs returns the per-listener pending-message capacity
// to use for an exact-delivery coder natsbench run. It is derived from
// the subjectPlan so symmetric and asymmetric modes use the right value
// (in symmetric modes plan.ExpectPerSub[j] already accounts for
// msgs*publishers_on_subject).
//
// Returns at least benchmarkPendingMsgsFloor and at most
// benchmarkPendingMsgsCap. Returns the floor when the plan has no
// subscribers or every subscriber expects zero messages.
func benchmarkPendingMsgs(plan subjectPlan) int {
	var maxExpected int64
	for _, e := range plan.ExpectPerSub {
		if e > maxExpected {
			maxExpected = e
		}
	}
	if maxExpected < int64(benchmarkPendingMsgsFloor) {
		return benchmarkPendingMsgsFloor
	}
	if maxExpected > int64(benchmarkPendingMsgsCap) {
		return benchmarkPendingMsgsCap
	}
	return int(maxExpected)
}

// formatBenchTimeoutError builds the timeout error returned by a coder
// natsbench runner when subscribers do not reach their expected
// delivery counts before the deadline. The message always includes
// drop-signal accounting so a silent local-queue overflow is visible
// instead of disguised as "timed out for no reason". If the run
// observed a non-drop subscriber error it is wrapped so callers see
// it as the timeout's cause.
func formatBenchTimeoutError(delivered, expected int64, subs int, drops int64, firstSubErr error) error {
	base := fmt.Sprintf("timeout: delivered %d of %d (subs=%d, drops=%d)", delivered, expected, subs, drops)
	if firstSubErr != nil {
		return xerrors.Errorf("%s: first subscriber error: %w", base, firstSubErr)
	}
	return xerrors.New(base)
}
