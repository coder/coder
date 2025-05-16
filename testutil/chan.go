package testutil

import (
	"context"
	"testing"
)

// TryReceive will attempt to receive a value from the chan and return it. If
// the context expires before a value can be received, it will fail the test. If
// the channel is closed, the zero value of the channel type will be returned.
//
// Safety: Must only be called from the Go routine that created `t`.
func TryReceive[A any](ctx context.Context, t testing.TB, c <-chan A) (a A) {
	t.Helper()
	select {
	case <-t.Context().Done():
		t.Fatal("test timeout")
	case <-ctx.Done():
		t.Fatal("context timeout")
	case a = <-c:
	}
	return a
}

// RequireReceive will receive a value from the chan and return it. If the
// context expires or the channel is closed before a value can be received,
// it will fail the test.
//
// Safety: Must only be called from the Go routine that created `t`.
func RequireReceive[A any](ctx context.Context, t testing.TB, c <-chan A) (a A) {
	t.Helper()
	var ok bool
	select {
	case <-t.Context().Done():
		t.Fatal("test timeout")
	case <-ctx.Done():
		t.Fatal("context timeout")
	case a, ok = <-c:
		if !ok {
			t.Fatal("channel closed")
		}
	}
	return a
}

// RequireSend will send the given value over the chan and then return. If
// the context expires before the send succeeds, it will fail the test.
//
// Safety: Must only be called from the Go routine that created `t`.
func RequireSend[A any](ctx context.Context, t testing.TB, c chan<- A, a A) {
	t.Helper()
	select {
	case <-t.Context().Done():
		t.Fatal("test timeout")
	case <-ctx.Done():
		t.Fatal("context timeout")
	case c <- a:
		// OK!
	}
}
