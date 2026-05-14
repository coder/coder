package main

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/xerrors"
)

func TestAwaitCoderDeliveryDoneAllDone(t *testing.T) {
	t.Parallel()
	states := []*coderSubState{
		newCoderSubState(0), // pre-closed
		newCoderSubState(2),
	}
	// Simulate two hot deliveries on the second subscriber.
	go func() {
		states[1].count.Add(1)
		states[1].count.Add(1)
		states[1].markDone()
	}()
	var firstSubErr atomic.Value
	if err := awaitCoderDeliveryDone("delivery", time.Second, states, &firstSubErr); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestAwaitCoderDeliveryDoneTimeoutReportsDrops(t *testing.T) {
	t.Parallel()
	states := []*coderSubState{
		newCoderSubState(10),
		newCoderSubState(10),
	}
	states[0].count.Store(3)
	states[0].drops.Store(2)
	states[1].count.Store(0)
	var firstSubErr atomic.Value
	firstSubErr.Store(xerrors.New("listener died"))
	err := awaitCoderDeliveryDone("delivery", 25*time.Millisecond, states, &firstSubErr)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"delivered 3", "of 20", "drops=2", "subs=2", "first subscriber error", "listener died"} {
		if !strings.Contains(msg, want) {
			t.Errorf("timeout error %q missing %q", msg, want)
		}
	}
}

func TestCoderSubCallbackRoutesWarmup(t *testing.T) {
	t.Parallel()
	st := newClusterCoderSubState(5)
	var firstSubErr atomic.Value
	cb := coderSubCallback(st, &firstSubErr)
	// Warmup payload from replica 3 should not increment count.
	cb(t.Context(), warmupPayload(3), nil)
	if st.count.Load() != 0 {
		t.Errorf("warmup payload must not increment hot count, got %d", st.count.Load())
	}
	if !st.warmup.satisfied(1 << 3) {
		t.Errorf("warmup state must record replica 3")
	}
	// Hot payload increments count.
	cb(t.Context(), []byte{0, 1, 2, 3}, nil)
	if st.count.Load() != 1 {
		t.Errorf("hot payload must increment count, got %d", st.count.Load())
	}
}

func TestCoderSubCallbackRecordsDrops(t *testing.T) {
	t.Parallel()
	st := newCoderSubState(5)
	var firstSubErr atomic.Value
	cb := coderSubCallback(st, &firstSubErr)
	cb(t.Context(), nil, dummyDroppedMessagesError{})
	if st.drops.Load() != 1 {
		t.Errorf("expected drops=1, got %d", st.drops.Load())
	}
	if firstSubErr.Load() != nil {
		t.Errorf("drop error must not be recorded as first sub error")
	}
}

func TestCoderSubCallbackRecordsFirstSubErr(t *testing.T) {
	t.Parallel()
	st := newCoderSubState(5)
	var firstSubErr atomic.Value
	cb := coderSubCallback(st, &firstSubErr)
	sentinel := xerrors.New("listener exploded")
	cb(t.Context(), nil, sentinel)
	got, _ := firstSubErr.Load().(error)
	if !errors.Is(got, sentinel) {
		t.Errorf("expected sentinel via errors.Is, got %v", got)
	}
}

func TestBenchmarkDrainTimeoutCappedAt30s(t *testing.T) {
	t.Parallel()
	got := benchmarkDrainTimeout(time.Hour)
	if got != 30*time.Second {
		t.Errorf("got %s, want 30s cap", got)
	}
	got = benchmarkDrainTimeout(5 * time.Second)
	if got != 5*time.Second {
		t.Errorf("got %s, want 5s", got)
	}
	got = benchmarkDrainTimeout(0)
	if got != 30*time.Second {
		t.Errorf("got %s, want 30s default", got)
	}
}

func TestCleanupTimeoutCappedAt60s(t *testing.T) {
	t.Parallel()
	if got := cleanupTimeout(time.Hour); got != 60*time.Second {
		t.Errorf("got %s, want 60s cap", got)
	}
	if got := cleanupTimeout(5 * time.Second); got != 5*time.Second {
		t.Errorf("got %s, want 5s", got)
	}
}

func TestWarmupTimeoutBudget(t *testing.T) {
	t.Parallel()
	// 1/3 of -timeout, capped at 30s, floored at 250ms.
	if got := warmupTimeout(90 * time.Second); got != 30*time.Second {
		t.Errorf("got %s, want 30s cap", got)
	}
	if got := warmupTimeout(30 * time.Second); got != 10*time.Second {
		t.Errorf("got %s, want 10s", got)
	}
	if got := warmupTimeout(10 * time.Millisecond); got != 250*time.Millisecond {
		t.Errorf("got %s, want 250ms floor", got)
	}
}

type dummyDroppedMessagesError struct{}

func (dummyDroppedMessagesError) Error() string { return "dropped messages" }
func (dummyDroppedMessagesError) Is(target error) bool {
	// Treat as the pubsub.ErrDroppedMessages sentinel so the callback
	// routes us to st.drops via errors.Is. We import that sentinel in
	// coder_runners.go; mirroring it here keeps the test file free of
	// the pubsub import.
	return target.Error() == "dropped messages"
}
