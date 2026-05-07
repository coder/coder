package chatcompletions //nolint:testpackage // tests unexported internals

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/quartz"
)

func TestScanForCorrelatingToolCallID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		messages []openai.ChatCompletionMessageParamUnion
		expected *string
	}{
		{
			name:     "no messages",
			messages: nil,
			expected: nil,
		},
		{
			name: "no tool messages",
			messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
				openai.AssistantMessage("hi there"),
			},
			expected: nil,
		},
		{
			name: "single tool message",
			messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
				openai.ToolMessage("result", "call_abc"),
			},
			expected: utils.PtrTo("call_abc"),
		},
		{
			name: "multiple tool messages returns last",
			messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
				openai.ToolMessage("first result", "call_first"),
				openai.AssistantMessage("thinking"),
				openai.ToolMessage("second result", "call_second"),
			},
			expected: utils.PtrTo("call_second"),
		},
		{
			name: "last message is not a tool message",
			messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
				openai.ToolMessage("first result", "call_first"),
				openai.AssistantMessage("thinking"),
			},
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := &interceptionBase{
				req: &ChatCompletionNewParamsWrapper{
					ChatCompletionNewParams: openai.ChatCompletionNewParams{
						Messages: tc.messages,
					},
				},
			}

			require.Equal(t, tc.expected, base.CorrelatingToolCallID())
		})
	}
}

func TestProcessKeyPoolError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		err                error
		expectedNil        bool
		expectedStatus     int
		expectedRetryAfter time.Duration
	}{
		{
			// Transient with valid keys present: 429, no Retry-After.
			name:               "transient_zero_retry_after",
			err:                &keypool.TransientKeyPoolError{},
			expectedStatus:     http.StatusTooManyRequests,
			expectedRetryAfter: 0,
		},
		{
			// Transient with cooldown: 429, Retry-After set.
			name:               "transient_with_retry_after",
			err:                &keypool.TransientKeyPoolError{RetryAfter: 5 * time.Second},
			expectedStatus:     http.StatusTooManyRequests,
			expectedRetryAfter: 5 * time.Second,
		},
		{
			// Permanent: 502 api_error.
			name:           "permanent_returns_502",
			err:            keypool.ErrPermanentKeyPool,
			expectedStatus: http.StatusBadGateway,
		},
		{
			// Anything else: not a pool-exhaustion error.
			name:        "non_pool_exhaustion_error_returns_nil",
			err:         xerrors.New("some other error"),
			expectedNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := processKeyPoolError(tc.err)
			if tc.expectedNil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tc.expectedStatus, got.StatusCode)
			assert.Equal(t, tc.expectedRetryAfter, got.RetryAfter)
		})
	}
}

func TestMarkKeyOnError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		err            error
		expectedReturn bool
		expectedState  keypool.KeyState
	}{
		{
			// Not an *openai.Error: no status code to act on.
			name:           "non_api_error_returns_false",
			err:            xerrors.New("network failure"),
			expectedReturn: false,
			expectedState:  keypool.KeyStateValid,
		},
		{
			// Rate-limited: temporary cooldown.
			name:           "429_marks_temporary",
			err:            &openai.Error{StatusCode: http.StatusTooManyRequests, Response: &http.Response{StatusCode: http.StatusTooManyRequests}},
			expectedReturn: true,
			expectedState:  keypool.KeyStateTemporary,
		},
		{
			// Auth failure: mark permanent.
			name:           "401_marks_permanent",
			err:            &openai.Error{StatusCode: http.StatusUnauthorized, Response: &http.Response{StatusCode: http.StatusUnauthorized}},
			expectedReturn: true,
			expectedState:  keypool.KeyStatePermanent,
		},
		{
			// Auth forbidden: mark permanent.
			name:           "403_marks_permanent",
			err:            &openai.Error{StatusCode: http.StatusForbidden, Response: &http.Response{StatusCode: http.StatusForbidden}},
			expectedReturn: true,
			expectedState:  keypool.KeyStatePermanent,
		},
		{
			// Server errors are not key-specific.
			name:           "500_does_not_mark",
			err:            &openai.Error{StatusCode: http.StatusInternalServerError, Response: &http.Response{StatusCode: http.StatusInternalServerError}},
			expectedReturn: false,
			expectedState:  keypool.KeyStateValid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool, err := keypool.New([]string{"key-0"}, quartz.NewMock(t))
			require.NoError(t, err)
			key, err := pool.Walker().Next()
			require.NoError(t, err)

			base := &interceptionBase{cfg: config.OpenAI{KeyPool: pool}, logger: slog.Make()}

			got := base.markKeyOnError(context.Background(), key, tc.err)
			assert.Equal(t, tc.expectedReturn, got)
			assert.Equal(t, tc.expectedState, key.State())
		})
	}
}

func TestWriteUpstreamError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		respErr      *responseError
		expectStatus int
		// Empty string means the header should be absent.
		expectRetryAfter string
		// Substring expected in the marshaled body. Empty means no body check.
		expectBodyContains string
	}{
		{
			// Standard error: status, code, and JSON body written.
			name:               "writes_status_and_body",
			respErr:            newErrorResponse("upstream failed", "api_error", "server_error", http.StatusBadGateway, 0),
			expectStatus:       http.StatusBadGateway,
			expectBodyContains: `"upstream failed"`,
		},
		{
			// OpenAI envelope: the code field round-trips into the body.
			name:               "writes_code_field",
			respErr:            newErrorResponse("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 0),
			expectStatus:       http.StatusTooManyRequests,
			expectBodyContains: `"rate_limit_exceeded"`,
		},
		{
			// Whole-second retryAfter: emitted as integer seconds.
			name:             "retry_after_in_seconds",
			respErr:          newErrorResponse("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 60*time.Second),
			expectStatus:     http.StatusTooManyRequests,
			expectRetryAfter: "60",
		},
		{
			// 500ms rounds up to Retry-After: 1.
			name:             "retry_after_500ms_rounds_up_to_one",
			respErr:          newErrorResponse("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 500*time.Millisecond),
			expectStatus:     http.StatusTooManyRequests,
			expectRetryAfter: "1",
		},
		{
			// 200ms rounds up to Retry-After: 1.
			name:             "retry_after_200ms_rounds_up_to_one",
			respErr:          newErrorResponse("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 200*time.Millisecond),
			expectStatus:     http.StatusTooManyRequests,
			expectRetryAfter: "1",
		},
		{
			// Negative retryAfter: header omitted.
			name:             "negative_retry_after_omits_header",
			respErr:          newErrorResponse("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, -1*time.Second),
			expectStatus:     http.StatusTooManyRequests,
			expectRetryAfter: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := &interceptionBase{logger: slog.Make()}

			w := httptest.NewRecorder()
			base.writeUpstreamError(w, tc.respErr)

			assert.Equal(t, tc.expectStatus, w.Code, "status code")
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"), "Content-Type header")
			assert.Equal(t, tc.expectRetryAfter, w.Header().Get("Retry-After"), "Retry-After header")
			if tc.expectBodyContains != "" {
				assert.Contains(t, w.Body.String(), tc.expectBodyContains, "response body")
			}
		})
	}
}
