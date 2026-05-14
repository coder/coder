package main

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/xerrors"
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
