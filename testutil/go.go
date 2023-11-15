package testutil

import (
	"context"
	"testing"
)

// Go runs fn in a goroutine and waits until fn has completed before
// test completion. Done is returned for optionally waiting for fn to
// exit.
func Go(t *testing.T, fn func()) (done <-chan struct{}) {
	t.Helper()

	doneC := make(chan struct{})
	t.Cleanup(func() {
		<-doneC
	})
	go func() {
		fn()
		close(doneC)
	}()

	return doneC
}

// GoContext runs fn in a goroutine passing a context that will be
// canceled on test completion and wait until fn has finished executing.
// Done and cancel are returned for optionally waiting until completion
// or early cancellation.
func GoContext(t *testing.T, fn func(context.Context)) (done <-chan struct{}, cancel context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	doneC := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
	})
	go func() {
		fn(ctx)
		close(doneC)
	}()

	return doneC, cancel
}
