package main

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/xerrors"

	codernats "github.com/coder/coder/v2/coderd/x/nats"
)

func TestBenchmarkPendingMsgsUsesMaxExpected(t *testing.T) {
	t.Parallel()
	// 2 pubs * 50 msgs/pub split (total=100), 4 subjects, 4 subs.
	// Subjects 0,1 each have one publisher emitting 50 msgs; subjects
	// 2,3 have zero publishers, so subscribers on them expect 0.
	// Max ExpectPerSub is 50, well below the floor (1024).
	plan, err := planSubjects(2, 4, 4, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	if got := benchmarkPendingMsgs(plan); got != benchmarkPendingMsgsFloor {
		t.Errorf("benchmarkPendingMsgs = %d, want floor %d (max expected = 50)", got, benchmarkPendingMsgsFloor)
	}
}

func TestBenchmarkPendingMsgsLargeExactDelivery(t *testing.T) {
	t.Parallel()
	// 10 pubs, 10 subs, 1 subject, msgs=100_000 total. Every sub expects
	// 100_000. That is above the floor and below the cap so the helper
	// should return it verbatim.
	plan, err := planSubjects(10, 10, 1, 100_000, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	if got := benchmarkPendingMsgs(plan); got != 100_000 {
		t.Errorf("benchmarkPendingMsgs = %d, want 100000", got)
	}
}

func TestBenchmarkPendingMsgsSymmetricMultiplesPerSubject(t *testing.T) {
	t.Parallel()
	// Symmetric: msgs is per-publisher. 10 pubs * 1000 msgs = 10_000 per
	// subject (with 1 subject) and each sub expects 10_000. The harness
	// must size pending from ExpectPerSub, not from raw msgs.
	plan, err := planSubjects(10, 30, 1, 1000, true)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	if got := benchmarkPendingMsgs(plan); got != 10_000 {
		t.Errorf("benchmarkPendingMsgs = %d, want 10000 (10 pubs * 1000 msgs)", got)
	}
}

func TestBenchmarkPendingMsgsCap(t *testing.T) {
	t.Parallel()
	// Request more than the cap; helper must clamp rather than allow
	// unbounded per-listener memory.
	plan, err := planSubjects(1, 1, 1, benchmarkPendingMsgsCap+5, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	if got := benchmarkPendingMsgs(plan); got != benchmarkPendingMsgsCap {
		t.Errorf("benchmarkPendingMsgs = %d, want cap %d", got, benchmarkPendingMsgsCap)
	}
}

func TestBenchmarkPendingMsgsZeroSubs(t *testing.T) {
	t.Parallel()
	// No subscribers at all: helper must not panic and must return
	// at least the floor so callers can pass it to Options without
	// special-casing.
	plan, err := planSubjects(1, 0, 1, 1_000, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	if got := benchmarkPendingMsgs(plan); got != benchmarkPendingMsgsFloor {
		t.Errorf("benchmarkPendingMsgs = %d, want floor %d", got, benchmarkPendingMsgsFloor)
	}
}

func TestFormatBenchTimeoutErrorIncludesDrops(t *testing.T) {
	t.Parallel()
	err := formatBenchTimeoutError(123, 1000, 4, 17, nil)
	if err == nil {
		t.Fatalf("formatBenchTimeoutError returned nil")
	}
	msg := err.Error()
	for _, want := range []string{"delivered 123", "of 1000", "subs=4", "drops=17"} {
		if !strings.Contains(msg, want) {
			t.Errorf("timeout error %q missing %q", msg, want)
		}
	}
}

func TestFormatBenchTimeoutErrorWrapsFirstSubErr(t *testing.T) {
	t.Parallel()
	sentinel := xerrors.New("connection broken")
	err := formatBenchTimeoutError(0, 10, 1, 0, sentinel)
	if err == nil {
		t.Fatalf("formatBenchTimeoutError returned nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected errors.Is to find sentinel in %v", err)
	}
	if !strings.Contains(err.Error(), "first subscriber error") {
		t.Errorf("expected 'first subscriber error' in %q", err.Error())
	}
}

func TestBenchmarkMaxPendingSmallPayloadHitsFloor(t *testing.T) {
	t.Parallel()
	// 2 pubs * 50 msgs total, 128 B payload. Estimate is well below
	// codernats.DefaultMaxPending (1 GiB), so the helper must
	// floor at DefaultMaxPending so the symmetric cluster runs never
	// silently choose a smaller budget than the wrapper production
	// default.
	plan, err := planSubjects(2, 2, 1, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	dec := benchmarkMaxPending(plan, 128, 0)
	if dec.Effective != codernats.DefaultMaxPending {
		t.Errorf("Effective = %d, want DefaultMaxPending %d", dec.Effective, codernats.DefaultMaxPending)
	}
	if dec.Forced {
		t.Errorf("Forced = true, want false (no override)")
	}
	if dec.BelowEstimate {
		t.Errorf("BelowEstimate = true, want false (estimate <= effective)")
	}
	if dec.Capped {
		t.Errorf("Capped = true, want false")
	}
	if dec.Estimate <= 0 {
		t.Errorf("Estimate = %d, want > 0", dec.Estimate)
	}
}

func TestBenchmarkMaxPendingLargePayloadExceedsDefault(t *testing.T) {
	t.Parallel()
	// Reproduce the failing 64 KiB symmetric run shape:
	//   -msgs=2000 -pubs=10 -subs=30 -subjects=1 (symmetric)
	//   total per sub = 20000, payload = 65536 B
	// Estimated need ~ 20000 * (65536 + 128) = ~1.31 GiB > 1 GiB
	// default. The helper must produce Effective > DefaultMaxPending
	// so the server does not slow-consumer-disconnect subscribers.
	plan, err := planSubjects(10, 30, 1, 2000, true)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	dec := benchmarkMaxPending(plan, 65536, 0)
	if dec.Effective <= codernats.DefaultMaxPending {
		t.Errorf("Effective = %d, want > DefaultMaxPending %d", dec.Effective, codernats.DefaultMaxPending)
	}
	if dec.Effective != dec.Estimate {
		t.Errorf("Effective = %d, want Estimate = %d (no cap, no override)", dec.Effective, dec.Estimate)
	}
	if dec.BelowEstimate {
		t.Errorf("BelowEstimate = true, want false")
	}
	if dec.Capped {
		t.Errorf("Capped = true, want false (estimate well under cap)")
	}
	// Sanity: the failing 128 MiB ceiling is now clearly exceeded.
	if dec.Effective < 128<<20 {
		t.Errorf("Effective = %d, want at least 128 MiB (1.31 GiB expected)", dec.Effective)
	}
}

func TestBenchmarkMaxPendingOverrideForced(t *testing.T) {
	t.Parallel()
	plan, err := planSubjects(10, 30, 1, 2000, true)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	const override int64 = 64 << 20
	dec := benchmarkMaxPending(plan, 65536, override)
	if dec.Effective != override {
		t.Errorf("Effective = %d, want override %d", dec.Effective, override)
	}
	if !dec.Forced {
		t.Errorf("Forced = false, want true")
	}
	// The override is below the estimate (~1.31 GiB), so the helper
	// must flag BelowEstimate so the header warning fires.
	if !dec.BelowEstimate {
		t.Errorf("BelowEstimate = false, want true (override 64 MiB << estimate)")
	}
	if dec.Capped {
		t.Errorf("Capped = true, want false (override path does not cap)")
	}
}

func TestBenchmarkMaxPendingOverrideAboveEstimate(t *testing.T) {
	t.Parallel()
	plan, err := planSubjects(2, 2, 1, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	const override int64 = 4 << 30 // 4 GiB, well above tiny estimate
	dec := benchmarkMaxPending(plan, 128, override)
	if dec.Effective != override {
		t.Errorf("Effective = %d, want %d", dec.Effective, override)
	}
	if !dec.Forced {
		t.Errorf("Forced = false, want true")
	}
	if dec.BelowEstimate {
		t.Errorf("BelowEstimate = true, want false (override > estimate)")
	}
}

func TestBenchmarkMaxPendingCapApplied(t *testing.T) {
	t.Parallel()
	// Construct a plan whose estimate exceeds benchmarkMaxPendingCap so
	// the helper clamps and reports Capped + BelowEstimate.
	// We need maxExpectedPerSub * (payload + 128) > 16 GiB.
	// payload = 1 MiB, maxExpectedPerSub = 17 * 1024 -> estimate
	// = 17_408 * (1 MiB + 128) > 16 GiB.
	plan, err := planSubjects(1, 1, 1, 17408, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	dec := benchmarkMaxPending(plan, 1<<20, 0)
	if dec.Effective != benchmarkMaxPendingCap {
		t.Errorf("Effective = %d, want cap %d", dec.Effective, benchmarkMaxPendingCap)
	}
	if !dec.Capped {
		t.Errorf("Capped = false, want true")
	}
	if !dec.BelowEstimate {
		t.Errorf("BelowEstimate = false, want true (capped implies below estimate)")
	}
}

func TestBenchmarkMaxPendingDescribeWarns(t *testing.T) {
	t.Parallel()
	plan, err := planSubjects(10, 30, 1, 2000, true)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	belowOverride := int64(128 << 20)
	dec := benchmarkMaxPending(plan, 65536, belowOverride)
	desc := dec.describe()
	for _, want := range []string{"max-pending=", "override-below-estimate", "WARNING"} {
		if !strings.Contains(desc, want) {
			t.Errorf("describe() = %q, missing %q", desc, want)
		}
	}
}

func TestBenchmarkMaxPendingDescribeNoWarnWhenSafe(t *testing.T) {
	t.Parallel()
	plan, err := planSubjects(2, 2, 1, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	dec := benchmarkMaxPending(plan, 128, 0)
	desc := dec.describe()
	if strings.Contains(desc, "WARNING") {
		t.Errorf("describe() = %q, did not expect WARNING (safe default)", desc)
	}
	if !strings.Contains(desc, "workload-derived") {
		t.Errorf("describe() = %q, missing source=workload-derived", desc)
	}
}

func TestBenchmarkMaxPendingZeroPlan(t *testing.T) {
	t.Parallel()
	// No subscribers: max expected is zero. Helper must still return
	// at least DefaultMaxPending so callers can pass the value to the
	// server without a special case.
	plan, err := planSubjects(2, 0, 1, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	dec := benchmarkMaxPending(plan, 128, 0)
	if dec.Effective < codernats.DefaultMaxPending {
		t.Errorf("Effective = %d, want >= DefaultMaxPending", dec.Effective)
	}
	if dec.BelowEstimate {
		t.Errorf("BelowEstimate = true, want false (zero estimate)")
	}
}

func TestFormatBenchTimeoutErrorZeroDropsStillReported(t *testing.T) {
	t.Parallel()
	// Even when drops==0 the harness must include the field so users
	// can tell a missing-delivery timeout apart from a drop-driven
	// timeout at a glance.
	err := formatBenchTimeoutError(5, 10, 2, 0, nil)
	if !strings.Contains(err.Error(), "drops=0") {
		t.Errorf("expected 'drops=0' in %q", err.Error())
	}
}
