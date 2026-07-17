package chaterror_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want chaterror.ClassifiedError
	}{
		{
			name: "AmbiguousOverloadKeepsProviderUnknown",
			err:  xerrors.New("status 529 from upstream"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider is temporarily overloaded.",
				Detail:     "status 529 from upstream",
				Kind:       codersdk.ChatErrorKindOverloaded,
				Provider:   "",
				Retryable:  true,
				StatusCode: 529,
			},
		},
		{
			name: "ExplicitAnthropicOverload",
			err:  xerrors.New("anthropic overloaded_error"),
			want: chaterror.ClassifiedError{
				Message:    "Anthropic is temporarily overloaded.",
				Detail:     "anthropic overloaded_error",
				Kind:       codersdk.ChatErrorKindOverloaded,
				Provider:   "anthropic",
				Retryable:  true,
				StatusCode: 0,
			},
		},
		{
			name: "AnthropicMissingMessageStop",
			err: xerrors.Errorf(
				"anthropic stream closed before message_stop: %w",
				io.EOF,
			),
			want: chaterror.ClassifiedError{
				Message:    "Anthropic stream closed unexpectedly before the response completed.",
				Kind:       codersdk.ChatErrorKindTimeout,
				Provider:   "anthropic",
				Retryable:  true,
				StatusCode: 0,
			},
		},
		{
			name: "OpenAIResponsesMissingTerminalEvent",
			err: xerrors.Errorf(
				"openai responses stream closed before terminal event: %w",
				io.EOF,
			),
			want: chaterror.ClassifiedError{
				Message:    "OpenAI stream closed unexpectedly before the response completed.",
				Kind:       codersdk.ChatErrorKindTimeout,
				Provider:   "openai",
				Retryable:  true,
				StatusCode: 0,
			},
		},
		{
			name: "AuthBeatsConfig",
			err:  xerrors.New("authentication failed: invalid model"),
			want: chaterror.ClassifiedError{
				Message:    "Authentication with the AI provider failed. Check the API key and permissions.",
				Kind:       codersdk.ChatErrorKindAuth,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "PureConfig",
			err:  xerrors.New("invalid model"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider rejected the model configuration. Check the selected model and provider settings.",
				Detail:     "invalid model",
				Kind:       codersdk.ChatErrorKindConfig,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "BareForbiddenClassifiesAsAuth",
			err:  xerrors.New("forbidden"),
			want: chaterror.ClassifiedError{
				Message:    "Authentication with the AI provider failed. Check the API key and permissions.",
				Kind:       codersdk.ChatErrorKindAuth,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "ExplicitStatus401ClassifiesAsAuth",
			err:  xerrors.New("status 401 from upstream"),
			want: chaterror.ClassifiedError{
				Message:    "Authentication with the AI provider failed. Check the API key and permissions.",
				Kind:       codersdk.ChatErrorKindAuth,
				Provider:   "",
				Retryable:  false,
				StatusCode: 401,
			},
		},
		{
			name: "ExplicitStatus403ClassifiesAsAuth",
			err:  xerrors.New("status 403 from upstream"),
			want: chaterror.ClassifiedError{
				Message:    "Authentication with the AI provider failed. Check the API key and permissions.",
				Kind:       codersdk.ChatErrorKindAuth,
				Provider:   "",
				Retryable:  false,
				StatusCode: 403,
			},
		},
		{
			name: "ForbiddenContextLengthClassifiesAsConfig",
			err:  xerrors.New("forbidden: context length exceeded"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider rejected the model configuration. Check the selected model and provider settings.",
				Detail:     "forbidden: context length exceeded",
				Kind:       codersdk.ChatErrorKindConfig,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "ExplicitStatus429ClassifiesAsRateLimit",
			err:  xerrors.New("status 429 from upstream"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider is rate limiting requests.",
				Detail:     "status 429 from upstream",
				Kind:       codersdk.ChatErrorKindRateLimit,
				Provider:   "",
				Retryable:  true,
				StatusCode: 429,
			},
		},
		{
			name: "RateLimitDoesNotBeatConfig",
			err:  xerrors.New("status 429: invalid model"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider rejected the model configuration. Check the selected model and provider settings.",
				Detail:     "status 429: invalid model",
				Kind:       codersdk.ChatErrorKindConfig,
				Provider:   "",
				Retryable:  false,
				StatusCode: 429,
			},
		},
		{
			name: "UsageLimitPatternDoesNotBeatConfigWith429",
			err:  xerrors.New("status 429: invalid model quota"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider rejected the model configuration. Check the selected model and provider settings.",
				Detail:     "status 429: invalid model quota",
				Kind:       codersdk.ChatErrorKindConfig,
				Provider:   "",
				Retryable:  false,
				StatusCode: 429,
			},
		},
		{
			name: "ServiceUnavailableClassifiesAsRetryableTimeout",
			err:  xerrors.New("service unavailable"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider is temporarily unavailable.",
				Detail:     "service unavailable",
				Kind:       codersdk.ChatErrorKindTimeout,
				Provider:   "",
				Retryable:  true,
				StatusCode: 0,
			},
		},
		{
			name: "TimeoutDoesNotBeatConfigViaStatusCode",
			err:  xerrors.New("status 503: invalid model"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider rejected the model configuration. Check the selected model and provider settings.",
				Detail:     "status 503: invalid model",
				Kind:       codersdk.ChatErrorKindConfig,
				Provider:   "",
				Retryable:  false,
				StatusCode: 503,
			},
		},
		{
			name: "TimeoutDoesNotBeatConfigViaMessage",
			err:  xerrors.New("service unavailable: model not found"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider rejected the model configuration. Check the selected model and provider settings.",
				Detail:     "service unavailable: model not found",
				Kind:       codersdk.ChatErrorKindConfig,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "ConnectionRefusedUnsupportedModelClassifiesAsConfig",
			err:  xerrors.New("connection refused: unsupported model"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider rejected the model configuration. Check the selected model and provider settings.",
				Detail:     "connection refused: unsupported model",
				Kind:       codersdk.ChatErrorKindConfig,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "DeadlineExceededStaysNonRetryableTimeout",
			err:  context.DeadlineExceeded,
			want: chaterror.ClassifiedError{
				Message:    "The request timed out before it completed.",
				Detail:     "context deadline exceeded",
				Kind:       codersdk.ChatErrorKindTimeout,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "ProviderTransportResetIsRetryable",
			err:  errors.Join(chaterror.ErrProviderTransportReset, context.Canceled),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider is temporarily unavailable.",
				Detail:     "provider transport reset context canceled",
				Kind:       codersdk.ChatErrorKindTimeout,
				Provider:   "",
				Retryable:  true,
				StatusCode: 0,
			},
		},
		{
			name: "BareContextCanceledStaysNonRetryable",
			err:  context.Canceled,
			want: chaterror.ClassifiedError{
				Message:    "The request was canceled before it completed.",
				Kind:       codersdk.ChatErrorKindGeneric,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "Status500ContextCanceledClassifiesAsRetryable",
			err:  xerrors.Errorf("received status 500 from upstream: %w", context.Canceled),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider returned an unexpected error.",
				Detail:     "received status 500 from upstream: context canceled",
				Kind:       codersdk.ChatErrorKindGeneric,
				Provider:   "",
				Retryable:  true,
				StatusCode: http.StatusInternalServerError,
			},
		},
		{
			name: "ProviderStatus500ContextCanceledClassifiesAsRetryable",
			err: xerrors.Errorf("provider stream closed: %w", errors.Join(
				context.Canceled,
				&fantasy.ProviderError{
					Message:    "context canceled",
					StatusCode: http.StatusInternalServerError,
				},
			)),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider returned an unexpected error.",
				Detail:     "context canceled",
				Kind:       codersdk.ChatErrorKindGeneric,
				Provider:   "",
				Retryable:  true,
				StatusCode: http.StatusInternalServerError,
			},
		},
		// The next cases model the error that fantasy produces
		// when aibridge's disabledProviderHandler returns a 503
		// plain-text sentinel. Fantasy sets Title from the HTTP
		// status text and Message from the response body (including
		// the trailing newline written by http.Error).
		{
			name: "ProviderDisabled503ClassifiesAsProviderDisabled",
			err: &fantasy.ProviderError{
				Title:      fantasy.ErrorTitleForStatusCode(http.StatusServiceUnavailable),
				Message:    fmt.Sprintf("%s: AI provider %q is disabled\n", codersdk.ChatErrorKindProviderDisabled, "openai"),
				StatusCode: http.StatusServiceUnavailable,
			},
			want: chaterror.ClassifiedError{
				Message:    "The OpenAI provider has been disabled. Contact your Coder administrator.",
				Detail:     fmt.Sprintf("%s: AI provider %q is disabled", codersdk.ChatErrorKindProviderDisabled, "openai"),
				Kind:       codersdk.ChatErrorKindProviderDisabled,
				Provider:   "openai",
				Retryable:  false,
				StatusCode: 503,
			},
		},
		{
			name: "ProviderDisabled503UnknownProvider",
			err: &fantasy.ProviderError{
				Title:      fantasy.ErrorTitleForStatusCode(http.StatusServiceUnavailable),
				Message:    fmt.Sprintf("%s: AI provider %q is disabled\n", codersdk.ChatErrorKindProviderDisabled, "mycustomprovider"),
				StatusCode: http.StatusServiceUnavailable,
			},
			want: chaterror.ClassifiedError{
				Message:    "The AI provider has been disabled. Contact your Coder administrator.",
				Detail:     fmt.Sprintf("%s: AI provider %q is disabled", codersdk.ChatErrorKindProviderDisabled, "mycustomprovider"),
				Kind:       codersdk.ChatErrorKindProviderDisabled,
				Provider:   "",
				Retryable:  false,
				StatusCode: 503,
			},
		},
		{
			name: "ProviderDisabledPlainErrorString",
			err:  xerrors.New(fmt.Sprintf("%s: AI provider %q is disabled", codersdk.ChatErrorKindProviderDisabled, "anthropic")),
			want: chaterror.ClassifiedError{
				Message:    "The Anthropic provider has been disabled. Contact your Coder administrator.",
				Detail:     fmt.Sprintf("%s: AI provider %q is disabled", codersdk.ChatErrorKindProviderDisabled, "anthropic"),
				Kind:       codersdk.ChatErrorKindProviderDisabled,
				Provider:   "anthropic",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "ProviderDisabledBeatsTimeout503",
			err: &fantasy.ProviderError{
				Title:      fantasy.ErrorTitleForStatusCode(http.StatusServiceUnavailable),
				Message:    fmt.Sprintf("%s: AI provider %q is disabled\n", codersdk.ChatErrorKindProviderDisabled, "google"),
				StatusCode: http.StatusServiceUnavailable,
			},
			want: chaterror.ClassifiedError{
				Message:    "The Google provider has been disabled. Contact your Coder administrator.",
				Detail:     fmt.Sprintf("%s: AI provider %q is disabled", codersdk.ChatErrorKindProviderDisabled, "google"),
				Kind:       codersdk.ChatErrorKindProviderDisabled,
				Provider:   "google",
				Retryable:  false,
				StatusCode: 503,
			},
		},
		{
			name: "Generic503StillClassifiesAsTimeout",
			err: &fantasy.ProviderError{
				Message:    "service unavailable",
				StatusCode: 503,
			},
			want: chaterror.ClassifiedError{
				Message:    "The AI provider is temporarily unavailable.",
				Detail:     "service unavailable",
				Kind:       codersdk.ChatErrorKindTimeout,
				Provider:   "",
				Retryable:  true,
				StatusCode: 503,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chaterror.Classify(tt.err))
		})
	}
}

func TestClassify_OpenAIResponsesAPIDiagnostics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          string
		responseBody string
		wantDetail   string
		forbidden    []string
	}{
		{
			name:         "FunctionCallOutputMissing",
			err:          "No Tool Output Found For Function Call call_sensitive123",
			responseBody: `{"error":{"message":"No tool output found for function call call_sensitive123"}}`,
			wantDetail:   "OpenAI Responses API request continuity diagnostic: match=function_call_output_missing.",
			forbidden:    []string{"call_sensitive123"},
		},
		{
			name:         "WebSearchReasoningMissing",
			err:          "Item 'ws_sensitive123' of type 'web_search_call' WAS PROVIDED WITHOUT ITS REQUIRED 'reasoning' item: 'rs_sensitive123'",
			responseBody: `{"error":{"message":"Item 'ws_sensitive123' of type 'web_search_call' was provided without its required 'reasoning' item: 'rs_sensitive123'"}}`,
			wantDetail:   "OpenAI Responses API request continuity diagnostic: match=web_search_reasoning_missing.",
			forbidden:    []string{"ws_sensitive123", "rs_sensitive123"},
		},
	}

	assertNoLeak := func(t *testing.T, classified chaterror.ClassifiedError, forbidden []string) {
		t.Helper()
		for _, value := range forbidden {
			require.NotContains(t, classified.Message, value)
			require.NotContains(t, classified.Detail, value)
		}
	}

	assertDirectionalMessage := func(t *testing.T, message string) {
		t.Helper()
		require.Contains(t, message, "chat continuation")
		require.Contains(t, message, "internal state mismatch")
		require.Contains(t, message, "not a configuration or billing issue")
	}

	for _, tt := range tests {
		t.Run(tt.name+"/BareString", func(t *testing.T) {
			t.Parallel()

			classified := chaterror.Classify(xerrors.New(tt.err))
			require.Equal(t, codersdk.ChatErrorKindGeneric, classified.Kind)
			require.False(t, classified.Retryable)
			require.Zero(t, classified.StatusCode)
			assertDirectionalMessage(t, classified.Message)
			require.Equal(t, tt.wantDetail, classified.Detail)
			assertNoLeak(t, classified, tt.forbidden)
		})

		t.Run(tt.name+"/WrappedProviderError", func(t *testing.T) {
			t.Parallel()

			classified := chaterror.Classify(xerrors.Errorf(
				"provider request failed: %w",
				testProviderError(
					"",
					400,
					nil,
					testProviderResponseDump(tt.responseBody),
				),
			))
			require.Equal(t, codersdk.ChatErrorKindGeneric, classified.Kind)
			require.False(t, classified.Retryable)
			require.Equal(t, 400, classified.StatusCode)
			assertDirectionalMessage(t, classified.Message)
			require.Equal(t, tt.wantDetail, classified.Detail)
			assertNoLeak(t, classified, tt.forbidden)
		})
	}
}

func TestClassify_PatternCoverage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       string
		wantKind  codersdk.ChatErrorKind
		wantRetry bool
	}{
		{name: "OverloadedLiteral", err: "overloaded", wantKind: codersdk.ChatErrorKindOverloaded, wantRetry: true},
		{name: "RateLimitLiteral", err: "rate limit", wantKind: codersdk.ChatErrorKindRateLimit, wantRetry: true},
		{name: "RateLimitUnderscoreLiteral", err: "rate_limit", wantKind: codersdk.ChatErrorKindRateLimit, wantRetry: true},
		{name: "RateLimitedLiteral", err: "rate limited", wantKind: codersdk.ChatErrorKindRateLimit, wantRetry: true},
		{name: "RateLimitedHyphenLiteral", err: "rate-limited", wantKind: codersdk.ChatErrorKindRateLimit, wantRetry: true},
		{name: "TooManyRequestsLiteral", err: "too many requests", wantKind: codersdk.ChatErrorKindRateLimit, wantRetry: true},
		{name: "TimeoutLiteral", err: "timeout", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "TimedOutLiteral", err: "timed out", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "ServiceUnavailableLiteral", err: "service unavailable", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "UnavailableLiteral", err: "unavailable", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "ConnectionResetLiteral", err: "connection reset", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "ConnectionRefusedLiteral", err: "connection refused", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "EOFLiteral", err: "eof", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "BrokenPipeLiteral", err: "broken pipe", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "BadGatewayLiteral", err: "bad gateway", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "GatewayTimeoutLiteral", err: "gateway timeout", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "ClientConnLiteral", err: "client conn", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "GOAWAYLiteral", err: "goaway", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "HTTP2StreamClosedLiteral", err: "http2: stream closed", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "UseOfClosedNetworkConnectionLiteral", err: "use of closed network connection", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "HTTP2InternalErrorReceivedFromPeerLiteral", err: "internal_error; received from peer", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "HTTP2RefusedStreamReceivedFromPeerLiteral", err: "refused_stream; received from peer", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "HTTP2CancelReceivedFromPeerLiteral", err: "cancel; received from peer", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "HTTP2EnhanceYourCalmReceivedFromPeerLiteral", err: "enhance_your_calm; received from peer", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "HTTP2NoErrorReceivedFromPeerLiteral", err: "no_error; received from peer", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "AuthenticationLiteral", err: "authentication", wantKind: codersdk.ChatErrorKindAuth, wantRetry: false},
		{name: "UnauthorizedLiteral", err: "unauthorized", wantKind: codersdk.ChatErrorKindAuth, wantRetry: false},
		{name: "InvalidAPIKeyLiteral", err: "invalid api key", wantKind: codersdk.ChatErrorKindAuth, wantRetry: false},
		{name: "InvalidAPIKeyUnderscoreLiteral", err: "invalid_api_key", wantKind: codersdk.ChatErrorKindAuth, wantRetry: false},
		{name: "QuotaLiteral", err: "quota", wantKind: codersdk.ChatErrorKindUsageLimit, wantRetry: false},
		{name: "BillingLiteral", err: "billing", wantKind: codersdk.ChatErrorKindUsageLimit, wantRetry: false},
		{name: "InsufficientQuotaLiteral", err: "insufficient_quota", wantKind: codersdk.ChatErrorKindUsageLimit, wantRetry: false},
		{name: "PaymentRequiredLiteral", err: "payment required", wantKind: codersdk.ChatErrorKindUsageLimit, wantRetry: false},
		{name: "ForbiddenLiteral", err: "forbidden", wantKind: codersdk.ChatErrorKindAuth, wantRetry: false},
		{name: "InvalidModelLiteral", err: "invalid model", wantKind: codersdk.ChatErrorKindConfig, wantRetry: false},
		{name: "ModelNotFoundLiteral", err: "model not found", wantKind: codersdk.ChatErrorKindConfig, wantRetry: false},
		{name: "ModelNotFoundUnderscoreLiteral", err: "model_not_found", wantKind: codersdk.ChatErrorKindConfig, wantRetry: false},
		{name: "UnsupportedModelLiteral", err: "unsupported model", wantKind: codersdk.ChatErrorKindConfig, wantRetry: false},
		{name: "ContextLengthExceededLiteral", err: "context length exceeded", wantKind: codersdk.ChatErrorKindConfig, wantRetry: false},
		{name: "ContextExceededLiteral", err: "context_exceeded", wantKind: codersdk.ChatErrorKindConfig, wantRetry: false},
		{name: "MaximumContextLengthLiteral", err: "maximum context length", wantKind: codersdk.ChatErrorKindConfig, wantRetry: false},
		{name: "MalformedConfigLiteral", err: "malformed config", wantKind: codersdk.ChatErrorKindConfig, wantRetry: false},
		{name: "MalformedConfigurationLiteral", err: "malformed configuration", wantKind: codersdk.ChatErrorKindConfig, wantRetry: false},
		{name: "ServerErrorLiteral", err: "server error", wantKind: codersdk.ChatErrorKindGeneric, wantRetry: true},
		{name: "InternalServerErrorLiteral", err: "internal server error", wantKind: codersdk.ChatErrorKindGeneric, wantRetry: true},
		{name: "ChatInterruptedLiteral", err: "chat interrupted", wantKind: codersdk.ChatErrorKindGeneric, wantRetry: false},
		{name: "RequestInterruptedLiteral", err: "request interrupted", wantKind: codersdk.ChatErrorKindGeneric, wantRetry: false},
		{name: "OperationInterruptedLiteral", err: "operation interrupted", wantKind: codersdk.ChatErrorKindGeneric, wantRetry: false},
		{name: "Status408", err: "status 408", wantKind: codersdk.ChatErrorKindTimeout, wantRetry: true},
		{name: "Status500", err: "status 500", wantKind: codersdk.ChatErrorKindGeneric, wantRetry: true},
		{name: "ProviderDisabledLiteral", err: "provider_disabled", wantKind: codersdk.ChatErrorKindProviderDisabled, wantRetry: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			classified := chaterror.Classify(xerrors.New(tt.err))
			require.Equal(t, tt.wantKind, classified.Kind)
			require.Equal(t, tt.wantRetry, classified.Retryable)
		})
	}
}

func TestClassify_TransportFailuresUseBroaderRetryMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  string
	}{
		{name: "TimeoutLiteral", err: "timeout"},
		{name: "EOFLiteral", err: "eof"},
		{name: "BrokenPipeLiteral", err: "broken pipe"},
		{name: "ConnectionResetLiteral", err: "connection reset"},
		{name: "ConnectionRefusedLiteral", err: "connection refused"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			classified := chaterror.Classify(xerrors.New(tt.err))
			require.Equal(t, codersdk.ChatErrorKindTimeout, classified.Kind)
			require.True(t, classified.Retryable)
			require.Equal(
				t,
				"The AI provider is temporarily unavailable.",
				classified.Message,
			)
		})
	}
}

// TestClassify_HTTP2TransportErrors checks HTTP/2 transport errors
// classify as retryable ChatErrorKindTimeout. Split into two sub-tables so a
// bug in transport matching cannot be masked by provider detection
// (and vice versa).
func TestClassify_HTTP2TransportErrors(t *testing.T) {
	t.Parallel()

	// Transport patterns, no provider hint. Provider stays empty and
	// Message uses the generic subject.
	transportOnly := []struct {
		name string
		err  string
	}{
		{
			name: "HTTP2ClientConnForceClosed",
			err:  "http2: client connection force closed via ClientConn.Close",
		},
		{
			name: "HTTP2TransportGOAWAY",
			err:  "http2: Transport received Server's graceful shutdown GOAWAY",
		},
		{
			name: "HTTP2ServerGOAWAY",
			err:  "http2: server sent GOAWAY and closed the connection",
		},
		{
			name: "HTTP2StreamClosed",
			err:  "http2: stream closed",
		},
		{
			name: "HTTP2PeerInternalStreamReset",
			err:  "stream error: stream ID 455; INTERNAL_ERROR; received from peer",
		},
		{
			name: "UseOfClosedNetworkConnectionOnPOST",
			err:  `Post "https://example.com/v1/messages": use of closed network connection`,
		},
		{
			name: "HTTP2ClientConnIsClosed",
			err:  "http2: client conn is closed",
		},
		{
			name: "HTTP2ClientConnNotUsable",
			err:  "http2: client conn not usable",
		},
		{
			name: "HTTP2ClientConnNotEstablished",
			err:  "http2: client conn could not be established",
		},
		{
			name: "HTTP2ClientConnectionLost",
			err:  "http2: client connection lost",
		},
	}

	for _, tt := range transportOnly {
		t.Run("TransportOnly/"+tt.name, func(t *testing.T) {
			t.Parallel()

			classified := chaterror.Classify(xerrors.New(tt.err))
			require.Equal(t, codersdk.ChatErrorKindTimeout, classified.Kind, "Kind")
			require.True(t, classified.Retryable, "Retryable")
			require.Equal(t, "", classified.Provider, "Provider")
			require.Equal(t,
				"The AI provider is temporarily unavailable.",
				classified.Message,
				"Message",
			)
		})
	}

	// Same transport signature with a provider host in the URL so
	// detectProvider can stamp Provider.
	providerDetection := []struct {
		name        string
		err         string
		provider    string
		wantMessage string
	}{
		{
			name:        "CustomerRegressionAnthropic",
			err:         `stream response: Post "https://api.anthropic.com/v1/messages": http2: client connection force closed via ClientConn.Close`,
			provider:    "anthropic",
			wantMessage: "Anthropic is temporarily unavailable.",
		},
		{
			name:        "OpenAIForceClosed",
			err:         `stream response: Post "https://api.openai.com/v1/chat/completions": http2: client connection force closed via ClientConn.Close`,
			provider:    "openai",
			wantMessage: "OpenAI is temporarily unavailable.",
		},
		{
			name:        "AnthropicPeerInternalStreamReset",
			err:         `stream response: Post "https://api.anthropic.com/v1/messages": stream error: stream ID 455; INTERNAL_ERROR; received from peer`,
			provider:    "anthropic",
			wantMessage: "Anthropic is temporarily unavailable.",
		},
		{
			name:        "GoogleGOAWAY",
			err:         `stream response: Post "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:streamGenerateContent": http2: server sent GOAWAY and closed the connection`,
			provider:    "google",
			wantMessage: "Google is temporarily unavailable.",
		},
	}

	for _, tt := range providerDetection {
		t.Run("ProviderDetection/"+tt.name, func(t *testing.T) {
			t.Parallel()

			classified := chaterror.Classify(xerrors.New(tt.err))
			require.Equal(t, codersdk.ChatErrorKindTimeout, classified.Kind, "Kind")
			require.True(t, classified.Retryable, "Retryable")
			require.Equal(t, tt.provider, classified.Provider, "Provider")
			require.Equal(t, tt.err, classified.Detail, "Detail")
			require.Equal(t, tt.wantMessage, classified.Message, "Message")
		})
	}
}

func TestClassify_HTTP2StreamErrorValues(t *testing.T) {
	t.Parallel()

	peerReset := func(code http2.ErrCode) http2.StreamError {
		return http2.StreamError{
			StreamID: 455,
			Code:     code,
			Cause:    xerrors.New("received from peer"),
		}
	}

	retryable := []struct {
		name string
		err  error
		want chaterror.ClassifiedError
	}{
		{
			name: "Internal",
			err:  peerReset(http2.ErrCodeInternal),
			want: chaterror.ClassifiedError{
				Message:   "The AI provider is temporarily unavailable.",
				Detail:    "stream error: stream ID 455; INTERNAL_ERROR; received from peer",
				Kind:      codersdk.ChatErrorKindTimeout,
				Retryable: true,
			},
		},
		{
			name: "RefusedStream",
			err:  peerReset(http2.ErrCodeRefusedStream),
			want: chaterror.ClassifiedError{
				Message:   "The AI provider is temporarily unavailable.",
				Detail:    "stream error: stream ID 455; REFUSED_STREAM; received from peer",
				Kind:      codersdk.ChatErrorKindTimeout,
				Retryable: true,
			},
		},
		{
			name: "CancelPointer",
			err: &http2.StreamError{
				StreamID: 455,
				Code:     http2.ErrCodeCancel,
				Cause:    xerrors.New("received from peer"),
			},
			want: chaterror.ClassifiedError{
				Message:   "The AI provider is temporarily unavailable.",
				Detail:    "stream error: stream ID 455; CANCEL; received from peer",
				Kind:      codersdk.ChatErrorKindTimeout,
				Retryable: true,
			},
		},
		{
			name: "EnhanceYourCalm",
			err:  peerReset(http2.ErrCodeEnhanceYourCalm),
			want: chaterror.ClassifiedError{
				Message:   "The AI provider is temporarily unavailable.",
				Detail:    "stream error: stream ID 455; ENHANCE_YOUR_CALM; received from peer",
				Kind:      codersdk.ChatErrorKindTimeout,
				Retryable: true,
			},
		},
		{
			name: "NoError",
			err:  peerReset(http2.ErrCodeNo),
			want: chaterror.ClassifiedError{
				Message:   "The AI provider is temporarily unavailable.",
				Detail:    "stream error: stream ID 455; NO_ERROR; received from peer",
				Kind:      codersdk.ChatErrorKindTimeout,
				Retryable: true,
			},
		},
	}

	for _, tt := range retryable {
		t.Run("Retryable/"+tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chaterror.Classify(tt.err))
		})
	}

	localNonRetryable := []struct {
		name string
		err  error
	}{
		{
			name: "CancelWithoutPeerCause",
			err: http2.StreamError{
				StreamID: 455,
				Code:     http2.ErrCodeCancel,
			},
		},
		{
			name: "InternalWithLocalCause",
			err: http2.StreamError{
				StreamID: 455,
				Code:     http2.ErrCodeInternal,
				Cause:    xerrors.New("local transport reset"),
			},
		},
	}
	for _, tt := range localNonRetryable {
		t.Run("NonRetryable/"+tt.name, func(t *testing.T) {
			t.Parallel()
			classified := chaterror.Classify(tt.err)
			require.Equal(t, codersdk.ChatErrorKindGeneric, classified.Kind)
			require.False(t, classified.Retryable)
		})
	}

	nonRetryable := []struct {
		name string
		code http2.ErrCode
	}{
		{name: "Protocol", code: http2.ErrCodeProtocol},
		{name: "FlowControl", code: http2.ErrCodeFlowControl},
		{name: "FrameSize", code: http2.ErrCodeFrameSize},
		{name: "Compression", code: http2.ErrCodeCompression},
	}
	for _, tt := range nonRetryable {
		t.Run("NonRetryable/"+tt.name, func(t *testing.T) {
			t.Parallel()
			classified := chaterror.Classify(peerReset(tt.code))
			require.Equal(t, codersdk.ChatErrorKindGeneric, classified.Kind)
			require.False(t, classified.Retryable)
		})
	}
}

func TestClassify_HTTP2StreamIDDoesNotBecomeStatusCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want chaterror.ClassifiedError
	}{
		{
			name: "RetryableInternalWithAuthLikeStreamID",
			err: http2.StreamError{
				StreamID: 401,
				Code:     http2.ErrCodeInternal,
				Cause:    xerrors.New("received from peer"),
			},
			want: chaterror.ClassifiedError{
				Message:   "The AI provider is temporarily unavailable.",
				Detail:    "stream error: stream ID 401; INTERNAL_ERROR; received from peer",
				Kind:      codersdk.ChatErrorKindTimeout,
				Retryable: true,
			},
		},
		{
			name: "NonRetryableProtocolWithTimeoutLikeStreamID",
			err: http2.StreamError{
				StreamID: 503,
				Code:     http2.ErrCodeProtocol,
				Cause:    xerrors.New("received from peer"),
			},
			want: chaterror.ClassifiedError{
				Message: "The chat request failed unexpectedly.",
				Detail:  "stream error: stream ID 503; PROTOCOL_ERROR; received from peer",
				Kind:    codersdk.ChatErrorKindGeneric,
			},
		},
		{
			name: "StringFallbackInternalWithAuthLikeStreamID",
			err:  xerrors.New("stream error: stream ID 401; INTERNAL_ERROR; received from peer"),
			want: chaterror.ClassifiedError{
				Message:   "The AI provider is temporarily unavailable.",
				Detail:    "stream error: stream ID 401; INTERNAL_ERROR; received from peer",
				Kind:      codersdk.ChatErrorKindTimeout,
				Retryable: true,
			},
		},
		{
			name: "StringProtocolWithTimeoutLikeStreamID",
			err:  xerrors.New("stream error: stream ID 503; PROTOCOL_ERROR; received from peer"),
			want: chaterror.ClassifiedError{
				Message: "The chat request failed unexpectedly.",
				Detail:  "stream error: stream ID 503; PROTOCOL_ERROR; received from peer",
				Kind:    codersdk.ChatErrorKindGeneric,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chaterror.Classify(tt.err))
		})
	}
}

func TestClassify_StatusCodeBeatsTypedHTTP2StreamError(t *testing.T) {
	t.Parallel()

	err := xerrors.Errorf(
		"provider returned status 401: %w",
		http2.StreamError{
			StreamID: 455,
			Code:     http2.ErrCodeInternal,
			Cause:    xerrors.New("received from peer"),
		},
	)

	require.Equal(t, chaterror.ClassifiedError{
		Message:    "Authentication with the AI provider failed. Check the API key and permissions.",
		Kind:       codersdk.ChatErrorKindAuth,
		Retryable:  false,
		StatusCode: 401,
	}, chaterror.Classify(err))
}

// TestClassify_UsageLimitBeatsAuth verifies that quota/billing text
// patterns classify as usage_limit even when auth signals are present.
func TestClassify_UsageLimitBeatsAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          string
		wantKind     codersdk.ChatErrorKind
		wantRetry    bool
		wantStatus   int
		wantProvider string
	}{
		{
			name:      "QuotaBeatsAuth",
			err:       "unauthorized: insufficient_quota",
			wantKind:  codersdk.ChatErrorKindUsageLimit,
			wantRetry: false,
		},
		{
			name:      "PureAuthStillWorks",
			err:       "unauthorized",
			wantKind:  codersdk.ChatErrorKindAuth,
			wantRetry: false,
		},
		{
			name:       "Status401StillAuth",
			err:        "status 401",
			wantKind:   codersdk.ChatErrorKindAuth,
			wantRetry:  false,
			wantStatus: 401,
		},
		{
			// Real production error from OpenAI when quota is exceeded.
			name: "OpenAIInsufficientQuotaRealWorld",
			err: `stream response: received error while streaming: {"type":"insufficient_quota",` +
				`"code":"insufficient_quota","message":"You exceeded your current quota, please check ` +
				`your plan and billing details. For more information on this error, read the docs: ` +
				`https://platform.openai.com/docs/guides/error-codes/api-errors.","param":null}`,
			wantKind:     codersdk.ChatErrorKindUsageLimit,
			wantRetry:    false,
			wantProvider: "openai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			classified := chaterror.Classify(xerrors.New(tt.err))
			require.Equal(t, tt.wantKind, classified.Kind)
			require.Equal(t, tt.wantRetry, classified.Retryable)
			if tt.wantStatus != 0 {
				require.Equal(t, tt.wantStatus, classified.StatusCode)
			}
			if tt.wantProvider != "" {
				require.Equal(t, tt.wantProvider, classified.Provider)
			}
		})
	}
}

func TestClassify_UsageLimitMatchesStructuredDetail(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(testProviderError(
		"upstream failed",
		500,
		nil,
		testProviderResponseDump(`{"error":{"message":"check your billing plan"}}`),
	))

	require.Equal(t, codersdk.ChatErrorKindUsageLimit, classified.Kind)
	require.False(t, classified.Retryable)
	require.Equal(t, 500, classified.StatusCode)
	require.Equal(t, "check your billing plan", classified.Detail)
}

func TestClassify_InsufficientQuotaBeats429RateLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "StatusText",
			err:  xerrors.New("status 429: insufficient_quota"),
		},
		{
			name: "StructuredProviderError",
			err: testProviderError(
				"upstream failed",
				429,
				nil,
				testProviderResponseDump(`{"error":{"message":"insufficient_quota"}}`),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			classified := chaterror.Classify(tt.err)
			require.Equal(t, codersdk.ChatErrorKindUsageLimit, classified.Kind)
			require.False(t, classified.Retryable)
			require.Equal(t, 429, classified.StatusCode)
		})
	}
}

func TestClassify_UsageLimitPatternsDoNotBeat429(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		wantProvider string
	}{
		{
			name:         "GoogleGeminiQuotaText",
			err:          xerrors.New("gemini status 429: Resource has been exhausted (e.g. check quota)."),
			wantProvider: "google",
		},
		{
			name:         "AzureOpenAIQuotaRemaining",
			err:          xerrors.New("azure openai exceeded token rate limit; quota remaining: 0; status 429"),
			wantProvider: "azure",
		},
		{
			name: "BillingPlanRateLimit",
			err:  xerrors.New("status 429: rate limited: upgrade your billing plan for higher rate limits"),
		},
		{
			name: "StructuredProviderQuotaText",
			err:  testProviderError("Resource has been exhausted (e.g. check quota).", 429, nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			classified := chaterror.Classify(tt.err)
			require.Equal(t, codersdk.ChatErrorKindRateLimit, classified.Kind)
			require.True(t, classified.Retryable)
			require.Equal(t, 429, classified.StatusCode)
			require.Equal(t, tt.wantProvider, classified.Provider)
		})
	}
}

// TestClassify_StatusCodeBeatsHTTP2Transport ensures explicit status
// codes still win over the new HTTP/2 patterns.
func TestClassify_StatusCodeBeatsHTTP2Transport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		err           string
		wantKind      codersdk.ChatErrorKind
		wantRetryable bool
		wantStatus    int
	}{
		{
			name:          "HTTP2With429",
			err:           "http2: server error 429 Too Many Requests",
			wantKind:      codersdk.ChatErrorKindRateLimit,
			wantRetryable: true,
			wantStatus:    429,
		},
		{
			name:          "HTTP2With401",
			err:           "http2: 401 unauthorized",
			wantKind:      codersdk.ChatErrorKindAuth,
			wantRetryable: false,
			wantStatus:    401,
		},
		{
			name:          "ClientConnWith429RateLimitWins",
			err:           "http2: client conn is closed: status 429 Too Many Requests",
			wantKind:      codersdk.ChatErrorKindRateLimit,
			wantRetryable: true,
			wantStatus:    429,
		},
		{
			name:          "GOAWAYWith401AuthWins",
			err:           "http2: server sent GOAWAY: status 401 unauthorized",
			wantKind:      codersdk.ChatErrorKindAuth,
			wantRetryable: false,
			wantStatus:    401,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			classified := chaterror.Classify(xerrors.New(tt.err))
			require.Equal(t, tt.wantKind, classified.Kind, "Kind")
			require.Equal(t, tt.wantRetryable, classified.Retryable, "Retryable")
			require.Equal(t, tt.wantStatus, classified.StatusCode, "StatusCode")
		})
	}
}

func TestClassify_StreamSilenceTimeoutWrappedClassificationWins(t *testing.T) {
	t.Parallel()

	wrapped := chaterror.WithClassification(
		xerrors.New("context canceled"),
		chaterror.ClassifiedError{
			Kind:      codersdk.ChatErrorKindStreamSilenceTimeout,
			Provider:  "openai",
			Retryable: true,
		},
	)

	require.Equal(t, chaterror.ClassifiedError{
		Message:    "OpenAI did not send response data in time.",
		Kind:       codersdk.ChatErrorKindStreamSilenceTimeout,
		Provider:   "openai",
		Retryable:  true,
		StatusCode: 0,
	}, chaterror.Classify(wrapped))
}

func TestWithProviderUsesExplicitHint(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(xerrors.New("openai received status 429 from upstream"))
	require.Equal(t, "openai", classified.Provider)

	enriched := classified.WithProvider("azure openai")
	require.Equal(t, chaterror.ClassifiedError{
		Message:    "Azure OpenAI is rate limiting requests.",
		Detail:     "openai received status 429 from upstream",
		Kind:       codersdk.ChatErrorKindRateLimit,
		Provider:   "azure",
		Retryable:  true,
		StatusCode: 429,
	}, enriched)
}

func TestWithProviderAddsProviderWhenUnknown(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(xerrors.New("received status 429 from upstream"))
	require.Empty(t, classified.Provider)

	enriched := classified.WithProvider("openai")
	require.Equal(t, chaterror.ClassifiedError{
		Message:    "OpenAI is rate limiting requests.",
		Detail:     "received status 429 from upstream",
		Kind:       codersdk.ChatErrorKindRateLimit,
		Provider:   "openai",
		Retryable:  true,
		StatusCode: 429,
	}, enriched)
}

func TestClassify_UsesStructuredProviderStatusAndRetryAfter(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(testProviderError(
		"",
		429,
		map[string]string{"Retry-After": "30"},
	))

	require.Equal(t, chaterror.ClassifiedError{
		Message:    "The AI provider is rate limiting requests.",
		Kind:       codersdk.ChatErrorKindRateLimit,
		Provider:   "",
		Retryable:  true,
		StatusCode: 429,
		RetryAfter: 30 * time.Second,
	}, classified)
}

func TestClassify_PrefersRetryAfterMsOverRetryAfter(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(testProviderError(
		"upstream failed",
		429,
		map[string]string{
			"Retry-After":    "30",
			"ReTrY-AfTeR-Ms": "1500",
		},
	))

	require.Equal(t, 429, classified.StatusCode)
	require.Equal(t, 1500*time.Millisecond, classified.RetryAfter)
}

func TestClassify_ParsesRetryAfterHTTPDate(t *testing.T) {
	t.Parallel()

	// http.TimeFormat has second precision, so formatting truncates the
	// sub-second component (up to ~1s of loss). Round the target up to the
	// next whole second before formatting so the parsed deadline is never
	// earlier than now+offset, regardless of where now's fractional second
	// lands. Without this, a now with frac near 1s plus any scheduling
	// jitter can drive the computed RetryAfter just under offset-1s and
	// flake the lower bound.
	offset := 3 * time.Second
	target := time.Now().Add(offset).Truncate(time.Second).Add(time.Second)
	retryAt := target.UTC().Format(http.TimeFormat)
	classified := chaterror.Classify(testProviderError(
		"upstream failed",
		429,
		map[string]string{"Retry-After": retryAt},
	))

	require.Equal(t, 429, classified.StatusCode)
	require.GreaterOrEqual(t, classified.RetryAfter, offset-time.Second)
	require.LessOrEqual(t, classified.RetryAfter, offset+time.Second)
}

func TestClassify_IgnoresInvalidRetryAfter(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(testProviderError(
		"upstream failed",
		429,
		map[string]string{"Retry-After": "definitely not a delay"},
	))

	require.Zero(t, classified.RetryAfter)
}

func TestWithProviderPreservesRetryAfter(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(testProviderError(
		"",
		429,
		map[string]string{"Retry-After": "30"},
	))

	enriched := classified.WithProvider("openai")
	require.Equal(t, 30*time.Second, enriched.RetryAfter)
	require.Equal(t, chaterror.ClassifiedError{
		Message:    "OpenAI is rate limiting requests.",
		Kind:       codersdk.ChatErrorKindRateLimit,
		Provider:   "openai",
		Retryable:  true,
		StatusCode: 429,
		RetryAfter: 30 * time.Second,
	}, enriched)
}

func TestClassify_UsesStructuredProviderDetailFromResponseDump(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(testProviderError(
		"",
		400,
		nil,
		testProviderResponseDump(`{"error":{"type":"invalid_request_error","message":"Image exceeds 5 MB maximum."}}`),
	))

	require.Equal(t, chaterror.ClassifiedError{
		Message:    "The AI provider returned an unexpected error.",
		Detail:     "Image exceeds 5 MB maximum.",
		Kind:       codersdk.ChatErrorKindGeneric,
		Provider:   "",
		Retryable:  false,
		StatusCode: 400,
	}, classified)
}

func TestClassify_UsesTopLevelProviderMessage(t *testing.T) {
	t.Parallel()

	// Many providers return a bare top-level message rather than the
	// nested error envelope. Surface that message directly instead of the
	// raw provider error string.
	classified := chaterror.Classify(testProviderError(
		"",
		400,
		nil,
		testProviderResponseDump(`{"message":"The provided request is not valid"}`),
	))

	require.Equal(t, "The provided request is not valid", classified.Detail)
}

func TestClassify_UnwrapsBedrockTransportWrapper(t *testing.T) {
	t.Parallel()

	// AWS Bedrock errors reach chatd wrapped twice: aibridge returns the
	// nested Anthropic envelope, but error.message is itself the Anthropic
	// SDK transport string that embeds the raw Bedrock body.
	wrapped := `POST \"https://bedrock-runtime.eu-north-1.amazonaws.com/v1/messages\": 400 Bad Request {\"message\":\"The provided request is not valid\"}`
	classified := chaterror.Classify(testProviderError(
		"",
		400,
		nil,
		testProviderResponseDump(`{"error":{"message":"`+wrapped+`","type":"api_error"}}`),
	)).WithProvider("bedrock")

	require.Equal(t, chaterror.ClassifiedError{
		Message:    "AWS Bedrock returned an unexpected error.",
		Detail:     "The provided request is not valid",
		Kind:       codersdk.ChatErrorKindGeneric,
		Provider:   "bedrock",
		Retryable:  false,
		StatusCode: 400,
	}, classified)
}

func TestClassify_DoesNotUnwrapNonTransportMessage(t *testing.T) {
	t.Parallel()

	// A plain nested message that does not match the transport wrapper
	// prefix must pass through unchanged, braces and all.
	classified := chaterror.Classify(testProviderError(
		"",
		400,
		nil,
		testProviderResponseDump(`{"error":{"message":"Value {x} is not allowed."}}`),
	))

	require.Equal(t, "Value {x} is not allowed.", classified.Detail)
}

func TestClassify_UnwrapsTransportWrapperWithBraceInURL(t *testing.T) {
	t.Parallel()

	// A templated URL containing a brace must not be mistaken for the JSON
	// body; the inner message is still extracted.
	wrapped := `POST \"https://example.com/{resource}/invoke\": 400 Bad Request {\"message\":\"real error\"}`
	classified := chaterror.Classify(testProviderError(
		"",
		400,
		nil,
		testProviderResponseDump(`{"error":{"message":"`+wrapped+`"}}`),
	))

	require.Equal(t, "real error", classified.Detail)
}

func TestClassify_KeepsTransportWrapperWhenNoBody(t *testing.T) {
	t.Parallel()

	// When the message matches the transport prefix but has no JSON body at
	// all after it, the wrapper is surfaced unchanged.
	classified := chaterror.Classify(testProviderError(
		`POST "https://example.com/api": 500 Internal Server Error`,
		500,
		nil,
	))

	require.Equal(t, `POST "https://example.com/api": 500 Internal Server Error`, classified.Detail)
}

func TestClassify_KeepsTransportWrapperWhenInnerBodyNotJSON(t *testing.T) {
	t.Parallel()

	// When the message matches the transport prefix but the trailing body
	// has no extractable message, the wrapper is surfaced unchanged rather
	// than dropped.
	wrapped := `POST \"https://bedrock-runtime.eu-north-1.amazonaws.com/v1/messages\": 400 Bad Request {\"foo\":\"bar\"}`
	classified := chaterror.Classify(testProviderError(
		"",
		400,
		nil,
		testProviderResponseDump(`{"error":{"message":"`+wrapped+`"}}`),
	))

	require.Equal(t,
		`POST "https://bedrock-runtime.eu-north-1.amazonaws.com/v1/messages": 400 Bad Request {"foo":"bar"}`,
		classified.Detail)
}

func TestClassify_PrefersNestedMessageOverTopLevel(t *testing.T) {
	t.Parallel()

	// When both shapes are present, the nested error.message wins.
	classified := chaterror.Classify(testProviderError(
		"",
		400,
		nil,
		testProviderResponseDump(`{"message":"top level","error":{"message":"nested wins"}}`),
	))

	require.Equal(t, "nested wins", classified.Detail)
}

// TestClassify_KeepsTopLevelMessageWhenErrorIsNonObject guards against a
// regression where a single decode into a combined struct would fail (and
// silently drop a usable top-level message) whenever "error" is present as a
// non-object value such as a string code.
func TestClassify_KeepsTopLevelMessageWhenErrorIsNonObject(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(testProviderError(
		"",
		429,
		nil,
		testProviderResponseDump(`{"message":"rate limited","error":"rate_limit"}`),
	))

	require.Equal(t, "rate limited", classified.Detail)
}

func TestClassify_AuthKeepsStructuredProviderDetail(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(testProviderError(
		"invalid api key test-key",
		401,
		nil,
		testProviderResponseDump(`{"error":{"message":"Incorrect API key provided."}}`),
	))

	require.Equal(t, chaterror.ClassifiedError{
		Message:    "Authentication with the AI provider failed. Check the API key and permissions.",
		Detail:     "Incorrect API key provided.",
		Kind:       codersdk.ChatErrorKindAuth,
		Retryable:  false,
		StatusCode: 401,
	}, classified)
}

func TestClassify_FallsBackToProviderMessageForDetail(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(testProviderError(
		"  image exceeds 5 MB maximum  ",
		400,
		nil,
		testProviderResponseDump("not-json"),
	))

	require.Equal(t, "image exceeds 5 MB maximum", classified.Detail)
}

func TestClassify_UnwrapsTransportWrapperInMessageFallback(t *testing.T) {
	t.Parallel()

	// When the response dump is unavailable, the detail falls back to
	// providerErr.Message, which for Bedrock via aibridge is itself the SDK
	// transport wrapper. It must be unwrapped to the clean inner message.
	classified := chaterror.Classify(testProviderError(
		`POST "https://bedrock-runtime.eu-north-1.amazonaws.com/v1/messages": 400 Bad Request {"message":"The provided request is not valid"}`,
		400,
		nil,
	))

	require.Equal(t, "The provided request is not valid", classified.Detail)
}

func TestClassify_TruncatesProviderDetail(t *testing.T) {
	t.Parallel()

	detail := strings.Repeat("x", 510)
	classified := chaterror.Classify(testProviderError(
		"",
		400,
		nil,
		testProviderResponseDump(`{"error":{"message":"`+detail+`"}}`),
	))

	require.Len(t, []rune(classified.Detail), 500)
	require.True(t, strings.HasSuffix(classified.Detail, "…"))
}

func TestClassify_MissingKeyPreClassified(t *testing.T) {
	t.Parallel()

	raw := xerrors.New("AI Gateway routing requires the active turn API key ID")
	wrapped := chaterror.WithClassification(raw, chaterror.ClassifiedError{
		Kind:      codersdk.ChatErrorKindMissingKey,
		Retryable: false,
		Detail:    "If this error persists after resending, please report it as a bug.",
	})

	classified := chaterror.Classify(wrapped)
	require.Equal(t, codersdk.ChatErrorKindMissingKey, classified.Kind)
	require.False(t, classified.Retryable)
	require.Equal(t, "If this error persists after resending, please report it as a bug.", classified.Detail)
	require.Equal(t,
		"This conversation was started with an API key that is no longer available."+
			" Send your message again to continue.",
		classified.Message,
		"Message should be filled by terminalMessage when not set explicitly",
	)
}

func testProviderError(
	message string,
	statusCode int,
	headers map[string]string,
	responseBody ...[]byte,
) error {
	var body []byte
	if len(responseBody) > 0 {
		body = responseBody[0]
	}
	return &fantasy.ProviderError{
		Message:         message,
		StatusCode:      statusCode,
		ResponseHeaders: headers,
		ResponseBody:    body,
	}
}

func testProviderResponseDump(body string) []byte {
	return []byte(`HTTP/1.1 400 Bad Request
Content-Type: application/json

` + body)
}
