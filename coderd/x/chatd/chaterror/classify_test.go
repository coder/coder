package chaterror_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
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
				Message:    "The AI provider is temporarily overloaded (HTTP 529).",
				Kind:       chaterror.KindOverloaded,
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
				Kind:       chaterror.KindOverloaded,
				Provider:   "anthropic",
				Retryable:  true,
				StatusCode: 0,
			},
		},
		{
			name: "AuthBeatsConfig",
			err:  xerrors.New("authentication failed: invalid model"),
			want: chaterror.ClassifiedError{
				Message:    "Authentication with the AI provider failed. Check the API key, permissions, and billing settings.",
				Kind:       chaterror.KindAuth,
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
				Kind:       chaterror.KindConfig,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "BareForbiddenClassifiesAsAuth",
			err:  xerrors.New("forbidden"),
			want: chaterror.ClassifiedError{
				Message:    "Authentication with the AI provider failed. Check the API key, permissions, and billing settings.",
				Kind:       chaterror.KindAuth,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "ExplicitStatus401ClassifiesAsAuth",
			err:  xerrors.New("status 401 from upstream"),
			want: chaterror.ClassifiedError{
				Message:    "Authentication with the AI provider failed. Check the API key, permissions, and billing settings.",
				Kind:       chaterror.KindAuth,
				Provider:   "",
				Retryable:  false,
				StatusCode: 401,
			},
		},
		{
			name: "ExplicitStatus403ClassifiesAsAuth",
			err:  xerrors.New("status 403 from upstream"),
			want: chaterror.ClassifiedError{
				Message:    "Authentication with the AI provider failed. Check the API key, permissions, and billing settings.",
				Kind:       chaterror.KindAuth,
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
				Kind:       chaterror.KindConfig,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
			},
		},
		{
			name: "ExplicitStatus429ClassifiesAsRateLimit",
			err:  xerrors.New("status 429 from upstream"),
			want: chaterror.ClassifiedError{
				Message:    "The AI provider is rate limiting requests (HTTP 429).",
				Kind:       chaterror.KindRateLimit,
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
				Kind:       chaterror.KindConfig,
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
				Kind:       chaterror.KindTimeout,
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
				Kind:       chaterror.KindConfig,
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
				Kind:       chaterror.KindConfig,
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
				Kind:       chaterror.KindConfig,
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
				Kind:       chaterror.KindTimeout,
				Provider:   "",
				Retryable:  false,
				StatusCode: 0,
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

func TestClassify_PatternCoverage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       string
		wantKind  string
		wantRetry bool
	}{
		{name: "OverloadedLiteral", err: "overloaded", wantKind: chaterror.KindOverloaded, wantRetry: true},
		{name: "RateLimitLiteral", err: "rate limit", wantKind: chaterror.KindRateLimit, wantRetry: true},
		{name: "RateLimitUnderscoreLiteral", err: "rate_limit", wantKind: chaterror.KindRateLimit, wantRetry: true},
		{name: "RateLimitedLiteral", err: "rate limited", wantKind: chaterror.KindRateLimit, wantRetry: true},
		{name: "RateLimitedHyphenLiteral", err: "rate-limited", wantKind: chaterror.KindRateLimit, wantRetry: true},
		{name: "TooManyRequestsLiteral", err: "too many requests", wantKind: chaterror.KindRateLimit, wantRetry: true},
		{name: "TimeoutLiteral", err: "timeout", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "TimedOutLiteral", err: "timed out", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "ServiceUnavailableLiteral", err: "service unavailable", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "UnavailableLiteral", err: "unavailable", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "ConnectionResetLiteral", err: "connection reset", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "ConnectionRefusedLiteral", err: "connection refused", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "EOFLiteral", err: "eof", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "BrokenPipeLiteral", err: "broken pipe", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "BadGatewayLiteral", err: "bad gateway", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "GatewayTimeoutLiteral", err: "gateway timeout", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "ClientConnLiteral", err: "client conn", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "GOAWAYLiteral", err: "goaway", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "HTTP2StreamClosedLiteral", err: "http2: stream closed", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "UseOfClosedNetworkConnectionLiteral", err: "use of closed network connection", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "AuthenticationLiteral", err: "authentication", wantKind: chaterror.KindAuth, wantRetry: false},
		{name: "UnauthorizedLiteral", err: "unauthorized", wantKind: chaterror.KindAuth, wantRetry: false},
		{name: "InvalidAPIKeyLiteral", err: "invalid api key", wantKind: chaterror.KindAuth, wantRetry: false},
		{name: "InvalidAPIKeyUnderscoreLiteral", err: "invalid_api_key", wantKind: chaterror.KindAuth, wantRetry: false},
		{name: "QuotaLiteral", err: "quota", wantKind: chaterror.KindAuth, wantRetry: false},
		{name: "BillingLiteral", err: "billing", wantKind: chaterror.KindAuth, wantRetry: false},
		{name: "InsufficientQuotaLiteral", err: "insufficient_quota", wantKind: chaterror.KindAuth, wantRetry: false},
		{name: "PaymentRequiredLiteral", err: "payment required", wantKind: chaterror.KindAuth, wantRetry: false},
		{name: "ForbiddenLiteral", err: "forbidden", wantKind: chaterror.KindAuth, wantRetry: false},
		{name: "InvalidModelLiteral", err: "invalid model", wantKind: chaterror.KindConfig, wantRetry: false},
		{name: "ModelNotFoundLiteral", err: "model not found", wantKind: chaterror.KindConfig, wantRetry: false},
		{name: "ModelNotFoundUnderscoreLiteral", err: "model_not_found", wantKind: chaterror.KindConfig, wantRetry: false},
		{name: "UnsupportedModelLiteral", err: "unsupported model", wantKind: chaterror.KindConfig, wantRetry: false},
		{name: "ContextLengthExceededLiteral", err: "context length exceeded", wantKind: chaterror.KindConfig, wantRetry: false},
		{name: "ContextExceededLiteral", err: "context_exceeded", wantKind: chaterror.KindConfig, wantRetry: false},
		{name: "MaximumContextLengthLiteral", err: "maximum context length", wantKind: chaterror.KindConfig, wantRetry: false},
		{name: "MalformedConfigLiteral", err: "malformed config", wantKind: chaterror.KindConfig, wantRetry: false},
		{name: "MalformedConfigurationLiteral", err: "malformed configuration", wantKind: chaterror.KindConfig, wantRetry: false},
		{name: "ServerErrorLiteral", err: "server error", wantKind: chaterror.KindGeneric, wantRetry: true},
		{name: "InternalServerErrorLiteral", err: "internal server error", wantKind: chaterror.KindGeneric, wantRetry: true},
		{name: "ChatInterruptedLiteral", err: "chat interrupted", wantKind: chaterror.KindGeneric, wantRetry: false},
		{name: "RequestInterruptedLiteral", err: "request interrupted", wantKind: chaterror.KindGeneric, wantRetry: false},
		{name: "OperationInterruptedLiteral", err: "operation interrupted", wantKind: chaterror.KindGeneric, wantRetry: false},
		{name: "Status408", err: "status 408", wantKind: chaterror.KindTimeout, wantRetry: true},
		{name: "Status500", err: "status 500", wantKind: chaterror.KindGeneric, wantRetry: true},
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
			require.Equal(t, chaterror.KindTimeout, classified.Kind)
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
// classify as retryable KindTimeout. Split into two sub-tables so a
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
			require.Equal(t, chaterror.KindTimeout, classified.Kind, "Kind")
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
			require.Equal(t, chaterror.KindTimeout, classified.Kind, "Kind")
			require.True(t, classified.Retryable, "Retryable")
			require.Equal(t, tt.provider, classified.Provider, "Provider")
			require.Equal(t, tt.wantMessage, classified.Message, "Message")
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
		wantKind      string
		wantRetryable bool
		wantStatus    int
	}{
		{
			name:          "HTTP2With429",
			err:           "http2: server error 429 Too Many Requests",
			wantKind:      chaterror.KindRateLimit,
			wantRetryable: true,
			wantStatus:    429,
		},
		{
			name:          "HTTP2With401",
			err:           "http2: 401 unauthorized",
			wantKind:      chaterror.KindAuth,
			wantRetryable: false,
			wantStatus:    401,
		},
		{
			name:          "ClientConnWith429RateLimitWins",
			err:           "http2: client conn is closed: status 429 Too Many Requests",
			wantKind:      chaterror.KindRateLimit,
			wantRetryable: true,
			wantStatus:    429,
		},
		{
			name:          "GOAWAYWith401AuthWins",
			err:           "http2: server sent GOAWAY: status 401 unauthorized",
			wantKind:      chaterror.KindAuth,
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

func TestClassify_StartupTimeoutWrappedClassificationWins(t *testing.T) {
	t.Parallel()

	wrapped := chaterror.WithClassification(
		xerrors.New("context canceled"),
		chaterror.ClassifiedError{
			Kind:      chaterror.KindStartupTimeout,
			Provider:  "openai",
			Retryable: true,
		},
	)

	require.Equal(t, chaterror.ClassifiedError{
		Message:    "OpenAI did not start responding in time.",
		Kind:       chaterror.KindStartupTimeout,
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
		Message:    "Azure OpenAI is rate limiting requests (HTTP 429).",
		Kind:       chaterror.KindRateLimit,
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
		Message:    "OpenAI is rate limiting requests (HTTP 429).",
		Kind:       chaterror.KindRateLimit,
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
		Message:    "The AI provider is rate limiting requests (HTTP 429).",
		Kind:       chaterror.KindRateLimit,
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

	retryAt := time.Now().Add(3 * time.Second).UTC().Format(http.TimeFormat)
	classified := chaterror.Classify(testProviderError(
		"upstream failed",
		429,
		map[string]string{"Retry-After": retryAt},
	))

	require.Equal(t, 429, classified.StatusCode)
	require.GreaterOrEqual(t, classified.RetryAfter, 2*time.Second)
	require.LessOrEqual(t, classified.RetryAfter, 4*time.Second)
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
		"upstream failed",
		429,
		map[string]string{"Retry-After": "30"},
	))

	enriched := classified.WithProvider("openai")
	require.Equal(t, 30*time.Second, enriched.RetryAfter)
	require.Equal(t, chaterror.ClassifiedError{
		Message:    "OpenAI is rate limiting requests (HTTP 429).",
		Kind:       chaterror.KindRateLimit,
		Provider:   "openai",
		Retryable:  true,
		StatusCode: 429,
		RetryAfter: 30 * time.Second,
	}, enriched)
}

func testProviderError(message string, statusCode int, headers map[string]string) error {
	return &fantasy.ProviderError{
		Message:         message,
		StatusCode:      statusCode,
		ResponseHeaders: headers,
	}
}
