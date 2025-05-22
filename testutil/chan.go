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
func TryReceive[A any](ctx context.Context, t testing.TB, c <-chan A) A {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal("timeout")
		var a A
		return a
	case a := <-c:
		return a
	}
}

// RequireReceive will receive a value from the chan and return it. If the
// context expires or the channel is closed before a value can be received,
// it will fail the test.
//
// Safety: Must only be called from the Go routine that created `t`.
func RequireReceive[A any](ctx context.Context, t testing.TB, c <-chan A) A {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal("timeout")
		var a A
		return a
	case a, ok := <-c:
		if !ok {
			t.Fatal("channel closed")
		}
		return a
	}
}

// RequireSend will send the given value over the chan and then return. If
// the context expires before the send succeeds, it will fail the test.
//
// Safety: Must only be called from the Go routine that created `t`.
func RequireSend[A any](ctx context.Context, t testing.TB, c chan<- A, a A) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal("timeout")
	case c <- a:
		// OK!
	}
}

// SoftTryReceive will attempt to receive a value from the chan and return it. If
// the context expires before a value can be received, it will mark the test as
// failed but continue execution. If the channel is closed, the zero value of the
// channel type will be returned.
// The second return value indicates whether the receive was successful. In
// particular, if the channel is closed, the second return value will be true.
//
// Safety: can be called from any goroutine.
func SoftTryReceive[A any](ctx context.Context, t testing.TB, c <-chan A) (A, bool) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Error("timeout")
		var a A
		return a, false
	case a := <-c:
		return a, true
	}
}

// AssertReceive will receive a value from the chan and return it. If the
// context expires or the channel is closed before a value can be received,
// it will mark the test as failed but continue execution.
// The second return value indicates whether the receive was successful.
//
// Safety: can be called from any goroutine.
func AssertReceive[A any](ctx context.Context, t testing.TB, c <-chan A) (A, bool) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Error("timeout")
		var a A
		return a, false
	case a, ok := <-c:
		if !ok {
			t.Error("channel closed")
		}
		return a, ok
	}
}

// AssertSend will send the given value over the chan and then return. If
// the context expires before the send succeeds, it will mark the test as failed
// but continue execution.
// The second return value indicates whether the send was successful.
//
// Safety: can be called from any goroutine.
func AssertSend[A any](ctx context.Context, t testing.TB, c chan<- A, a A) bool {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Error("timeout")
		return false
	case c <- a:
		return true
	}
}
