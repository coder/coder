package chatretry_test

import (
	"context"
	"fmt"
	"testing"

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
