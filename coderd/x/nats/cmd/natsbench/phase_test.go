package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/xerrors"
)

func TestAwaitOrTimeoutDoneFirst(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	close(done)
	if err := awaitOrTimeout("test", time.Minute, done, nil); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestAwaitOrTimeoutFiresOnTimeout(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	called := false
	err := awaitOrTimeout("publish", 10*time.Millisecond, done, func() string {
		called = true
		return "delivered=3 of 10"
	})
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	var pe *phaseTimeoutError
	if !errors.As(err, &pe) {
		t.Fatalf("expected phaseTimeoutError, got %T", err)
	}
	if pe.phase != "publish" {
		t.Errorf("phase = %q, want %q", pe.phase, "publish")
	}
	if !called {
		t.Errorf("expected diag callback to be invoked on timeout")
	}
	if !strings.Contains(err.Error(), "delivered=3 of 10") {
		t.Errorf("expected diag in error message, got %q", err.Error())
	}
}

func TestAwaitOrTimeoutZeroMeansForever(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(done)
	}()
	if err := awaitOrTimeout("delivery", 0, done, nil); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestAwaitWaitGroup(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
	}()
	if err := awaitWaitGroup("publishers", time.Second, &wg, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestRunBoundedCleanupReturnsErr(t *testing.T) {
	t.Parallel()
	sentinel := xerrors.New("close failed")
	err := runBoundedCleanup("close", time.Second, func() error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel, got %v", err)
	}
}

func TestRunBoundedCleanupTimesOut(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	defer close(release)
	err := runBoundedCleanup("close", 10*time.Millisecond, func() error {
		<-release
		return nil
	})
	if err == nil {
		t.Fatalf("expected cleanup timeout, got nil")
	}
	var pe *phaseTimeoutError
	if !errors.As(err, &pe) {
		t.Fatalf("expected phaseTimeoutError, got %T", err)
	}
}

func TestPublishPhaseDiagFormatting(t *testing.T) {
	t.Parallel()
	d := publishPhaseDiag{
		published: 100, expectPublished: 1000,
		delivered: 50, expectDelivered: 4000,
		drops:       3,
		firstPubErr: xerrors.New("write: broken pipe"),
		firstSubErr: xerrors.New("listener closed"),
	}
	s := d.String()
	for _, want := range []string{
		"published=100/1000", "delivered=50/4000", "drops=3",
		"write: broken pipe", "listener closed",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q in diag %q", want, s)
		}
	}
}

func TestDumpStacksContainsThisGoroutine(t *testing.T) {
	t.Parallel()
	dump := dumpStacks()
	if len(dump) == 0 {
		t.Fatalf("empty stack dump")
	}
	if !strings.Contains(string(dump), "TestDumpStacksContainsThisGoroutine") {
		t.Errorf("stack dump did not contain current goroutine: %s", string(dump[:min(2000, len(dump))]))
	}
}
