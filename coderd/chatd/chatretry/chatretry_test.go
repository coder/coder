package chatretry_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatretry"
)

func TestIsRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		// Retryable errors.
		{
			name:      "Overloaded",
			err:       xerrors.New("model is overloaded, please try again"),
			retryable: true,
		},
		{
			name:      "RateLimit",
			err:       xerrors.New("rate limit exceeded"),
			retryable: true,
		},
		{
			name:      "RateLimitUnderscore",
			err:       xerrors.New("rate_limit: too many requests"),
			retryable: true,
		},
		{
			name:      "TooManyRequests",
			err:       xerrors.New("too many requests"),
			retryable: true,
		},
		{
			name:      "HTTP429InMessage",
			err:       xerrors.New("received status 429 from upstream"),
			retryable: false, // "429" alone is not a pattern; needs matching text.
		},
		{
			name:      "HTTP529InMessage",
			err:       xerrors.New("received status 529 from upstream"),
			retryable: true,
		},
		{
			name:      "ServerError500",
			err:       xerrors.New("status 500: internal server error"),
			retryable: true,
		},
		{
			name:      "ServerErrorGeneric",
			err:       xerrors.New("server error"),
			retryable: true,
		},
		{
			name:      "ConnectionReset",
			err:       xerrors.New("read tcp: connection reset by peer"),
			retryable: true,
		},
		{
			name:      "ConnectionRefused",
			err:       xerrors.New("dial tcp: connection refused"),
			retryable: true,
		},
		{
			name:      "EOF",
			err:       xerrors.New("unexpected EOF"),
			retryable: true,
		},
		{
			name:      "BrokenPipe",
			err:       xerrors.New("write: broken pipe"),
			retryable: true,
		},
		{
			name:      "NetworkTimeout",
			err:       xerrors.New("i/o timeout"),
			retryable: true,
		},
		{
			name:      "ServiceUnavailable",
			err:       xerrors.New("service unavailable"),
			retryable: true,
		},
		{
			name:      "Unavailable",
			err:       xerrors.New("the service is currently unavailable"),
			retryable: true,
		},
		{
			name:      "Status502",
			err:       xerrors.New("status 502: bad gateway"),
			retryable: true,
		},
		{
			name:      "Status503",
			err:       xerrors.New("status 503"),
			retryable: true,
		},

		// Non-retryable errors.
		{
			name:      "Nil",
			err:       nil,
			retryable: false,
		},
		{
			name:      "ContextCanceled",
			err:       context.Canceled,
			retryable: false,
		},
		{
			name:      "ContextCanceledWrapped",
			err:       xerrors.Errorf("operation failed: %w", context.Canceled),
			retryable: false,
		},
		{
			name:      "ContextCanceledMessage",
			err:       xerrors.New("context canceled"),
			retryable: false,
		},
		{
			name:      "ContextDeadlineExceeded",
			err:       xerrors.New("context deadline exceeded"),
			retryable: false,
		},
		{
			name:      "Authentication",
			err:       xerrors.New("authentication failed"),
			retryable: false,
		},
		{
			name:      "Unauthorized",
			err:       xerrors.New("401 Unauthorized"),
			retryable: false,
		},
		{
			name:      "Forbidden",
			err:       xerrors.New("403 Forbidden"),
			retryable: false,
		},
		{
			name:      "InvalidAPIKey",
			err:       xerrors.New("invalid api key"),
			retryable: false,
		},
		{
			name:      "InvalidAPIKeyUnderscore",
			err:       xerrors.New("invalid_api_key"),
			retryable: false,
		},
		{
			name:      "InvalidModel",
			err:       xerrors.New("invalid model: gpt-5-turbo"),
			retryable: false,
		},
		{
			name:      "ModelNotFound",
			err:       xerrors.New("model not found"),
			retryable: false,
		},
		{
			name:      "ModelNotFoundUnderscore",
			err:       xerrors.New("model_not_found"),
			retryable: false,
		},
		{
			name:      "ContextLengthExceeded",
			err:       xerrors.New("context length exceeded"),
			retryable: false,
		},
		{
			name:      "ContextExceededUnderscore",
			err:       xerrors.New("context_exceeded"),
			retryable: false,
		},
		{
			name:      "MaximumContextLength",
			err:       xerrors.New("maximum context length"),
			retryable: false,
		},
		{
			name:      "QuotaExceeded",
			err:       xerrors.New("quota exceeded"),
			retryable: false,
		},
		{
			name:      "BillingError",
			err:       xerrors.New("billing issue: payment required"),
			retryable: false,
		},

		// Wrapped errors preserve retryability.
		{
			name:      "WrappedRetryable",
			err:       xerrors.Errorf("provider call failed: %w", xerrors.New("service unavailable")),
			retryable: true,
		},
		{
			name:      "WrappedNonRetryable",
			err:       xerrors.Errorf("provider call failed: %w", xerrors.New("invalid api key")),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chatretry.IsRetryable(tt.err)
			if got != tt.retryable {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.retryable)
			}
		})
	}
}

func TestStatusCodeRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code      int
		retryable bool
	}{
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{529, true},
		{200, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Status%d", tt.code), func(t *testing.T) {
			t.Parallel()
			got := chatretry.StatusCodeRetryable(tt.code)
			if got != tt.retryable {
				t.Errorf("StatusCodeRetryable(%d) = %v, want %v", tt.code, got, tt.retryable)
			}
		})
	}
}

func TestDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
		{6, 60 * time.Second},  // Capped at MaxDelay.
		{10, 60 * time.Second}, // Still capped.
		{100, 60 * time.Second},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Attempt%d", tt.attempt), func(t *testing.T) {
			t.Parallel()
			got := chatretry.Delay(tt.attempt)
			if got != tt.want {
				t.Errorf("Delay(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

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
