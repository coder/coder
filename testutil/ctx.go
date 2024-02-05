package testutil

import (
	"context"
	"testing"
	"time"
)

func Context(t *testing.T, dur time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), dur)
	t.Cleanup(cancel)
	return ctx
}

func RequireRecvCtx[A any](ctx context.Context, t testing.TB, c <-chan A) (a A) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal("timeout")
		return a
	case a = <-c:
		return a
	}
}

func RequireSendCtx[A any](ctx context.Context, t testing.TB, c chan<- A, a A) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal("timeout")
	case c <- a:
		// OK!
	}
}
