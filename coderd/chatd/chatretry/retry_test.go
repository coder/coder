package chatretry_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatretry"
)

func TestRetry_SuccessOnFirstTry(t *testing.T) {
	t.Parallel()

	calls := 0
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		calls++
		return nil
	}, nil)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected fn called once, got %d", calls)
	}
}

func TestRetry_TransientThenSuccess(t *testing.T) {
	t.Parallel()

	calls := 0
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		calls++
		if calls == 1 {
			return xerrors.New("service unavailable")
		}
		return nil
	}, nil)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected fn called twice, got %d", calls)
	}
}

func TestRetry_MultipleTransientThenSuccess(t *testing.T) {
	t.Parallel()

	calls := 0
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		calls++
		if calls <= 3 {
			return xerrors.New("overloaded")
		}
		return nil
	}, nil)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 4 {
		t.Fatalf("expected fn called 4 times, got %d", calls)
	}
}

func TestRetry_NonRetryableError(t *testing.T) {
	t.Parallel()

	calls := 0
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		calls++
		return xerrors.New("invalid api key")
	}, nil)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "invalid api key" {
		t.Fatalf("expected 'invalid api key', got %q", err.Error())
	}
	if calls != 1 {
		t.Fatalf("expected fn called once, got %d", calls)
	}
}

func TestRetry_ContextCanceledDuringWait(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := chatretry.Retry(ctx, func(_ context.Context) error {
		calls++
		// Cancel after the first retryable error so the wait
		// select picks up the cancellation.
		if calls == 1 {
			cancel()
		}
		return xerrors.New("overloaded")
	}, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRetry_ContextCanceledDuringFn(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	err := chatretry.Retry(ctx, func(_ context.Context) error {
		cancel()
		// Return a retryable error; the loop should detect that
		// ctx is done and return the context error.
		return xerrors.New("overloaded")
	}, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRetry_OnRetryCalledWithCorrectArgs(t *testing.T) {
	t.Parallel()

	type retryRecord struct {
		attempt int
		errMsg  string
		delay   time.Duration
	}
	var records []retryRecord

	calls := 0
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		calls++
		if calls <= 2 {
			return xerrors.New("rate limit exceeded")
		}
		return nil
	}, func(attempt int, err error, delay time.Duration) {
		records = append(records, retryRecord{
			attempt: attempt,
			errMsg:  err.Error(),
			delay:   delay,
		})
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 onRetry calls, got %d", len(records))
	}
	if records[0].attempt != 1 {
		t.Errorf("first onRetry attempt = %d, want 1", records[0].attempt)
	}
	if records[1].attempt != 2 {
		t.Errorf("second onRetry attempt = %d, want 2", records[1].attempt)
	}
	if records[0].errMsg != "rate limit exceeded" {
		t.Errorf("first onRetry error = %q, want 'rate limit exceeded'", records[0].errMsg)
	}
}

func TestRetry_OnRetryNilDoesNotPanic(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		if calls.Add(1) == 1 {
			return xerrors.New("overloaded")
		}
		return nil
	}, nil)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
