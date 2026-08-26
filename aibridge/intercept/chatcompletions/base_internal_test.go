package chatcompletions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/quartz"
)

func TestRecordTokenUsage(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name     string
		msgID    string
		usage    openai.CompletionUsage
		expected *recorder.TokenUsageRecord
	}{
		{
			name:  "with_all_token_details",
			msgID: "cmpl_full",
			usage: openai.CompletionUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
				PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
					CachedTokens:     40,
					CacheWriteTokens: 25,
					AudioTokens:      3,
				},
				CompletionTokensDetails: openai.CompletionUsageCompletionTokensDetails{
					AcceptedPredictionTokens: 7,
					RejectedPredictionTokens: 2,
					AudioTokens:              1,
					ReasoningTokens:          9,
				},
			},
			expected: &recorder.TokenUsageRecord{
				InterceptionID:        id.String(),
				MsgID:                 "cmpl_full",
				Input:                 35, // 100 prompt - 40 cache read - 25 cache write
				Output:                50,
				CacheReadInputTokens:  40,
				CacheWriteInputTokens: 25,
				ExtraTokenTypes: map[string]int64{
					"prompt_audio":                   3,
					"completion_accepted_prediction": 7,
					"completion_rejected_prediction": 2,
					"completion_audio":               1,
					"completion_reasoning":           9,
				},
			},
		},
		{
			name:  "all_tokens_cached",
			msgID: "cmpl_cached",
			usage: openai.CompletionUsage{
				PromptTokens:     100,
				CompletionTokens: 20,
				PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
					CachedTokens: 100,
				},
			},
			expected: &recorder.TokenUsageRecord{
				InterceptionID:       id.String(),
				MsgID:                "cmpl_cached",
				Input:                0, // 100 prompt - 100 cached
				Output:               20,
				CacheReadInputTokens: 100,
				ExtraTokenTypes: map[string]int64{
					"prompt_audio":                   0,
					"completion_accepted_prediction": 0,
					"completion_rejected_prediction": 0,
					"completion_audio":               0,
					"completion_reasoning":           0,
				},
			},
		},
		{
			// Upstream violates the invariant that PromptTokens includes
			// CachedTokens. Input must clamp to 0 so it never panics a
			// Prometheus counter when used as an increment.
			name:  "cached_tokens_exceed_prompt_tokens_clamps_to_zero",
			msgID: "cmpl_cached_exceed",
			usage: openai.CompletionUsage{
				PromptTokens:     40,
				CompletionTokens: 20,
				PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
					CachedTokens:     30,
					CacheWriteTokens: 30,
				},
			},
			expected: &recorder.TokenUsageRecord{
				InterceptionID:        id.String(),
				MsgID:                 "cmpl_cached_exceed",
				Input:                 0, // max(0, 40 prompt - 60 cached)
				Output:                20,
				CacheReadInputTokens:  30,
				CacheWriteInputTokens: 30,
				ExtraTokenTypes: map[string]int64{
					"prompt_audio":                   0,
					"completion_accepted_prediction": 0,
					"completion_rejected_prediction": 0,
					"completion_audio":               0,
					"completion_reasoning":           0,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &testutil.MockRecorder{}
			base := &interceptionBase{
				id:       id,
				recorder: rec,
				logger:   slog.Make(),
			}

			base.recordTokenUsage(t.Context(), tc.msgID, tc.usage)

			tokens := rec.RecordedTokenUsages()
			require.Len(t, tokens, 1)
			got := tokens[0]
			got.CreatedAt = time.Time{} // ignore time
			require.Equal(t, tc.expected, got)
			require.GreaterOrEqual(t, got.Input, int64(0), "input must never be negative")
		})
	}
}

func TestSumUsage(t *testing.T) {
	t.Parallel()

	first := openai.CompletionUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
			CachedTokens:     10,
			CacheWriteTokens: 20,
			AudioTokens:      30,
		},
		CompletionTokensDetails: openai.CompletionUsageCompletionTokensDetails{
			AcceptedPredictionTokens: 40,
			RejectedPredictionTokens: 50,
			AudioTokens:              60,
			ReasoningTokens:          70,
		},
	}
	second := openai.CompletionUsage{
		PromptTokens:     200,
		CompletionTokens: 100,
		TotalTokens:      300,
		PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
			CachedTokens:     1,
			CacheWriteTokens: 2,
			AudioTokens:      3,
		},
		CompletionTokensDetails: openai.CompletionUsageCompletionTokensDetails{
			AcceptedPredictionTokens: 4,
			RejectedPredictionTokens: 5,
			AudioTokens:              6,
			ReasoningTokens:          7,
		},
	}

	require.Equal(t, openai.CompletionUsage{
		PromptTokens:     300,
		CompletionTokens: 150,
		TotalTokens:      450,
		PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
			CachedTokens:     11,
			CacheWriteTokens: 22,
			AudioTokens:      33,
		},
		CompletionTokensDetails: openai.CompletionUsageCompletionTokensDetails{
			AcceptedPredictionTokens: 44,
			RejectedPredictionTokens: 55,
			AudioTokens:              66,
			ReasoningTokens:          77,
		},
	}, sumUsage(first, second))
}

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
			expected: new("call_abc"),
		},
		{
			name: "multiple tool messages returns last",
			messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
				openai.ToolMessage("first result", "call_first"),
				openai.AssistantMessage("thinking"),
				openai.ToolMessage("second result", "call_second"),
			},
			expected: new("call_second"),
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
			// Auth failure: temporary cooldown so the key recovers.
			name:           "401_marks_temporary",
			err:            &openai.Error{StatusCode: http.StatusUnauthorized, Response: &http.Response{StatusCode: http.StatusUnauthorized}},
			expectedReturn: true,
			expectedState:  keypool.KeyStateTemporary,
		},
		{
			// Forbidden is per-request, not key-specific.
			name:           "403_does_not_mark",
			err:            &openai.Error{StatusCode: http.StatusForbidden, Response: &http.Response{StatusCode: http.StatusForbidden}},
			expectedReturn: false,
			expectedState:  keypool.KeyStateValid,
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
			pool, err := keypool.New(config.ProviderOpenAI, []string{"key-0"}, quartz.NewMock(t), nil)
			require.NoError(t, err)
			key, keyPoolErr := pool.Walker().Next()
			require.Nil(t, keyPoolErr)

			base := &interceptionBase{cred: &intercept.CentralizedPool{Pool: pool}, logger: slog.Make()}

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
		respErr      *intercept.ResponseError
		expectStatus int
		// Empty string means the header should be absent.
		expectRetryAfter string
		// Substring expected in the marshaled body. Empty means no body check.
		expectBodyContains string
	}{
		{
			// Standard error: status, code, and JSON body written.
			name:               "writes_status_and_body",
			respErr:            intercept.NewResponseError("upstream failed", "api_error", "server_error", http.StatusBadGateway, 0),
			expectStatus:       http.StatusBadGateway,
			expectBodyContains: `"upstream failed"`,
		},
		{
			// OpenAI envelope: the code field round-trips into the body.
			name:               "writes_code_field",
			respErr:            intercept.NewResponseError("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 0),
			expectStatus:       http.StatusTooManyRequests,
			expectBodyContains: `"rate_limit_exceeded"`,
		},
		{
			// Whole-second retryAfter: emitted as integer seconds.
			name:             "retry_after_in_seconds",
			respErr:          intercept.NewResponseError("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 60*time.Second),
			expectStatus:     http.StatusTooManyRequests,
			expectRetryAfter: "60",
		},
		{
			// 500ms rounds up to Retry-After: 1.
			name:             "retry_after_500ms_rounds_up_to_one",
			respErr:          intercept.NewResponseError("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 500*time.Millisecond),
			expectStatus:     http.StatusTooManyRequests,
			expectRetryAfter: "1",
		},
		{
			// 200ms rounds up to Retry-After: 1.
			name:             "retry_after_200ms_rounds_up_to_one",
			respErr:          intercept.NewResponseError("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 200*time.Millisecond),
			expectStatus:     http.StatusTooManyRequests,
			expectRetryAfter: "1",
		},
		{
			// Negative retryAfter: header omitted.
			name:             "negative_retry_after_omits_header",
			respErr:          intercept.NewResponseError("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, -1*time.Second),
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
