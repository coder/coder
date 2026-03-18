package chatretry_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		message    string
		kind       string
		provider   string
		retryable  bool
		statusCode int
	}{
		{
			name:       "AnthropicOverloaded",
			err:        xerrors.New("anthropic API error: status 529 overloaded_error: Overloaded"),
			message:    "Anthropic is temporarily overloaded (HTTP 529). Please try again later.",
			kind:       "overloaded",
			provider:   "anthropic",
			retryable:  true,
			statusCode: 529,
		},
		{
			name:       "RateLimitByStatusCode",
			err:        xerrors.New("received status 429 from upstream"),
			message:    "The AI provider is rate limiting requests (HTTP 429). Please try again later.",
			kind:       "rate_limit",
			provider:   "",
			retryable:  true,
			statusCode: 429,
		},
		{
			name:       "Timeout",
			err:        xerrors.New("dial tcp: i/o timeout"),
			message:    "The AI provider did not respond in time. Please try again.",
			kind:       "timeout",
			provider:   "",
			retryable:  true,
			statusCode: 0,
		},
		{
			name:       "Auth",
			err:        xerrors.New("invalid api key"),
			message:    "Authentication with the AI provider failed. Check the API key, permissions, and billing settings.",
			kind:       "auth",
			provider:   "",
			retryable:  false,
			statusCode: 401,
		},
		{
			name:       "Config",
			err:        xerrors.New("model not found"),
			message:    "The AI provider rejected the model configuration. Check the selected model and provider settings.",
			kind:       "config",
			provider:   "",
			retryable:  false,
			statusCode: 0,
		},
		{
			name:       "Generic",
			err:        xerrors.New("boom"),
			message:    "The chat request failed unexpectedly. Please try again.",
			kind:       "generic",
			provider:   "",
			retryable:  false,
			statusCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			classified := chatretry.ClassifyError(tt.err)
			require.Equal(t, tt.message, classified.Message)
			require.Equal(t, tt.kind, classified.Kind)
			require.Equal(t, tt.provider, classified.Provider)
			require.Equal(t, tt.retryable, classified.Retryable)
			require.Equal(t, tt.statusCode, classified.StatusCode)
		})
	}
}

func TestClassifyError_UsesWrappedClassification(t *testing.T) {
	t.Parallel()

	classified := chatretry.ClassifyError(
		xerrors.New("received status 429 from upstream"),
	).WithProvider("openai")
	wrapped := chatretry.WithClassification(
		xerrors.New("max retry attempts exceeded: received status 429 from upstream"),
		classified,
	)

	require.Equal(t, classified, chatretry.ClassifyError(wrapped))
}

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
			retryable: true,
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
			classified := chatretry.ClassifyError(tt.err)
			got := chatretry.IsRetryable(tt.err)
			if got != tt.retryable {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.retryable)
			}
			if classified.Retryable != got {
				t.Errorf(
					"ClassifyError(%v).Retryable = %v, want %v",
					tt.err,
					classified.Retryable,
					got,
				)
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
		{408, true},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
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
		attempt    int
		errMsg     string
		message    string
		kind       string
		provider   string
		retryable  bool
		statusCode int
		delay      time.Duration
	}
	var records []retryRecord

	calls := 0
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		calls++
		if calls <= 2 {
			return xerrors.New("received status 429 from upstream")
		}
		return nil
	}, func(
		attempt int,
		err error,
		classified chatretry.ClassifiedError,
		delay time.Duration,
	) {
		records = append(records, retryRecord{
			attempt:    attempt,
			errMsg:     err.Error(),
			message:    classified.Message,
			kind:       classified.Kind,
			provider:   classified.Provider,
			retryable:  classified.Retryable,
			statusCode: classified.StatusCode,
			delay:      delay,
		})
	})
	require.NoError(t, err)
	require.Len(t, records, 2)

	require.Equal(t, 1, records[0].attempt)
	require.Equal(t, 2, records[1].attempt)
	require.Equal(t, "received status 429 from upstream", records[0].errMsg)
	require.Equal(
		t,
		"The AI provider is rate limiting requests (HTTP 429). Please try again later.",
		records[0].message,
	)
	require.Equal(t, "rate_limit", records[0].kind)
	require.Empty(t, records[0].provider)
	require.True(t, records[0].retryable)
	require.Equal(t, 429, records[0].statusCode)
	require.Equal(t, chatretry.Delay(0), records[0].delay)
	require.Equal(t, chatretry.Delay(1), records[1].delay)
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
