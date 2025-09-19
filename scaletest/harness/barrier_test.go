package harness_test

import (
	"sync"
	"testing"

	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/testutil"
)

func TestBarrier_MultipleRunners(t *testing.T) {
	t.Parallel()
	const numRunners = 3

	ctx := testutil.Context(t, testutil.WaitShort)
	barrier := harness.NewBarrier(numRunners)

	var wg sync.WaitGroup
	wg.Add(numRunners)

	done := make(chan struct{})

	for range numRunners {
		go func() {
			defer wg.Done()
			barrier.Wait()
		}()
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-ctx.Done():
		t.Fatal("barrier should have released all runners")
	}
}

func TestBarrier_Cancel(t *testing.T) {
	t.Parallel()
	const numRunners = 3

	ctx := testutil.Context(t, testutil.WaitShort)
	barrier := harness.NewBarrier(numRunners)

	var wg sync.WaitGroup
	wg.Add(numRunners - 1)

	done := make(chan struct{})

	for range numRunners - 1 {
		go func() {
			defer wg.Done()
			barrier.Wait()
		}()
	}

	barrier.Cancel()

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-ctx.Done():
		t.Fatal("barrier should have released after cancel")
	}
}
