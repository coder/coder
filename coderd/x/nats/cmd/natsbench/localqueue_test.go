package main

import (
	"strings"
	"testing"
	"unsafe"
)

func TestLocalQueueSlotBytesMatchesSliceHeader(t *testing.T) {
	t.Parallel()
	want := int64(unsafe.Sizeof([]byte(nil)))
	if localQueueSlotBytes != want {
		t.Fatalf("localQueueSlotBytes = %d, want %d", localQueueSlotBytes, want)
	}
	// On 64-bit Go runtimes the slice header is 24 bytes. On 32-bit
	// it is 12 bytes. We only fail if it is implausibly small (which
	// would indicate we accidentally went back to "pointer size" 8).
	if localQueueSlotBytes < 12 {
		t.Errorf("localQueueSlotBytes = %d is implausible for a slice header", localQueueSlotBytes)
	}
}

func TestLocalQueueCapacityZeroPlanDerived(t *testing.T) {
	t.Parallel()
	plan, err := planSubjects(2, 4, 4, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	cap1, clamped, err := localQueueCapacity(plan, 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if clamped {
		t.Errorf("plan-derived must not report clamped")
	}
	if cap1 != benchmarkPendingMsgs(plan) {
		t.Errorf("plan-derived capacity = %d, want %d", cap1, benchmarkPendingMsgs(plan))
	}
}

func TestLocalQueueCapacityOverrideRespectsFloor(t *testing.T) {
	t.Parallel()
	plan, err := planSubjects(1, 1, 1, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	cap1, clamped, err := localQueueCapacity(plan, 16)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !clamped {
		t.Errorf("override=16 must clamp to floor=%d", benchmarkPendingMsgsFloor)
	}
	if cap1 != benchmarkPendingMsgsFloor {
		t.Errorf("capacity = %d, want floor %d", cap1, benchmarkPendingMsgsFloor)
	}
}

func TestLocalQueueCapacityOverrideRespectsCap(t *testing.T) {
	t.Parallel()
	plan, err := planSubjects(1, 1, 1, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	cap1, clamped, err := localQueueCapacity(plan, benchmarkPendingMsgsCap+1)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !clamped {
		t.Errorf("override above cap must clamp")
	}
	if cap1 != benchmarkPendingMsgsCap {
		t.Errorf("capacity = %d, want cap %d", cap1, benchmarkPendingMsgsCap)
	}
}

func TestLocalQueueCapacityOverrideMidrange(t *testing.T) {
	t.Parallel()
	plan, err := planSubjects(1, 1, 1, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	cap1, clamped, err := localQueueCapacity(plan, 8192)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if clamped {
		t.Errorf("override in [floor, cap] must not be reported as clamped")
	}
	if cap1 != 8192 {
		t.Errorf("capacity = %d, want 8192", cap1)
	}
}

func TestLocalQueueCapacityNegativeIsError(t *testing.T) {
	t.Parallel()
	plan, err := planSubjects(1, 1, 1, 100, false)
	if err != nil {
		t.Fatalf("planSubjects: %v", err)
	}
	if _, _, err := localQueueCapacity(plan, -1); err == nil {
		t.Fatalf("expected error for negative override")
	}
}

func TestLocalQueueMemoryEstimate(t *testing.T) {
	t.Parallel()
	// 1024 slots * 30 listeners * 24 bytes = 720 KiB on 64-bit.
	got := localQueueMemoryEstimate(1024, 30)
	want := int64(1024) * 30 * localQueueSlotBytes
	if got != want {
		t.Errorf("estimate = %d, want %d", got, want)
	}
}

func TestLocalQueueMemoryEstimateZeroes(t *testing.T) {
	t.Parallel()
	if got := localQueueMemoryEstimate(0, 30); got != 0 {
		t.Errorf("capacity=0 must return 0 estimate, got %d", got)
	}
	if got := localQueueMemoryEstimate(1024, 0); got != 0 {
		t.Errorf("listeners=0 must return 0 estimate, got %d", got)
	}
}

func TestLocalQueueDescriptionPlanDerived(t *testing.T) {
	t.Parallel()
	d := localQueueDescription(1024, 4, 0, false)
	for _, want := range []string{"local-queue-msgs=1024", "source=plan-derived", "listeners=4", "chan-buf~="} {
		if !strings.Contains(d, want) {
			t.Errorf("description %q missing %q", d, want)
		}
	}
}

func TestLocalQueueDescriptionOverrideClamped(t *testing.T) {
	t.Parallel()
	d := localQueueDescription(benchmarkPendingMsgsCap, 8, benchmarkPendingMsgsCap+1, true)
	if !strings.Contains(d, "source=override-clamped") {
		t.Errorf("expected override-clamped tag, got %q", d)
	}
}

func TestHumanBytesAbs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{2 * 1024, "2.00 KiB"},
		{3 * 1024 * 1024, "3.00 MiB"},
	}
	for _, c := range cases {
		if got := humanBytesAbs(c.n); got != c.want {
			t.Errorf("humanBytesAbs(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
