package main

import (
	"fmt"
	"math"

	"golang.org/x/xerrors"

	codernats "github.com/coder/coder/v2/coderd/x/nats"
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

// Benchmark harness sizing for the server-side per-client outbound
// pending byte budget (codernats.Options.MaxPending and the equivalent
// natsserver.Options.MaxPending). MaxPending is enforced server-side:
// if a client's outbound queue ever exceeds it, the nats-server flags
// the connection as a slow consumer and disconnects it. In exact-
// delivery benchmark runs that is silent at-most-once message loss
// from the receiver's perspective (no Pubsub ErrDroppedMessages
// signal), so the harness must size MaxPending to comfortably hold a
// worst-case burst for one subscriber connection.
const (
	// benchmarkProtocolOverheadBytes is a conservative per-message
	// NATS protocol overhead allowance (MSG verb, subject, sid, reply,
	// CRLF, etc.). Real overhead is closer to ~30-50 bytes for the
	// natsbench subject namespaces but we round up for safety so the
	// derived MaxPending always sits above the actual byte cost.
	benchmarkProtocolOverheadBytes int64 = 128

	// benchmarkMaxPendingCap caps the workload-derived MaxPending so a
	// run with an absurdly large -msgs and -size combination cannot
	// silently ask each replica to reserve tens of gigabytes of
	// outbound buffer per client connection. 16 GiB is large enough
	// for every realistic natsbench configuration; runs that estimate
	// higher trip benchmarkMaxPendingResult.Capped and the harness
	// surfaces a clear "drops are possible" warning so the user knows
	// to either reduce the workload or override -max-pending.
	benchmarkMaxPendingCap int64 = 16 << 30
)

// benchmarkMaxPendingResult is the decision returned by
// benchmarkMaxPending. Callers pass Effective to the server option and
// use the remaining fields for header / warning lines.
//
// Forced indicates -max-pending was set to a positive value, in which
// case Effective is exactly that override. BelowEstimate is true when
// Effective is strictly less than the workload-derived Estimate; this
// is the "you asked for drops" diagnostic and is emitted whenever the
// override or the safety cap forces the harness below the estimate.
// Capped is true when the cap was the deciding factor (BelowEstimate
// is then also true).
type benchmarkMaxPendingResult struct {
	// Effective is the byte value to pass to the server.
	Effective int64
	// Estimate is the workload-derived requirement
	// (maxExpectedPerSub * (payloadSize + protocolOverhead)).
	Estimate int64
	// Forced is true when the operator overrode MaxPending via
	// -max-pending; Effective is then exactly the override.
	Forced bool
	// BelowEstimate is true when Effective < Estimate. In exact-
	// delivery runs this means a subscriber burst can exceed
	// MaxPending and the server may disconnect the subscriber as a
	// slow consumer, causing silent message loss.
	BelowEstimate bool
	// Capped is true when the workload-derived value was clamped to
	// benchmarkMaxPendingCap. Implies BelowEstimate.
	Capped bool
}

// benchmarkMaxPending derives the per-client outbound pending byte
// budget (server-side MaxPending) for an exact-delivery natsbench
// run. The formula is intentionally simple:
//
//	estimate = maxExpectedPerSub * (payloadSize + protocolOverhead)
//
// where maxExpectedPerSub is the largest entry in plan.ExpectPerSub
// (i.e. the worst-case number of messages a single subscriber
// connection might have to hold pending if its callback stalls). If
// an operator-supplied override is positive it is used verbatim and
// the helper records whether it sits below the estimate so the header
// can advertise that drops are possible. Otherwise the helper applies
// a floor at codernats.DefaultMaxPending so exact-delivery benchmark
// defaults never choose less than the wrapper production default, and
// a cap at benchmarkMaxPendingCap so absurd workloads do not silently
// ask for tens of gigabytes per replica.
func benchmarkMaxPending(plan subjectPlan, payloadSize int, override int64) benchmarkMaxPendingResult {
	if payloadSize < 0 {
		payloadSize = 0
	}
	var maxExpected int64
	for _, e := range plan.ExpectPerSub {
		if e > maxExpected {
			maxExpected = e
		}
	}
	perMsg := int64(payloadSize) + benchmarkProtocolOverheadBytes
	// Overflow-safe multiplication: clamp to MaxInt64 if the product
	// would overflow. A run that hits this is already in territory
	// where the cap will kick in below.
	estimate := int64(0)
	if maxExpected > 0 && perMsg > 0 {
		if maxExpected > math.MaxInt64/perMsg {
			estimate = math.MaxInt64
		} else {
			estimate = maxExpected * perMsg
		}
	}
	if override > 0 {
		return benchmarkMaxPendingResult{
			Effective:     override,
			Estimate:      estimate,
			Forced:        true,
			BelowEstimate: estimate > override,
		}
	}
	effective := estimate
	if effective < codernats.DefaultMaxPending {
		effective = codernats.DefaultMaxPending
	}
	var capped bool
	if effective > benchmarkMaxPendingCap {
		effective = benchmarkMaxPendingCap
		capped = true
	}
	return benchmarkMaxPendingResult{
		Effective:     effective,
		Estimate:      estimate,
		BelowEstimate: estimate > effective,
		Capped:        capped,
	}
}

// describe renders the header line that summarizes the MaxPending
// decision. Includes the effective value, the estimate, the source
// (workload-derived, override, override-below-estimate, etc.), and a
// clear "WARNING: drops are possible" tail when the effective value
// is below the workload estimate so an operator does not silently get
// at-most-once delivery on what is supposed to be an exact-delivery
// run.
func (d benchmarkMaxPendingResult) describe() string {
	source := "workload-derived"
	switch {
	case d.Forced && d.BelowEstimate:
		source = "override-below-estimate"
	case d.Forced:
		source = "override"
	case d.Capped:
		source = "workload-derived-capped"
	}
	out := fmt.Sprintf("max-pending=%s (source=%s, estimate=%s)",
		humanBytesAbs(d.Effective), source, humanBytesAbs(d.Estimate))
	if d.BelowEstimate {
		out += " WARNING: effective < estimate; server may disconnect slow consumers and drop messages"
	}
	return out
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
